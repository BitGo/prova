// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"encoding/hex"
	"github.com/bitgo/rmgd/blockchain"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/txscript"
	"github.com/bitgo/rmgd/wire"
	"reflect"
	"sync"
	"testing"
)

// fakeChain is used by the pool harness to provide generated test utxos and
// a current faked chain height to the pool callbacks.  This, in turn, allows
// transations to be appear as though they are spending completely valid utxos.
type fakeChain struct {
	sync.RWMutex
	utxos         *blockchain.UtxoViewpoint
	currentHeight uint32
}

// FetchUtxoView loads utxo details about the input transactions referenced by
// the passed transaction from the point of view of the fake chain.
// It also attempts to fetch the utxo details for the transaction itself so the
// returned view can be examined for duplicate unspent transaction outputs.
//
// This function is safe for concurrent access however the returned view is NOT.
func (s *fakeChain) FetchUtxoView(tx *rmgutil.Tx) (*blockchain.UtxoViewpoint, error) {
	s.RLock()
	defer s.RUnlock()

	// All entries are cloned to ensure modifications to the returned view
	// do not affect the fake chain's view.

	// Add an entry for the tx itself to the new view.
	viewpoint := blockchain.NewUtxoViewpoint()
	entry := s.utxos.LookupEntry(tx.Hash())
	viewpoint.Entries()[*tx.Hash()] = entry.Clone()

	// Add entries for all of the inputs to the tx to the new view.
	for _, txIn := range tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		entry := s.utxos.LookupEntry(originHash)
		viewpoint.Entries()[*originHash] = entry.Clone()
	}

	return viewpoint, nil
}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// ThreadTips returns the thread tips on the fake chain instance.
func (s *fakeChain) ThreadTips() map[rmgutil.ThreadID]*wire.OutPoint {
	return make(map[rmgutil.ThreadID]*wire.OutPoint)
}

// LastKeyID returns the last issued keyID on the the fake chain instance.
func (s *fakeChain) LastKeyID() btcec.KeyID {
	return btcec.KeyID(0)
}

// TotalSupply returns the total supply on the fake chain instance.
func (s *fakeChain) TotalSupply() uint64 {
	return uint64(0)
}

// AdminKeySets returns the set of admin keys on the fake chain instance.
func (s *fakeChain) AdminKeySets() map[btcec.KeySetType]btcec.PublicKeySet {
	return make(map[btcec.KeySetType]btcec.PublicKeySet)
}

// KeyIDs returns all keyID to pub key mapping set on the fake chain instance.
func (s *fakeChain) KeyIDs() btcec.KeyIdMap {
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})
	pubKey1, _ := btcec.ParsePubKey(hexToBytes("025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1"), btcec.S256())
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})
	pubKey2, _ := btcec.ParsePubKey(hexToBytes("038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202"), btcec.S256())
	return map[btcec.KeyID]*btcec.PublicKey{keyId1: pubKey1, keyId2: pubKey2}
}

// BestHeight returns the current height associated with the fake chain
// instance.
func (s *fakeChain) BestHeight() uint32 {
	s.RLock()
	height := s.currentHeight
	s.RUnlock()
	return height
}

// SetHeight sets the current height associated with the fake chain instance.
func (s *fakeChain) SetHeight(height uint32) {
	s.Lock()
	s.currentHeight = height
	s.Unlock()
}

// spendableOutput is a convenience type that houses a particular utxo and the
// amount associated with it.
type spendableOutput struct {
	outPoint wire.OutPoint
	amount   rmgutil.Amount
}

// txOutToSpendableOut returns a spendable output given a transaction and index
// of the output to use.  This is useful as a convenience when creating test
// transactions.
func txOutToSpendableOut(tx *rmgutil.Tx, outputNum uint32) spendableOutput {
	return spendableOutput{
		outPoint: wire.OutPoint{Hash: *tx.Hash(), Index: outputNum},
		amount:   rmgutil.Amount(tx.MsgTx().TxOut[outputNum].Value),
	}
}

// poolHarness provides a harness that includes functionality for creating and
// signing transactions as well as a fake chain that provides utxos for use in
// generating valid transactions.
type poolHarness struct {
	// privKey1 / 2 are the signing keys used for creating transactions throughout
	// the tests.
	//
	// payAddr is the Prova address for the signing keys and is used for the
	// payment address throughout the tests.
	privKey1    *btcec.PrivateKey
	privKey2    *btcec.PrivateKey
	payAddr     rmgutil.Address
	payScript   []byte
	chainParams *chaincfg.Params

	chain  *fakeChain
	txPool *TxPool
}

// CreateCoinbaseTx returns a coinbase transaction with the requested number of
// outputs paying an appropriate subsidy based on the passed block height to the
// address associated with the harness.  It automatically uses a standard
// signature script that starts with the block height that is required by
// version 2 blocks.
func (p *poolHarness) CreateCoinbaseTx(blockHeight uint32, numOutputs uint32) (*rmgutil.Tx, error) {
	// Create standard coinbase script.
	coinbaseScript, err := txscript.NewScriptBuilder().
		AddInt64(int64(blockHeight)).Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})
	totalInput := blockchain.CalcBlockSubsidy(blockHeight, p.chainParams)
	amountPerOutput := totalInput / int64(numOutputs)
	remainder := totalInput - amountPerOutput*int64(numOutputs)
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    amount,
		})
	}

	return rmgutil.NewTx(tx), nil
}

// CreateSignedTx creates a new signed transaction that consumes the provided
// inputs and generates the provided number of outputs by evenly splitting the
// total input amount.  All outputs will be to the payment script associated
// with the harness and all inputs are assumed to do the same.
func (p *poolHarness) CreateSignedTx(inputs []spendableOutput, numOutputs uint32) (*rmgutil.Tx, error) {
	// Calculate the total input amount and split it amongst the requested
	// number of outputs.
	var totalInput rmgutil.Amount
	for _, input := range inputs {
		totalInput += input.amount
	}
	amountPerOutput := int64(totalInput) / int64(numOutputs)
	remainder := int64(totalInput) - amountPerOutput*int64(numOutputs)

	tx := wire.NewMsgTx()
	for _, input := range inputs {
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: input.outPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
		})
	}
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    amount,
		})
	}

	lookupKey := func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
		return []txscript.PrivateKey{
			txscript.PrivateKey{p.privKey1, true},
			txscript.PrivateKey{p.privKey2, true},
		}, nil
	}

	// Sign the new transaction.
	for i := range tx.TxIn {
		sigScript, err := txscript.SignTxOutput(p.chainParams, tx,
			i, tx.TxOut[i].Value, p.payScript, txscript.SigHashAll, txscript.KeyClosure(lookupKey), nil, nil)
		if err != nil {
			return nil, err
		}
		tx.TxIn[i].SignatureScript = sigScript
	}

	return rmgutil.NewTx(tx), nil
}

// CreateTxChain creates a chain of zero-fee transactions (each subsequent
// transaction spends the entire amount from the previous one) with the first
// one spending the provided outpoint.  Each transaction spends the entire
// amount of the previous one and as such does not include any fees.
func (p *poolHarness) CreateTxChain(firstOutput spendableOutput, numTxns uint32) ([]*rmgutil.Tx, error) {
	txChain := make([]*rmgutil.Tx, 0, numTxns)
	prevOutPoint := firstOutput.outPoint
	spendableAmount := firstOutput.amount
	lookupKey := func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
		return []txscript.PrivateKey{
			txscript.PrivateKey{p.privKey1, true},
			txscript.PrivateKey{p.privKey2, true},
		}, nil
	}
	for i := uint32(0); i < numTxns; i++ {
		// Create the transaction using the previous transaction output
		// and paying the full amount to the payment address associated
		// with the harness.
		tx := wire.NewMsgTx()
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOutPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
		})
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    int64(spendableAmount),
		})

		// Sign the new transaction.
		sigScript, err := txscript.SignTxOutput(p.chainParams, tx,
			0, tx.TxOut[0].Value, p.payScript, txscript.SigHashAll, txscript.KeyClosure(lookupKey), nil, nil)
		if err != nil {
			return nil, err
		}
		tx.TxIn[0].SignatureScript = sigScript

		txChain = append(txChain, rmgutil.NewTx(tx))

		// Next transaction uses outputs from this one.
		prevOutPoint = wire.OutPoint{Hash: tx.TxHash(), Index: 0}
	}

	return txChain, nil
}

// newPoolHarness returns a new instance of a pool harness initialized with a
// fake chain and a TxPool bound to it that is configured with a policy suitable
// for testing.  Also, the fake chain is populated with the returned spendable
// outputs so the caller can easily create new valid transactions which build
// off of it.
func newPoolHarness(chainParams *chaincfg.Params) (*poolHarness, []spendableOutput, error) {
	// Use a hard coded key pair for deterministic results.
	privKey1, pubKey1 := btcec.PrivKeyFromBytes(btcec.S256(), []byte{
		0x2b, 0x8c, 0x52, 0xb7, 0x7b, 0x32, 0x7c, 0x75,
		0x5b, 0x9b, 0x37, 0x55, 0x00, 0xd3, 0xf4, 0xb2,
		0xda, 0x9b, 0x0a, 0x1f, 0xf6, 0x5f, 0x68, 0x91,
		0xd3, 0x11, 0xfe, 0x94, 0x29, 0x5b, 0xc2, 0x6a,
	})
	privKey2, _ := btcec.PrivKeyFromBytes(btcec.S256(), []byte{
		0xea, 0xf0, 0x2c, 0xa3, 0x48, 0xc5, 0x24, 0xe6,
		0x39, 0x26, 0x55, 0xba, 0x4d, 0x29, 0x60, 0x3c,
		0xd1, 0xa7, 0x34, 0x7d, 0x9d, 0x65, 0xcf, 0xe9,
		0x3c, 0xe1, 0xeb, 0xff, 0xdc, 0xa2, 0x26, 0x94,
	})
	pkHash := rmgutil.Hash160(pubKey1.SerializeCompressed())
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})

	// Generate associated Prova address and resulting payment script.
	payAddr, err := rmgutil.NewAddressProva(pkHash, []btcec.KeyID{keyId1, keyId2}, chainParams)
	if err != nil {
		return nil, nil, err
	}
	pkScript, err := txscript.PayToAddrScript(payAddr)
	if err != nil {
		return nil, nil, err
	}

	// Create a new fake chain and harness bound to it.
	chain := &fakeChain{utxos: blockchain.NewUtxoViewpoint()}
	harness := poolHarness{
		privKey1:    privKey1,
		privKey2:    privKey2,
		payAddr:     payAddr,
		payScript:   pkScript,
		chainParams: chainParams,

		chain: chain,
		txPool: New(&Config{
			Policy: Policy{
				DisableRelayPriority: true,
				FreeTxRelayLimit:     15.0,
				MaxOrphanTxs:         5,
				MaxOrphanTxSize:      1000,
				MaxSigOpsPerTx:       blockchain.MaxSigOpsPerBlock / 5,
				MinRelayTxFee:        1000, // 1 Atom per byte
			},
			ChainParams:     chainParams,
			FetchUtxoView:   chain.FetchUtxoView,
			ThreadTips:      chain.ThreadTips,
			LastKeyID:       chain.LastKeyID,
			TotalSupply:     chain.TotalSupply,
			GetKeyIDs:       chain.KeyIDs,
			GetAdminKeySets: chain.AdminKeySets,
			BestHeight:      chain.BestHeight,
			SigCache:        nil,
			HashCache:       txscript.NewHashCache(200),
			TimeSource:      blockchain.NewMedianTime(),
			AddrIndex:       nil,
		}),
	}

	// Create a single coinbase transaction and add it to the harness
	// chain's utxo set and set the harness chain height such that the
	// coinbase will mature in the next block.  This ensures the txpool
	// accepts transactions which spend immature coinbases that will become
	// mature in the next block.
	numOutputs := uint32(1)
	outputs := make([]spendableOutput, 0, numOutputs)
	curHeight := harness.chain.BestHeight()
	coinbase, err := harness.CreateCoinbaseTx(curHeight+1, numOutputs)
	if err != nil {
		return nil, nil, err
	}
	harness.chain.utxos.AddTxOuts(coinbase, curHeight+1)
	for i := uint32(0); i < numOutputs; i++ {
		outputs = append(outputs, txOutToSpendableOut(coinbase, i))
	}
	harness.chain.SetHeight(uint32(chainParams.CoinbaseMaturity) + curHeight)

	return &harness, outputs, nil
}

// TestSimpleOrphanChain ensures that a simple chain of orphans is handled
// properly.  In particular, it generates a chain of single input, single output
// transactions and inserts them while skipping the first linking transaction so
// they are all orphans.  Finally, it adds the linking transaction and ensures
// the entire orphan chain is moved to the transaction pool.
func TestSimpleOrphanChain(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool.
		if !harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatal("IsOrphanInPool: false for accepted orphan")
		}

		// Ensure the transaction is not in the transaction pool.
		if harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatal("IsTransactionInPool: true for accepted orphan")
		}

		// Ensure the transaction is reported as available.
		if !harness.txPool.HaveTransaction(tx.Hash()) {
			t.Fatal("HaveTransaction: false for accepted orphan")
		}
	}

	// Add the transaction which completes the orphan chain and ensure they
	// all get accepted.  Notice the accept orphans flag is also false here
	// to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	acceptedTxns, err := harness.txPool.ProcessTransaction(chainedTxns[0],
		false, false)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid "+
			"orphan %v", err)
	}
	if len(acceptedTxns) != len(chainedTxns) {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), len(chainedTxns))
	}
	for _, tx := range acceptedTxns {
		// Ensure none of the transactions are still in the orphan pool.
		if harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatalf("IsOrphanInPool: true for accepted tx %v",
				tx.Hash())
		}

		// Ensure all of the transactions are now in the transaction
		// pool.
		if !harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatalf("IsTransactionInPool: false for accepted tx %v",
				tx.Hash())
		}
	}
}

// TestOrphanReject ensures that orphans are properly rejected when the allow
// orphans flag is not set on ProcessTransaction.
func TestOrphanReject(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure orphans are rejected when the allow orphans flag is not set.
	for _, tx := range chainedTxns[1:] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, false,
			false)
		if err == nil {
			t.Fatalf("ProcessTransaction: did not fail on orphan "+
				"%v when allow orphans flag is false", tx.Hash())
		}
		expectedErr := RuleError{}
		if reflect.TypeOf(err) != reflect.TypeOf(expectedErr) {
			t.Fatalf("ProcessTransaction: wrong error got: <%T> %v, "+
				"want: <%T>", err, err, expectedErr)
		}
		code, extracted := extractRejectCode(err)
		if !extracted {
			t.Fatalf("ProcessTransaction: failed to extract reject "+
				"code from error %q", err)
		}
		if code != wire.RejectDuplicate {
			t.Fatalf("ProcessTransaction: unexpected reject code "+
				"-- got %v, want %v", code, wire.RejectDuplicate)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatal("ProcessTransaction: reported %d accepted "+
				"transactions from failed orphan attempt",
				len(acceptedTxns))
		}

		// Ensure the transaction is not in the orphan pool.
		if harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatal("IsOrphanInPool: true for rejected orphan")
		}

		// Ensure the transaction is not in the transaction pool.
		if harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatal("IsTransactionInPool: true for rejected orphan")
		}

		// Ensure the transaction is not reported as available.
		if harness.txPool.HaveTransaction(tx.Hash()) {
			t.Fatal("HaveTransaction: true for rejected orphan")
		}
	}
}
