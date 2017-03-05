// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bitgo/rmgd/blockchain"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/mining"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/wire"
)

const (
	// maxNonce is the maximum value a nonce can be in a block header.
	maxNonce = ^uint64(0) // 2^64 - 1

	// hpsUpdateSecs is the number of seconds to wait in between each
	// update to the hashes per second monitor.
	hpsUpdateSecs = 10

	// hashUpdateSec is the number of seconds each worker waits in between
	// notifying the speed monitor with how many hashes have been completed
	// while they are actively searching for a solution.  This is done to
	// reduce the amount of syncs between the workers that must be done to
	// keep track of the hashes per second.
	hashUpdateSecs = 15

	// validateKeysEnvironmentKey specifies the environment var name to
	// look up when populating the validate keys of the CPU miner.
	validateKeysEnvironmentKey = "RMGD_VALIDATE_KEYS"
)

var (
	// defaultNumWorkers is the default number of workers to use for mining
	// and is based on the number of processor cores.  This helps ensure the
	// system stays reasonably responsive under heavy load.
	defaultNumWorkers = uint32(runtime.NumCPU())
)

// CPUMiner provides facilities for solving blocks (mining) using the CPU in
// a concurrency-safe manner.  It consists of two main goroutines -- a speed
// monitor and a controller for worker goroutines which generate and solve
// blocks.  The number of goroutines can be set via the SetMaxGoRoutines
// function, but the default is based on the number of processor cores in the
// system which is typically sufficient.
type CPUMiner struct {
	sync.Mutex
	policy            *mining.Policy
	txSource          mining.TxSource
	server            *server
	numWorkers        uint32
	validateKeys      []*btcec.PrivateKey
	started           bool
	discreteMining    bool
	submitBlockLock   sync.Mutex
	wg                sync.WaitGroup
	workerWg          sync.WaitGroup
	updateNumWorkers  chan struct{}
	queryHashesPerSec chan float64
	updateHashes      chan uint64
	speedMonitorQuit  chan struct{}
	quit              chan struct{}
}

// speedMonitor handles tracking the number of hashes per second the mining
// process is performing.  It must be run as a goroutine.
func (m *CPUMiner) speedMonitor() {
	minrLog.Tracef("CPU miner speed monitor started")

	var hashesPerSec float64
	var totalHashes uint64
	ticker := time.NewTicker(time.Second * hpsUpdateSecs)
	defer ticker.Stop()

out:
	for {
		select {
		// Periodic updates from the workers with how many hashes they
		// have performed.
		case numHashes := <-m.updateHashes:
			totalHashes += numHashes

		// Time to update the hashes per second.
		case <-ticker.C:
			curHashesPerSec := float64(totalHashes) / hpsUpdateSecs
			if hashesPerSec == 0 {
				hashesPerSec = curHashesPerSec
			}
			hashesPerSec = (hashesPerSec + curHashesPerSec) / 2
			totalHashes = 0
			if hashesPerSec != 0 {
				minrLog.Debugf("Hash speed: %6.0f kilohashes/s",
					hashesPerSec/1000)
			}

		// Request for the number of hashes per second.
		case m.queryHashesPerSec <- hashesPerSec:
			// Nothing to do.

		case <-m.speedMonitorQuit:
			break out
		}
	}

	m.wg.Done()
	minrLog.Tracef("CPU miner speed monitor done")
}

// submitBlock submits the passed block to network after ensuring it passes all
// of the consensus validation rules.
func (m *CPUMiner) submitBlock(block *rmgutil.Block) bool {
	m.submitBlockLock.Lock()
	defer m.submitBlockLock.Unlock()

	// Ensure the block is not stale since a new block could have shown up
	// while the solution was being found.  Typically that condition is
	// detected and all work on the stale block is halted to start work on
	// a new block, but the check only happens periodically, so it is
	// possible a block was found and submitted in between.
	latestHash, _ := m.server.blockManager.chainState.Best()
	msgBlock := block.MsgBlock()
	if !msgBlock.Header.PrevBlock.IsEqual(latestHash) {
		minrLog.Debugf("Block submitted via CPU miner with previous "+
			"block %s is stale", msgBlock.Header.PrevBlock)
		return false
	}

	// Process this block using the same rules as blocks coming from other
	// nodes.  This will in turn relay it to the network like normal.
	isOrphan, err := m.server.blockManager.ProcessBlock(block, blockchain.BFNone)
	if err != nil {
		// Anything other than a rule violation is an unexpected error,
		// so log that error as an internal error.
		if _, ok := err.(blockchain.RuleError); !ok {
			minrLog.Errorf("Unexpected error while processing "+
				"block submitted via CPU miner: %v", err)
			return false
		}

		minrLog.Debugf("Block submitted via CPU miner rejected: %v", err)
		return false
	}
	if isOrphan {
		minrLog.Debugf("Block submitted via CPU miner is an orphan")
		return false
	}

	// The block was accepted.
	coinbaseTx := block.MsgBlock().Transactions[0].TxOut[0]
	minrLog.Infof("Block submitted via CPU miner accepted (hash %s, "+
		"amount %v)", block.Hash(), rmgutil.Amount(coinbaseTx.Value))
	return true
}

// solveBlock attempts to find some combination of a nonce and current
// timestamp which makes the passed block hash to a value less than the
// target difficulty.  The timestamp is updated periodically and the passed
// block is modified with all tweaks during this process.  This means that
// when the function returns true, the block is ready for submission.
//
// This function will return early with false when conditions that trigger a
// stale block such as a new block showing up or periodically when there are
// new transactions and enough time has elapsed without finding a solution.
func (m *CPUMiner) solveBlock(msgBlock *wire.MsgBlock, blockHeight uint32,
	ticker *time.Ticker, validateKey *btcec.PrivateKey,
	quit chan struct{}) bool {

	// Create a couple of convenience variables.
	header := &msgBlock.Header
	targetDifficulty := blockchain.CompactToBig(header.Bits)

	// Initial state.
	lastGenerated := time.Now()
	lastTxUpdate := m.txSource.LastUpdated()
	hashesCompleted := uint64(0)

	// Search through the entire nonce range for a solution while
	// periodically checking for early quit and stale block
	// conditions along with updates to the speed monitor.
	for i := uint64(0); i <= maxNonce; i++ {
		select {
		case <-quit:
			return false

		case <-ticker.C:
			m.updateHashes <- hashesCompleted
			hashesCompleted = 0

			// The current block is stale if the best block
			// has changed.
			bestHash, _ := m.server.blockManager.chainState.Best()
			if !header.PrevBlock.IsEqual(bestHash) {
				return false
			}

			// The current block is stale if the memory pool
			// has been updated since the block template was
			// generated and it has been at least one
			// minute.
			if lastTxUpdate != m.txSource.LastUpdated() &&
				time.Now().After(lastGenerated.Add(time.Minute)) {

				return false
			}

			UpdateBlockTime(msgBlock, m.server.blockManager, validateKey)

		default:
			// Non-blocking select to fall through
		}

		// Update the nonce and hash the block header.  Each
		// hash is actually a double sha256 (two hashes), so
		// increment the number of hashes completed for each
		// attempt accordingly.
		// TODO(prova) update this to reflect new hash function
		header.Nonce = i
		hash := header.BlockHash()
		hashesCompleted += 2

		// The block is solved when the new block hash is less
		// than the target difficulty.  Yay!
		if blockchain.HashToBig(&hash).Cmp(targetDifficulty) <= 0 {
			m.updateHashes <- hashesCompleted
			return true
		}
	}

	return false
}

// generateBlocks is a worker that is controlled by the miningWorkerController.
// It is self contained in that it creates block templates and attempts to solve
// them while detecting when it is performing stale work and reacting
// accordingly by generating a new block template.  When a block is solved, it
// is submitted.
//
// It must be run as a goroutine.
func (m *CPUMiner) generateBlocks(quit chan struct{}) {
	minrLog.Tracef("Starting generate blocks worker")

	// Start a ticker which is used to signal checks for stale work and
	// updates to the speed monitor.
	ticker := time.NewTicker(time.Second * hashUpdateSecs)
	defer ticker.Stop()
out:
	for {
		// Quit when the miner is stopped.
		select {
		case <-quit:
			break out
		default:
			// Non-blocking select to fall through
		}

		// TODO(prova): re-enable later, this cuts off mining when
		// no other peers are present.
		//
		// Wait until there is a connection to at least one other peer
		// since there is no way to relay a found block or receive
		// transactions to work on when there are no connected peers.
		// if m.server.ConnectedCount() == 0 {
		// 	time.Sleep(time.Second)
		// 	continue
		// }

		// No point in searching for a solution before the chain is
		// synced.  Also, grab the same lock as used for block
		// submission, since the current block will be changing and
		// this would otherwise end up building a new block template on
		// a block that is in the process of becoming stale.
		m.submitBlockLock.Lock()
		_, curHeight := m.server.blockManager.chainState.Best()
		if curHeight != 0 && !m.server.blockManager.IsCurrent() {
			m.submitBlockLock.Unlock()
			time.Sleep(time.Second)
			continue
		}

		// Choose a payment address at random.
		rand.Seed(time.Now().UnixNano())
		payToAddr := cfg.miningAddrs[rand.Intn(len(cfg.miningAddrs))]

		// Confirm that validate keys are present.
		if len(m.validateKeys) == 0 {
			errStr := fmt.Sprintf("Missing validate keys, set via"+
				" setvalidatekeys or env var %s", validateKeysEnvironmentKey)
			minrLog.Errorf(errStr)
			continue
		}

		// Check for invalid validate keys and stop generating if there
		// are any invalid keys detected.
		invalidValidateKey := m.detectInvalidValidateKey()
		if invalidValidateKey != nil {
			str := fmt.Sprintf("invalid validate key %v",
				invalidValidateKey.SerializeCompressed())
			minrLog.Errorf(str)
			m.submitBlockLock.Unlock()
			time.Sleep(10 * time.Second)
			continue
		}

		// Pick a validate key to use, absent rate-limited keys.
		var nonRateLimitedValidateKeys []*btcec.PrivateKey
		var validateKey *btcec.PrivateKey
		var validateKeyErr error
		for _, privKey := range m.validateKeys {
			var validatePubKey wire.BlockValidatingPubKey
			copy(validatePubKey[:wire.BlockValidatingPubKeySize], privKey.PubKey().SerializeCompressed()[:wire.BlockValidatingPubKeySize])
			validateKeyErr, isRateLimited := m.server.blockManager.chain.IsValidateKeyRateLimited(validatePubKey)
			if validateKeyErr != nil || isRateLimited {
				continue
			}
			nonRateLimitedValidateKeys = append(nonRateLimitedValidateKeys, privKey)
		}
		if validateKeyErr != nil {
			m.submitBlockLock.Unlock()
			errStr := fmt.Sprintf("Failed checking validate key %v", validateKeyErr)
			minrLog.Errorf(errStr)
			time.Sleep(time.Second)
			continue
		}
		if keysCount := len(nonRateLimitedValidateKeys); keysCount > 0 {
			// Choose a signing key at random.
			validateKey = nonRateLimitedValidateKeys[rand.Intn(keysCount)]
		} else {
			m.submitBlockLock.Unlock()
			errStr := fmt.Sprintf("Block generation rate limited.")
			minrLog.Errorf(errStr)
			time.Sleep(5 * time.Second)
			continue
		}

		// Create a new block template using the available transactions
		// in the memory pool as a source of transactions to potentially
		// include in the block.
		template, err := NewBlockTemplate(m.policy, m.server, payToAddr, validateKey)
		m.submitBlockLock.Unlock()
		if err != nil {
			errStr := fmt.Sprintf("Failed to create new block "+
				"template: %v", err)
			minrLog.Errorf(errStr)
			time.Sleep(time.Second)
			continue
		}

		// Attempt to solve the block.  The function will exit early
		// with false when conditions that trigger a stale block, so
		// a new block template can be generated.  When the return is
		// true a solution was found, so submit the solved block.
		if m.solveBlock(template.Block, curHeight+1, ticker, validateKey, quit) {
			block := rmgutil.NewBlock(template.Block)
			m.submitBlock(block)
		}
	}

	m.workerWg.Done()
	minrLog.Tracef("Generate blocks worker done")
}

// detectInvalidValidateKey determines if there is an invalid validate key in
// the miner's validate key set.  If there is an invalid key, it is returned.
func (m *CPUMiner) detectInvalidValidateKey() *btcec.PublicKey {
	adminKeySets := m.server.blockManager.chain.AdminKeySets()
	validateKeySet := adminKeySets[btcec.ValidateKeySet]
	for _, validateKey := range m.validateKeys {
		if validateKeySet.Pos(validateKey.PubKey()) == -1 {
			return validateKey.PubKey()
		}
	}
	return nil
}

// miningWorkerController launches the worker goroutines that are used to
// generate block templates and solve them.  It also provides the ability to
// dynamically adjust the number of running worker goroutines.
//
// It must be run as a goroutine.
func (m *CPUMiner) miningWorkerController() {
	// launchWorkers groups common code to launch a specified number of
	// workers for generating blocks.
	var runningWorkers []chan struct{}
	launchWorkers := func(numWorkers uint32) {
		for i := uint32(0); i < numWorkers; i++ {
			quit := make(chan struct{})
			runningWorkers = append(runningWorkers, quit)

			m.workerWg.Add(1)
			go m.generateBlocks(quit)
		}
	}

	// Launch the current number of workers by default.
	runningWorkers = make([]chan struct{}, 0, m.numWorkers)
	launchWorkers(m.numWorkers)

out:
	for {
		select {
		// Update the number of running workers.
		case <-m.updateNumWorkers:
			// No change.
			numRunning := uint32(len(runningWorkers))
			if m.numWorkers == numRunning {
				continue
			}

			// Add new workers.
			if m.numWorkers > numRunning {
				launchWorkers(m.numWorkers - numRunning)
				continue
			}

			// Signal the most recently created goroutines to exit.
			for i := numRunning - 1; i >= m.numWorkers; i-- {
				close(runningWorkers[i])
				runningWorkers[i] = nil
				runningWorkers = runningWorkers[:i]
			}

		case <-m.quit:
			for _, quit := range runningWorkers {
				close(quit)
			}
			break out
		}
	}

	// Wait until all workers shut down to stop the speed monitor since
	// they rely on being able to send updates to it.
	m.workerWg.Wait()
	close(m.speedMonitorQuit)
	m.wg.Done()
}

// EstablishValidateKeys attempts to populate validate keys from an env var.
func (m *CPUMiner) EstablishValidateKeys() {
	validateKeyValue := os.Getenv(validateKeysEnvironmentKey)
	// Avoid attempting to establish validate keys when there is no value.
	if validateKeyValue == "" {
		return
	}
	validateKeys := strings.Split(validateKeyValue, ",")
	validatePrivKeys := make([]*btcec.PrivateKey, len(validateKeys))
	for i, privKeyStr := range validateKeys {
		privKeyBytes, err := hex.DecodeString(privKeyStr)
		if err != nil {
			errStr := fmt.Sprintf("Failed parsing validate"+
				" key: %v %v", privKeyStr, err)
			minrLog.Errorf(errStr)
			return
		}
		privKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), privKeyBytes)
		validatePrivKeys[i] = privKey
	}
	m.validateKeys = validatePrivKeys
}

// Start begins the CPU mining process as well as the speed monitor used to
// track hashing metrics.  Calling this function when the CPU miner has
// already been started will have no effect.
//
// This function is safe for concurrent access.
func (m *CPUMiner) Start() {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is already running or if running in discrete
	// mode (using GenerateNBlocks).
	if m.started || m.discreteMining {
		return
	}

	if len(m.validateKeys) == 0 {
		m.EstablishValidateKeys()
	}

	m.quit = make(chan struct{})
	m.speedMonitorQuit = make(chan struct{})
	m.wg.Add(2)
	go m.speedMonitor()
	go m.miningWorkerController()

	m.started = true
	minrLog.Infof("CPU miner started")
}

// Stop gracefully stops the mining process by signalling all workers, and the
// speed monitor to quit.  Calling this function when the CPU miner has not
// already been started will have no effect.
//
// This function is safe for concurrent access.
func (m *CPUMiner) Stop() {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is not currently running or if running in
	// discrete mode (using GenerateNBlocks).
	if !m.started || m.discreteMining {
		return
	}

	close(m.quit)
	m.wg.Wait()
	m.started = false
	minrLog.Infof("CPU miner stopped")
}

// IsMining returns whether or not the CPU miner has been started and is
// therefore currenting mining.
//
// This function is safe for concurrent access.
func (m *CPUMiner) IsMining() bool {
	m.Lock()
	defer m.Unlock()

	return m.started
}

// HashesPerSecond returns the number of hashes per second the mining process
// is performing.  0 is returned if the miner is not currently running.
//
// This function is safe for concurrent access.
func (m *CPUMiner) HashesPerSecond() float64 {
	m.Lock()
	defer m.Unlock()

	// Nothing to do if the miner is not currently running.
	if !m.started {
		return 0
	}

	return <-m.queryHashesPerSec
}

// SetNumWorkers sets the number of workers to create which solve blocks.  Any
// negative values will cause a default number of workers to be used which is
// based on the number of processor cores in the system.  A value of 0 will
// cause all CPU mining to be stopped.
//
// This function is safe for concurrent access.
func (m *CPUMiner) SetNumWorkers(numWorkers int32) {
	if numWorkers == 0 {
		m.Stop()
	}

	// Don't lock until after the first check since Stop does its own
	// locking.
	m.Lock()
	defer m.Unlock()

	// Use default if provided value is negative.
	if numWorkers < 0 {
		m.numWorkers = defaultNumWorkers
	} else {
		m.numWorkers = uint32(numWorkers)
	}

	// When the miner is already running, notify the controller about the
	// the change.
	if m.started {
		m.updateNumWorkers <- struct{}{}
	}
}

// NumWorkers returns the number of workers which are running to solve blocks.
//
// This function is safe for concurrent access.
func (m *CPUMiner) NumWorkers() int32 {
	m.Lock()
	defer m.Unlock()

	return int32(m.numWorkers)
}

// SetValidateKeys updates the private keys used for signing.
//
// This function is safe for concurrent access.
func (m *CPUMiner) SetValidateKeys(validateKeys []*btcec.PrivateKey) {
	m.Lock()
	defer m.Unlock()
	m.validateKeys = validateKeys
}

// ValidateKeys returns the validate keys set to sign blocks.
//
// This function is safe for concurrent access.
func (m *CPUMiner) ValidateKeys() []*btcec.PrivateKey {
	m.Lock()
	defer m.Unlock()
	return m.validateKeys
}

// GenerateNBlocks generates the requested number of blocks. It is self
// contained in that it creates block templates and attempts to solve them while
// detecting when it is performing stale work and reacting accordingly by
// generating a new block template.  When a block is solved, it is submitted.
// The function returns a list of the hashes of generated blocks.
func (m *CPUMiner) GenerateNBlocks(n uint32) ([]*chainhash.Hash, error) {
	m.Lock()

	// Respond with an error if there's virtually 0 chance of CPU-mining a block.
	if !m.server.chainParams.GenerateSupported {
		m.Unlock()
		return nil, errors.New("No support for `generate` on the current " +
			"network, " + m.server.chainParams.Net.String() +
			", as it's unlikely to be possible to CPU-mine a block.")
	}

	// Respond with an error if server is already mining.
	if m.started || m.discreteMining {
		m.Unlock()
		return nil, errors.New("Server is already CPU mining. Please call " +
			"`setgenerate 0` before calling discrete `generate` commands.")
	}

	m.started = true
	m.discreteMining = true

	m.speedMonitorQuit = make(chan struct{})
	m.wg.Add(1)
	go m.speedMonitor()

	m.Unlock()

	minrLog.Tracef("Generating %d blocks", n)

	i := uint32(0)
	blockHashes := make([]*chainhash.Hash, n, n)

	// Start a ticker which is used to signal checks for stale work and
	// updates to the speed monitor.
	ticker := time.NewTicker(time.Second * hashUpdateSecs)
	defer ticker.Stop()

	for {
		// Read updateNumWorkers in case someone tries a `setgenerate` while
		// we're generating. We can ignore it as the `generate` RPC call only
		// uses 1 worker.
		select {
		case <-m.updateNumWorkers:
		default:
		}

		// Grab the lock used for block submission, since the current block will
		// be changing and this would otherwise end up building a new block
		// template on a block that is in the process of becoming stale.
		m.submitBlockLock.Lock()
		_, curHeight := m.server.blockManager.chainState.Best()

		// Choose a payment address at random.
		rand.Seed(time.Now().UnixNano())
		payToAddr := cfg.miningAddrs[rand.Intn(len(cfg.miningAddrs))]

		// Choose a validate key at random.
		validateKeys := m.ValidateKeys()
		validateKey := validateKeys[rand.Intn(len(validateKeys))]

		// Create a new block template using the available transactions
		// in the memory pool as a source of transactions to potentially
		// include in the block.
		template, err := NewBlockTemplate(m.policy, m.server, payToAddr, validateKey)
		m.submitBlockLock.Unlock()
		if err != nil {
			errStr := fmt.Sprintf("Failed to create new block "+
				"template: %v", err)
			minrLog.Errorf(errStr)
			continue
		}

		// Attempt to solve the block.  The function will exit early
		// with false when conditions that trigger a stale block, so
		// a new block template can be generated.  When the return is
		// true a solution was found, so submit the solved block.
		if m.solveBlock(template.Block, curHeight+1, ticker, validateKey, nil) {
			block := rmgutil.NewBlock(template.Block)
			m.submitBlock(block)
			blockHashes[i] = block.Hash()
			i++
			if i == n {
				minrLog.Tracef("Generated %d blocks", i)
				m.Lock()
				close(m.speedMonitorQuit)
				m.wg.Wait()
				m.started = false
				m.discreteMining = false
				m.Unlock()
				return blockHashes, nil
			}
		}
	}
}

// newCPUMiner returns a new instance of a CPU miner for the provided server.
// Use Start to begin the mining process.  See the documentation for CPUMiner
// type for more details.
func newCPUMiner(policy *mining.Policy, s *server) *CPUMiner {
	return &CPUMiner{
		policy:            policy,
		txSource:          s.txMemPool,
		server:            s,
		numWorkers:        defaultNumWorkers,
		updateNumWorkers:  make(chan struct{}),
		queryHashesPerSec: make(chan float64),
		updateHashes:      make(chan uint64),
	}
}
