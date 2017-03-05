// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/txscript"
	"github.com/bitgo/rmgd/wire"
)

const (
	// MaxSigOpsPerBlock is the maximum number of signature operations
	// allowed for a block.  It is a fraction of the max block payload size.
	MaxSigOpsPerBlock = wire.MaxBlockPayload / 50

	// MaxTimeOffsetSeconds is the maximum number of seconds a block time
	// is allowed to be ahead of the current time.  This is currently 2
	// hours.
	MaxTimeOffsetSeconds = 2 * 60 * 60

	// MinCoinbaseScriptLen is the minimum length a coinbase script can be.
	MinCoinbaseScriptLen = 2

	// MaxCoinbaseScriptLen is the maximum length a coinbase script can be.
	MaxCoinbaseScriptLen = 100

	// medianTimeBlocks is the number of previous blocks which should be
	// used to calculate the median time used to validate block timestamps.
	medianTimeBlocks = 11

	// serializedHeightVersion is the block version which changed block
	// coinbases to start with the serialized block height.
	serializedHeightVersion = 2

	// baseSubsidy is the starting subsidy amount for mined blocks.  This
	// value is halved every SubsidyHalvingInterval blocks.
	baseSubsidy = 5000 * rmgutil.AtomsPerGram

	// MaxAdminKeySetSize sets a limit for the size of admin key sets.
	// When admin transactions are validated, the pubKeyScript is generated
	// from all active keys of that thread. The limit is needed to not exceed
	// pubKeyScript size limits.
	MaxAdminKeySetSize = 42

	// MinValidateKeySetSize is the least amount of validators needed to run
	// the chain. Rate Limiting for validators should not conflict with this.
	MinValidateKeySetSize = 4
)

var (
	// zeroHash is the zero value for a chainhash.Hash and is defined as
	// a package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash = &chainhash.Hash{}
)

// isNullOutpoint determines whether or not a previous transaction output point
// is set.
func isNullOutpoint(outpoint *wire.OutPoint) bool {
	if outpoint.Index == math.MaxUint32 && outpoint.Hash.IsEqual(zeroHash) {
		return true
	}
	return false
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBase in that it works with a raw wire
// transaction as opposed to a higher level util transaction.
func IsCoinBaseTx(msgTx *wire.MsgTx) bool {
	// A coin base must only have one transaction input.
	if len(msgTx.TxIn) != 1 {
		return false
	}

	// The previous output of a coin base must have a max value index and
	// a zero hash.
	prevOut := &msgTx.TxIn[0].PreviousOutPoint
	if !isNullOutpoint(prevOut) {
		return false
	}

	return true
}

// IsCoinBase determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBaseTx in that it works with a higher
// level util transaction as opposed to a raw wire transaction.
func IsCoinBase(tx *rmgutil.Tx) bool {
	return IsCoinBaseTx(tx.MsgTx())
}

// IsFinalizedTransaction determines whether or not a transaction is finalized.
func IsFinalizedTransaction(tx *rmgutil.Tx, blockHeight uint32, blockTime time.Time) bool {
	msgTx := tx.MsgTx()

	// Lock time of zero means the transaction is finalized.
	lockTime := msgTx.LockTime
	if lockTime == 0 {
		return true
	}

	// The lock time field of a transaction is either a block height at
	// which the transaction is finalized or a timestamp depending on if the
	// value is before the txscript.LockTimeThreshold.  When it is under the
	// threshold it is a block height.
	blockTimeOrHeight := int64(0)
	if lockTime < txscript.LockTimeThreshold {
		blockTimeOrHeight = int64(blockHeight)
	} else {
		blockTimeOrHeight = blockTime.Unix()
	}
	if int64(lockTime) < blockTimeOrHeight {
		return true
	}

	// At this point, the transaction's lock time hasn't occurred yet, but
	// the transaction might still be finalized if the sequence number
	// for all transaction inputs is maxed out.
	for _, txIn := range msgTx.TxIn {
		if txIn.Sequence != math.MaxUint32 {
			return false
		}
	}
	return true
}

// CalcBlockSubsidy returns the subsidy amount a block at the provided height
// should have. This is mainly used for determining how much the coinbase for
// newly generated blocks awards as well as validating the coinbase for blocks
// has the expected value.
//
// The subsidy is halved every SubsidyReductionInterval blocks.  Mathematically
// this is: baseSubsidy / 2^(height/SubsidyReductionInterval)
//
// At the target block generation rate for the main network, this is
// approximately every 4 years.
func CalcBlockSubsidy(height uint32, chainParams *chaincfg.Params) int64 {
	if chainParams.SubsidyReductionInterval == 0 {
		return baseSubsidy
	}

	// Equivalent to: baseSubsidy / 2^(height/subsidyHalvingInterval)
	return baseSubsidy >> uint(height/chainParams.SubsidyReductionInterval)
}

// CheckTransactionSanity performs some preliminary checks on a transaction to
// ensure it is sane.  These checks are context free.
// TODO(prova): Notice that this code is a dupclicate of transaction
// validation code in checkTransactionStandard() of policy.go
// TODO(prova): extract functionality into admin tx validator.
func CheckTransactionSanity(tx *rmgutil.Tx) error {
	// A transaction must have at least one input.
	msgTx := tx.MsgTx()
	if len(msgTx.TxIn) == 0 {
		return ruleError(ErrNoTxInputs, "transaction has no inputs")
	}

	// A transaction must have at least one output.
	if len(msgTx.TxOut) == 0 {
		return ruleError(ErrNoTxOutputs, "transaction has no outputs")
	}

	// A transaction must not exceed the maximum allowed block payload when
	// serialized.
	serializedTxSize := tx.MsgTx().SerializeSize()
	if serializedTxSize > wire.MaxBlockPayload {
		str := fmt.Sprintf("serialized transaction is too big - got "+
			"%d, max %d", serializedTxSize, wire.MaxBlockPayload)
		return ruleError(ErrTxTooBig, str)
	}

	// Ensure the transaction amounts are in range.  Each transaction
	// output must not be negative or more than the max allowed per
	// transaction.  Also, the total of all outputs must abide by the same
	// restrictions.  All amounts in a transaction are in a unit value known
	// as an atom.  One gram is a quantity of atoms as defined by the
	// AtomsPerGram constant.
	var totalAtoms int64
	threadInt, adminOutputs := txscript.GetAdminDetails(tx)
	hasAdminOut := (threadInt >= 0)
	for txOutIndex, txOut := range msgTx.TxOut {
		atoms := txOut.Value
		if atoms < 0 {
			str := fmt.Sprintf("transaction output has negative "+
				"value of %v", atoms)
			return ruleError(ErrBadTxOutValue, str)
		}
		if atoms > rmgutil.MaxAtoms {
			str := fmt.Sprintf("transaction output value of %v is "+
				"higher than max allowed value of %v", atoms,
				rmgutil.MaxAtoms)
			return ruleError(ErrBadTxOutValue, str)
		}

		// Two's complement int64 overflow guarantees that any overflow
		// is detected and reported.  This is impossible for Bitcoin, but
		// perhaps possible if an alt increases the total money supply.
		totalAtoms += atoms
		if totalAtoms < 0 {
			str := fmt.Sprintf("total value of all transaction "+
				"outputs exceeds max allowed value of %v",
				rmgutil.MaxAtoms)
			return ruleError(ErrBadTxOutValue, str)
		}
		if totalAtoms > rmgutil.MaxAtoms {
			str := fmt.Sprintf("total value of all transaction "+
				"outputs is %v which is higher than max "+
				"allowed value of %v", totalAtoms,
				rmgutil.MaxAtoms)
			return ruleError(ErrBadTxOutValue, str)
		}

		// Only first output can be admin output
		scriptClass := txscript.GetScriptClass(txOut.PkScript)
		if scriptClass == txscript.ProvaAdminTy {
			if txOutIndex != 0 {
				str := fmt.Sprintf("transaction output %d: admin output "+
					"only allowed at position 0.", txOutIndex)
				return ruleError(ErrInvalidAdminTx, str)
			}
		}

		if hasAdminOut {
			if rmgutil.ThreadID(threadInt) != rmgutil.IssueThread {
				// All Admin tx output values must be 0 value
				if txOut.Value != 0 {
					str := fmt.Sprintf("admin transaction with non-zero value "+
						"output #%d.", txOutIndex)
					return ruleError(ErrInvalidAdminTx, str)
				}
			} else {
				// take care of issue thread
				// If issuance/destruction tx, any non-nulldata outputs must be valid Prova scripts
				isDestruction := len(msgTx.TxIn) > 1
				if txOutIndex > 0 {
					pops := adminOutputs[txOutIndex-1]
					scriptType := txscript.TypeOfScript(pops)
					if len(pops) == 1 {
						if !isDestruction {
							str := fmt.Sprintf("issue thread transaction %v "+
								"tries to issue and destroy at the same "+
								"time.", tx.Hash)
							return ruleError(ErrInvalidAdminTx, str)
						}
						if scriptType != txscript.NullDataTy {
							str := fmt.Sprintf("admin issue transaction %v "+
								"has invalid output #%d.", tx.Hash, txOutIndex)
							return ruleError(ErrInvalidAdminTx, str)
						} else {
							if atoms == 0 {
								str := fmt.Sprintf("admin issue transaction %v "+
									"trying to destroy 0 at output "+
									"#%d.", tx.Hash, txOutIndex)
								return ruleError(ErrInvalidAdminTx, str)
							}
						}
					} else {
						if scriptType != txscript.ProvaTy &&
							scriptType != txscript.GeneralProvaTy {
							str := fmt.Sprintf("admin issue transaction %v "+
								"expected to have prova output at %d, "+
								"but found %x.", tx.Hash, txOutIndex, pops)
							return ruleError(ErrInvalidAdminTx, str)
						}
						if atoms == 0 {
							str := fmt.Sprintf("admin issue transaction %v "+
								"trying to issue 0 at output "+
								"#%d.", tx.Hash, txOutIndex)
							return ruleError(ErrInvalidAdminTx, str)
						}
					}
				}
			}
		}
	}

	// Check for duplicate transaction inputs.
	existingTxOut := make(map[wire.OutPoint]struct{})
	for _, txIn := range msgTx.TxIn {
		if _, exists := existingTxOut[txIn.PreviousOutPoint]; exists {
			return ruleError(ErrDuplicateTxInputs, "transaction "+
				"contains duplicate inputs")
		}
		existingTxOut[txIn.PreviousOutPoint] = struct{}{}
	}

	// Coinbase script length must be between min and max length.
	if IsCoinBase(tx) {
		// Coinbase tx must be a standard prova tx
		if !txscript.IsProvaTx(tx) {
			// TODO(prova): fix the blockchain tests
			return ruleError(ErrInvalidCoinbase, "coinbase transaction is not of an allowed form")
		}
		slen := len(msgTx.TxIn[0].SignatureScript)
		if slen < MinCoinbaseScriptLen || slen > MaxCoinbaseScriptLen {
			str := fmt.Sprintf("coinbase transaction script length "+
				"of %d is out of range (min: %d, max: %d)",
				slen, MinCoinbaseScriptLen, MaxCoinbaseScriptLen)
			return ruleError(ErrBadCoinbaseScriptLen, str)
		}
		return nil
	}

	// Check admin transaction on ROOT and PROVISION thread
	// TODO(prova): Notice that this code is a dupclicate of transaction
	// validation code in checkTransactionStandard() of policy.go
	// TODO(prova): extract functionality into admin tx validator.
	if hasAdminOut {
		threadId := rmgutil.ThreadID(threadInt)
		if threadId == rmgutil.RootThread || threadId == rmgutil.ProvisionThread {
			// Admin tx may not have any other inputs
			if len(msgTx.TxIn) > 1 {
				str := fmt.Sprintf("admin transaction with more than 1 input.")
				return ruleError(ErrInvalidAdminTx, str)
			}
			// Admin tx must have at least 2 outputs
			if len(msgTx.TxOut) < 2 {
				str := fmt.Sprintf("admin transaction with no admin operations.")
				return ruleError(ErrInvalidAdminTx, str)
			}
			//

			// op pkscript
			for _, adminOpOut := range adminOutputs {
				// check conditions for admin ops
				// - Admin tx additional outputs must be nulldata scripts
				// - Key in nulldata script must be valid
				// - Data in nulldata scripts must match proper form expected for
				//   the thread
				if !txscript.IsValidAdminOp(adminOpOut, threadId) {
					str := fmt.Sprintf("admin transaction with invalid admin " +
						"operation found.")
					return ruleError(ErrInvalidAdminTx, str)
				}
			}
		}
	}

	if !(threadInt >= 0) && !txscript.IsProvaTx(tx) {
		// TODO(prova): fix the blockchain tests
		return ruleError(ErrInvalidTx, "transaction is not of an allowed form")
	}

	// Previous transaction outputs referenced by the inputs to this
	// transaction must not be null.
	for _, txIn := range msgTx.TxIn {
		prevOut := &txIn.PreviousOutPoint
		if isNullOutpoint(prevOut) {
			return ruleError(ErrBadTxInput, "transaction "+
				"input refers to previous output that "+
				"is null")
		}
	}

	return nil
}

// checkProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
//
// The flags modify the behavior of this function as follows:
//  - BFNoPoWCheck: The check to ensure the block hash is less than the target
//    difficulty is not performed.
func checkProofOfWork(header *wire.BlockHeader, powLimit *big.Int, flags BehaviorFlags) error {
	// The target difficulty must be larger than zero.
	target := CompactToBig(header.Bits)
	if target.Sign() <= 0 {
		str := fmt.Sprintf("block target difficulty of %064x is too low",
			target)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The target difficulty must be less than the maximum allowed.
	if target.Cmp(powLimit) > 0 {
		str := fmt.Sprintf("block target difficulty of %064x is "+
			"higher than max of %064x", target, powLimit)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The block hash must be less than the claimed target unless the flag
	// to avoid proof of work checks is set.
	if flags&BFNoPoWCheck != BFNoPoWCheck && header.Height >= 1 {
		// The block hash must be less than the claimed target.
		hash := header.BlockHash()
		hashNum := HashToBig(&hash)
		if hashNum.Cmp(target) > 0 {
			str := fmt.Sprintf("block hash of %064x is higher than "+
				"expected max of %064x", hashNum, target)
			return ruleError(ErrHighHash, str)
		}
	}

	return nil
}

// CheckProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
func CheckProofOfWork(block *rmgutil.Block, powLimit *big.Int) error {
	return checkProofOfWork(&block.MsgBlock().Header, powLimit, BFNone)
}

// CountSigOps returns the number of signature operations for all transaction
// input and output scripts in the provided transaction.  This uses the
// quicker, but imprecise, signature operation counting mechanism from
// txscript.
func CountSigOps(tx *rmgutil.Tx) int {
	msgTx := tx.MsgTx()

	// Accumulate the number of signature operations in all transaction
	// inputs.
	totalSigOps := 0
	for _, txIn := range msgTx.TxIn {
		numSigOps := txscript.GetSigOpCount(txIn.SignatureScript)
		totalSigOps += numSigOps
	}

	// Accumulate the number of signature operations in all transaction
	// outputs.
	for _, txOut := range msgTx.TxOut {
		numSigOps := txscript.GetSigOpCount(txOut.PkScript)
		totalSigOps += numSigOps
	}

	return totalSigOps
}

// CountP2SHSigOps returns the number of signature operations for all input
// transactions which are of the pay-to-script-hash type.  This uses the
// precise, signature operation counting mechanism from the script engine which
// requires access to the input transaction scripts.
func CountP2SHSigOps(tx *rmgutil.Tx, isCoinBaseTx bool, utxoView *UtxoViewpoint) (int, error) {
	// Coinbase transactions have no interesting inputs.
	if isCoinBaseTx {
		return 0, nil
	}

	// Accumulate the number of signature operations in all transaction
	// inputs.
	msgTx := tx.MsgTx()
	totalSigOps := 0
	for txInIndex, txIn := range msgTx.TxIn {
		// Ensure the referenced input transaction is available.
		originTxHash := &txIn.PreviousOutPoint.Hash
		originTxIndex := txIn.PreviousOutPoint.Index
		txEntry := utxoView.LookupEntry(originTxHash)
		if txEntry == nil || txEntry.IsOutputSpent(originTxIndex) {
			str := fmt.Sprintf("unable to find unspent output "+
				"%v referenced from transaction %s:%d",
				txIn.PreviousOutPoint, tx.Hash(), txInIndex)
			return 0, ruleError(ErrMissingTx, str)
		}

		// We're only interested in pay-to-script-hash types, so skip
		// this input if it's not one.
		pkScript := txEntry.PkScriptByIndex(originTxIndex)
		if !txscript.IsPayToScriptHash(pkScript) {
			continue
		}

		// Count the precise number of signature operations in the
		// referenced public key script.
		sigScript := txIn.SignatureScript
		numSigOps := txscript.GetPreciseSigOpCount(sigScript, pkScript,
			true)

		// We could potentially overflow the accumulator so check for
		// overflow.
		lastSigOps := totalSigOps
		totalSigOps += numSigOps
		if totalSigOps < lastSigOps {
			str := fmt.Sprintf("the public key script from output "+
				"%v contains too many signature operations - "+
				"overflow", txIn.PreviousOutPoint)
			return 0, ruleError(ErrTooManySigOps, str)
		}
	}

	return totalSigOps, nil
}

// checkBlockHeaderSanity performs some preliminary checks on a block header to
// ensure it is sane before continuing with processing.  These checks are
// context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkProofOfWork.
func checkBlockHeaderSanity(header *wire.BlockHeader, powLimit *big.Int, timeSource MedianTimeSource, flags BehaviorFlags) error {
	// Ensure the proof of work bits in the block header is in min/max range
	// and the block hash is less than the target value described by the
	// bits.
	err := checkProofOfWork(header, powLimit, flags)
	if err != nil {
		return err
	}

	// A block timestamp must not have a greater precision than one second.
	// This check is necessary because Go time.Time values support
	// nanosecond precision whereas the consensus rules only apply to
	// seconds and it's much nicer to deal with standard Go time values
	// instead of converting to seconds everywhere.
	if !header.Timestamp.Equal(time.Unix(header.Timestamp.Unix(), 0)) {
		str := fmt.Sprintf("block timestamp of %v has a higher "+
			"precision than one second", header.Timestamp)
		return ruleError(ErrInvalidTime, str)
	}

	// Ensure the block time is not too far in the future.
	//TODO(prova) fix test
	// maxTimestamp := timeSource.AdjustedTime().Add(time.Second *
	// 	MaxTimeOffsetSeconds)
	// if header.Timestamp.After(maxTimestamp) {
	// 	str := fmt.Sprintf("block timestamp of %v is too far in the "+
	// 		"future", header.Timestamp)
	// 	return ruleError(ErrTimeTooNew, str)
	// }

	return nil
}

// checkBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockHeaderSanity.
func checkBlockSanity(block *rmgutil.Block, powLimit *big.Int, timeSource MedianTimeSource, flags BehaviorFlags) error {
	msgBlock := block.MsgBlock()
	header := &msgBlock.Header
	err := checkBlockHeaderSanity(header, powLimit, timeSource, flags)
	if err != nil {
		return err
	}

	// A block must have at least one transaction.
	numTx := len(msgBlock.Transactions)
	if numTx == 0 {
		return ruleError(ErrNoTransactions, "block does not contain "+
			"any transactions")
	}

	// A block must not have more transactions than the max block payload.
	if numTx > wire.MaxBlockPayload {
		str := fmt.Sprintf("block contains too many transactions - "+
			"got %d, max %d", numTx, wire.MaxBlockPayload)
		return ruleError(ErrTooManyTransactions, str)
	}

	// A block must not exceed the maximum allowed block payload when
	// serialized.  The serialized size must match the header size value.
	serializedSize := msgBlock.SerializeSize()
	if serializedSize != int(header.Size) {
		str := fmt.Sprintf("serialized block size %d, does not match "+
			"header size %d", serializedSize, header.Size)
		return ruleError(ErrInconsistentBlkSize, str)
	}
	if serializedSize > wire.MaxBlockPayload {
		str := fmt.Sprintf("serialized block is too big - got %d, "+
			"max %d", serializedSize, wire.MaxBlockPayload)
		return ruleError(ErrBlockTooBig, str)
	}

	// The first transaction in a block must be a coinbase.
	transactions := block.Transactions()
	if !IsCoinBase(transactions[0]) {
		return ruleError(ErrFirstTxNotCoinbase, "first transaction in "+
			"block is not a coinbase")
	}

	// A block must not have more than one coinbase.
	for i, tx := range transactions[1:] {
		if IsCoinBase(tx) {
			str := fmt.Sprintf("block contains second coinbase at "+
				"index %d", i+1)
			return ruleError(ErrMultipleCoinbases, str)
		}
	}

	// Do some preliminary checks on each transaction to ensure they are
	// sane before continuing.
	for _, tx := range transactions {
		err := CheckTransactionSanity(tx)
		if err != nil {
			return err
		}
	}

	// Build merkle tree and ensure the calculated merkle root matches the
	// entry in the block header.  This also has the effect of caching all
	// of the transaction hashes in the block to speed up future hash
	// checks.  Bitcoind builds the tree here and checks the merkle root
	// after the following checks, but there is no reason not to check the
	// merkle root matches here.
	merkles := BuildMerkleTreeStore(block.Transactions())
	calculatedMerkleRoot := merkles[len(merkles)-1]
	if !header.MerkleRoot.IsEqual(calculatedMerkleRoot) {
		str := fmt.Sprintf("block merkle root is invalid - block "+
			"header indicates %v, but calculated value is %v",
			header.MerkleRoot, calculatedMerkleRoot)
		return ruleError(ErrBadMerkleRoot, str)
	}

	// Check for duplicate transactions.  This check will be fairly quick
	// since the transaction hashes are already cached due to building the
	// merkle tree above.
	existingTxHashes := make(map[chainhash.Hash]struct{})
	for _, tx := range transactions {
		hash := tx.Hash()
		if _, exists := existingTxHashes[*hash]; exists {
			str := fmt.Sprintf("block contains duplicate "+
				"transaction %v", hash)
			return ruleError(ErrDuplicateTx, str)
		}
		existingTxHashes[*hash] = struct{}{}
	}

	// The number of signature operations must be less than the maximum
	// allowed per block.
	totalSigOps := 0
	for _, tx := range transactions {
		// We could potentially overflow the accumulator so check for
		// overflow.
		lastSigOps := totalSigOps
		totalSigOps += CountSigOps(tx)
		if totalSigOps < lastSigOps || totalSigOps > MaxSigOpsPerBlock {
			str := fmt.Sprintf("block contains too many signature "+
				"operations - got %v, max %v", totalSigOps,
				MaxSigOpsPerBlock)
			return ruleError(ErrTooManySigOps, str)
		}
	}

	return nil
}

// CheckBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context free.
func CheckBlockSanity(block *rmgutil.Block, powLimit *big.Int, timeSource MedianTimeSource) error {
	return checkBlockSanity(block, powLimit, timeSource, BFNone)
}

// checkBlockHeaderContext peforms several validation checks on the block header
// which depend on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: All checks except those involving comparing the header against
//    the checkpoints are not performed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkBlockHeaderContext(header *wire.BlockHeader, prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// Ensure the difficulty specified in the block header matches
		// the calculated difficulty based on the previous block and
		// difficulty retarget rules.
		expectedDifficulty, err := b.calcNextRequiredDifficulty(prevNode)
		if err != nil {
			return err
		}
		blockDifficulty := header.Bits
		if blockDifficulty != expectedDifficulty {
			str := "block difficulty of %d is not the expected value of %d"
			str = fmt.Sprintf(str, blockDifficulty, expectedDifficulty)
			return ruleError(ErrUnexpectedDifficulty, str)
		}

		// Ensure the timestamp for the block header is after the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime, err := b.calcPastMedianTime(prevNode)
		if err != nil {
			log.Errorf("calcPastMedianTime: %v", err)
			return err
		}
		if !header.Timestamp.After(medianTime) {
			str := "block timestamp of %v is not after expected %v"
			str = fmt.Sprintf(str, header.Timestamp, medianTime)
			return ruleError(ErrTimeTooOld, str)
		}

		// Verify the block's signature by an active validate key.
		// TODO(prova): confirm that the validating pubkey is valid
		pubKey, err := btcec.ParsePubKey(header.ValidatingPubKey[:], btcec.S256())
		if err != nil {
			return err
		}
		if !header.Verify(pubKey) {
			return ruleError(ErrBadBlockSignature, "unable to validate block signature")
		}
	}

	// The height of this block is one more than the referenced previous
	// block. The header value for Height must be correct.
	blockHeight := prevNode.height + 1
	if header.Height != blockHeight {
		str := "block height of %d is not the expected value of %d"
		str = fmt.Sprintf(str, header.Height, blockHeight)
		return ruleError(ErrBadHeight, str)
	}

	// Ensure chain matches up to predetermined checkpoints.
	blockHash := header.BlockHash()
	if !b.verifyCheckpoint(blockHeight, &blockHash) {
		str := fmt.Sprintf("block at height %d does not match "+
			"checkpoint hash", blockHeight)
		return ruleError(ErrBadCheckpoint, str)
	}

	// Find the previous checkpoint and prevent blocks which fork the main
	// chain before it.  This prevents storage of new, otherwise valid,
	// blocks which build off of old blocks that are likely at a much easier
	// difficulty and therefore could be used to waste cache and disk space.
	checkpointBlock, err := b.findPreviousCheckpoint()
	if err != nil {
		return err
	}
	if checkpointBlock != nil && blockHeight < checkpointBlock.Height() {
		str := fmt.Sprintf("block at height %d forks the main chain "+
			"before the previous checkpoint at height %d",
			blockHeight, checkpointBlock.Height())
		return ruleError(ErrForkTooOld, str)
	}

	// TODO(prova): clean up / remove
	if !fastAdd {
		// Reject version 3 blocks once a majority of the network has
		// upgraded.  This is part of BIP0065.
		if header.Version < 4 && b.isMajorityVersion(4, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 2 blocks once a majority of the network has
		// upgraded.  This is part of BIP0066.
		if header.Version < 3 && b.isMajorityVersion(3, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 1 blocks once a majority of the network has
		// upgraded.  This is part of BIP0034.
		if header.Version < 2 && b.isMajorityVersion(2, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}
	}

	return nil
}

// checkBlockContext peforms several validation checks on the block which depend
// on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: The transaction are not checked to see if they are finalized
//    and the somewhat expensive BIP0034 validation is not performed.
//
// The flags are also passed to checkBlockHeaderContext.  See its documentation
// for how the flags modify its behavior.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkBlockContext(block *rmgutil.Block, prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	err := b.checkBlockHeaderContext(header, prevNode, flags)
	if err != nil {
		return err
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// The height of this block is one more than the referenced
		// previous block.
		blockHeight := prevNode.height + 1

		// Ensure all transactions in the block are finalized.
		for _, tx := range block.Transactions() {
			if !IsFinalizedTransaction(tx, blockHeight,
				header.Timestamp) {

				str := fmt.Sprintf("block contains unfinalized "+
					"transaction %v", tx.Hash())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}
	}

	return nil
}

// checkBIP0030 ensures blocks do not contain duplicate transactions which
// 'overwrite' older transactions that are not fully spent.  This prevents an
// attack where a coinbase and all of its dependent transactions could be
// duplicated to effectively revert the overwritten transactions to a single
// confirmation thereby making them vulnerable to a double spend.
//
// For more details, see https://en.bitcoin.it/wiki/BIP_0030 and
// http://r6.ca/blog/20120206T005236Z.html.
//
// This function MUST be called with the chain state lock held (for reads).
func (b *BlockChain) checkBIP0030(node *blockNode, block *rmgutil.Block, view *UtxoViewpoint) error {
	// Fetch utxo details for all of the transactions in this block.
	// Typically, there will not be any utxos for any of the transactions.
	fetchSet := make(map[chainhash.Hash]struct{})
	for _, tx := range block.Transactions() {
		fetchSet[*tx.Hash()] = struct{}{}
	}
	err := view.fetchUtxos(b.db, fetchSet)
	if err != nil {
		return err
	}

	// Duplicate transactions are only allowed if the previous transaction
	// is fully spent.
	for _, tx := range block.Transactions() {
		txEntry := view.LookupEntry(tx.Hash())
		if txEntry != nil && !txEntry.IsFullySpent() {
			str := fmt.Sprintf("tried to overwrite transaction %v "+
				"at block height %d that is not fully spent",
				tx.Hash(), txEntry.blockHeight)
			return ruleError(ErrOverwriteTx, str)
		}
	}

	return nil
}

// CheckTransactionInputs performs a series of checks on the inputs to a
// transaction to ensure they are valid.  An example of some of the checks
// include verifying all inputs exist, ensuring the coinbase seasoning
// requirements are met, detecting double spends, validating all values and fees
// are in the legal range and the total output amount doesn't exceed the input
// amount, and verifying the signatures to prove the spender was the owner of
// the funds and therefore allowed to spend them.  As it checks the inputs,
// it also calculates the total fees for the transaction and returns that value.
//
// NOTE: The transaction MUST have already been sanity checked with the
// CheckTransactionSanity function prior to calling this function.
func CheckTransactionInputs(tx *rmgutil.Tx, txHeight uint32, utxoView *UtxoViewpoint, chainParams *chaincfg.Params) (int64, error) {
	// Coinbase transactions have no inputs.
	if IsCoinBase(tx) {
		return 0, nil
	}

	txHash := tx.Hash()
	var totalAtomsIn int64
	threadInt, _ := txscript.GetAdminDetails(tx)
	hasAdminOut := (threadInt >= 0)
	hasAdminIn := false
	for txInIndex, txIn := range tx.MsgTx().TxIn {
		// Ensure the referenced input transaction is available.
		originTxHash := &txIn.PreviousOutPoint.Hash
		utxoEntry := utxoView.LookupEntry(originTxHash)
		if utxoEntry == nil {
			str := fmt.Sprintf("unable to find unspent output "+
				"%v referenced from transaction %s:%d",
				txIn.PreviousOutPoint, tx.Hash(), txInIndex)
			return 0, ruleError(ErrMissingTx, str)
		}

		// Ensure admin thread tips are only spendable by same type admin
		// transactions
		originPkScript := utxoEntry.PkScriptByIndex(txIn.PreviousOutPoint.Index)
		thisPkScript := tx.MsgTx().TxOut[0].PkScript
		if txscript.GetScriptClass(originPkScript) == txscript.ProvaAdminTy {
			if txInIndex != 0 {
				str := fmt.Sprintf("transaction %v tried to spend admin "+
					"thread transaction %v with input at position "+
					"%d. Only input #0 may spend an admin threads.",
					tx.Hash(), originTxHash, txInIndex)
				return 0, ruleError(ErrInvalidAdminTx, str)
			}
			if !hasAdminOut {
				str := fmt.Sprintf("transaction %v spends admin output, "+
					"yet does not continue admin thread. Should have admin "+
					"output at position 0.", tx.Hash())
				return 0, ruleError(ErrInvalidAdminTx, str)
			}
			hasAdminIn = true
			if thisPkScript[0] != originPkScript[0] ||
				thisPkScript[1] != originPkScript[1] {
				str := fmt.Sprintf("admin transaction input %v is "+
					"spending wrong thread.", tx.Hash())
				return 0, ruleError(ErrInvalidAdminTx, str)
			}
		}

		// If current transaction has admin output, but doesn't spend
		// an admin thread, it is not valid
		if hasAdminOut && !hasAdminIn {
			str := fmt.Sprintf("tried to issue admin operation "+
				"at transaction %s:%d without spending valid thread.",
				tx.Hash(), txInIndex)
			return 0, ruleError(ErrInvalidAdminTx, str)
		}

		// Ensure the transaction is not spending coins which have not
		// yet reached the required coinbase maturity.
		if utxoEntry.IsCoinBase() {
			originHeight := utxoEntry.BlockHeight()
			blocksSincePrev := txHeight - originHeight
			coinbaseMaturity := uint32(chainParams.CoinbaseMaturity)
			if blocksSincePrev < coinbaseMaturity {
				str := fmt.Sprintf("tried to spend coinbase "+
					"transaction %v from height %v at "+
					"height %v before required maturity "+
					"of %v blocks", originTxHash,
					originHeight, txHeight,
					coinbaseMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// Ensure the transaction is not double spending coins.
		originTxIndex := txIn.PreviousOutPoint.Index
		if utxoEntry.IsOutputSpent(originTxIndex) {
			str := fmt.Sprintf("transaction %s:%d tried to double "+
				"spend output %v", txHash, txInIndex,
				txIn.PreviousOutPoint)
			return 0, ruleError(ErrDoubleSpend, str)
		}

		// Ensure the transaction amounts are in range.  Each of the
		// output values of the input transactions must not be negative
		// or more than the max allowed per transaction.  All amounts in
		// a transaction are in a unit value known as an atom.  One
		// gram is a quantity of atoms as defined by the
		// AtomsPerGram constant.
		originTxAtoms := utxoEntry.AmountByIndex(originTxIndex)
		if originTxAtoms < 0 {
			str := fmt.Sprintf("transaction output has negative "+
				"value of %v", rmgutil.Amount(originTxAtoms))
			return 0, ruleError(ErrBadTxOutValue, str)
		}
		if originTxAtoms > rmgutil.MaxAtoms {
			str := fmt.Sprintf("transaction output value of %v is "+
				"higher than max allowed value of %v",
				rmgutil.Amount(originTxAtoms),
				rmgutil.MaxAtoms)
			return 0, ruleError(ErrBadTxOutValue, str)
		}

		// The total of all outputs must not be more than the max
		// allowed per transaction.  Also, we could potentially overflow
		// the accumulator so check for overflow.
		lastAtomsIn := totalAtomsIn
		totalAtomsIn += originTxAtoms
		if totalAtomsIn < lastAtomsIn ||
			totalAtomsIn > rmgutil.MaxAtoms {
			str := fmt.Sprintf("total value of all transaction "+
				"inputs is %v which is higher than max "+
				"allowed value of %v", totalAtomsIn,
				rmgutil.MaxAtoms)
			return 0, ruleError(ErrBadTxOutValue, str)
		}
	}

	// Calculate the total output amount for this transaction.  It is safe
	// to ignore overflow and out of range errors here because those error
	// conditions would have already been caught by checkTransactionSanity.
	var totalAtomsOut int64
	for _, txOut := range tx.MsgTx().TxOut {
		totalAtomsOut += txOut.Value
	}

	isIssueThread := false
	if hasAdminOut {
		threadId := rmgutil.ThreadID(threadInt)
		if threadId == rmgutil.IssueThread {
			isIssueThread = true // we should make exception for in/out check
		}
	}
	// Ensure the transaction does not spend more than its inputs.
	if totalAtomsIn < totalAtomsOut {
		if isIssueThread {
			// To be able to issue tokens, the out <= in rule is lifted for
			// issue thread transactions.

			// Yet, we need to make sure the transaction doesn't destroy more than
			// it's inputs, otherwise totalSupply calculation will be faulty.

			// Calculate the total destroyed amount for this transaction.  It is
			// safe to ignore overflow and out of range errors here because those
			// error conditions would have already been caught by
			// checkTransactionSanity.
			var totalAtomsDestroyed int64
			for _, txOut := range tx.MsgTx().TxOut {
				pops, _ := txscript.ParseScript(txOut.PkScript)
				if txscript.TypeOfScript(pops) == txscript.NullDataTy {
					totalAtomsDestroyed += txOut.Value
				}
			}
			if totalAtomsIn < totalAtomsDestroyed {
				str := fmt.Sprintf("admin transaction %v is trying to destroy "+
					"%v which is more than inputs %v.", txHash,
					totalAtomsDestroyed, totalAtomsIn)
				return 0, ruleError(ErrInvalidAdminTx, str)
			}
		} else {
			str := fmt.Sprintf("total value of all transaction inputs for "+
				"transaction %v is %v which is less than the amount "+
				"spent of %v", txHash, totalAtomsIn, totalAtomsOut)
			return 0, ruleError(ErrSpendTooHigh, str)
		}
	}

	// NOTE: bitcoind checks if the transaction fees are < 0 here, but that
	// is an impossible condition because of the check above that ensures
	// the inputs are >= the outputs.
	txFeeInAtoms := totalAtomsIn - totalAtomsOut
	// For issue thread admin transaction txFeeInAtoms can become negative.
	// We catch here:
	if isIssueThread && txFeeInAtoms < 0 {
		txFeeInAtoms = 0
	}
	return txFeeInAtoms, nil
}

// CheckProvaOutput checks that all keyIDs in the pkScript are known in
// the chain state.
//
// NOTE: The passed output MUST have already been sanity checked with the
// CheckTransactionSanity function prior to calling this function.
func CheckProvaOutput(tx *rmgutil.Tx, txOutIndex int, keyIDs []btcec.KeyID,
	keyView *KeyViewpoint) error {
	for _, keyID := range keyIDs {
		if keyView.aspKeyIdMap[keyID] == nil {
			str := fmt.Sprintf("transaction %v output %v has unknown "+
				"keyID %v.", tx.Hash(), txOutIndex, keyID)
			return ruleError(ErrInvalidTx, str)
		}
	}
	return nil
}

// CheckTransactionOutputs performs a series of checks on the outputs to ensure
// that they are valid in the context of the chain state.
//
// NOTE: The transaction MUST have already been sanity checked with the
// CheckTransactionSanity function prior to calling this function.
func CheckTransactionOutputs(tx *rmgutil.Tx, keyView *KeyViewpoint) error {
	threadInt, adminOutputs := txscript.GetAdminDetails(tx)
	hasAdminOut := (threadInt >= 0)
	if !hasAdminOut {
		// This is not an admin transaction, all outputs should be prova type
		// spending to active keyIDs
		for i, txOut := range tx.MsgTx().TxOut {
			output, _ := txscript.ParseScript(txOut.PkScript)
			keyIDs, err := txscript.ExtractKeyIDs(output)
			if err != nil {
				return ruleError(ErrInvalidTx, fmt.Sprintf("%v", err))
			}
			err = CheckProvaOutput(tx, i, keyIDs, keyView)
			if err != nil {
				return err
			}
		}
		return nil
	}
	threadId := rmgutil.ThreadID(threadInt)
	if threadId == rmgutil.IssueThread {
		for i, output := range adminOutputs {
			if len(output) > 1 {
				keyIDs, err := txscript.ExtractKeyIDs(output)
				if err != nil {
					return ruleError(ErrInvalidTx, fmt.Sprintf("%v", err))
				}
				// +1 here, because first out was thread output,
				// which is not contained in adminOutputs.
				err = CheckProvaOutput(tx, i+1, keyIDs, keyView)
				if err != nil {
					return err
				}
			}
		}
		return nil
	}
	for i := 0; i < len(adminOutputs); i++ {
		isAddOp, keySetType, pubKey,
			keyID := txscript.ExtractAdminOpData(adminOutputs[i])
		if keySetType == btcec.ASPKeySet {
			// TODO(prova): check pubKey collisions
			// TODO(prova): check strictly increasing keyID
			if isAddOp {
				if keyView.aspKeyIdMap[keyID] != nil {
					str := fmt.Sprintf("keyID %v added in transaction %v "+
						"exists already in admin set. Operation "+
						"rejected.", keyID, tx.Hash())
					return ruleError(ErrInvalidAdminOp, str)
				}
				if keyID != keyView.LastKeyID()+1 {
					str := fmt.Sprintf("keyID %v added in transaction %v "+
						"rejected. should be %v ", keyID, tx.Hash(), keyView.LastKeyID()+1)
					return ruleError(ErrInvalidAdminOp, str)
				}
			} else {
				if keyView.aspKeyIdMap[keyID] == nil {
					str := fmt.Sprintf("keyID %v can not be revoked in "+
						"transaction %v. It does not exist in admin set.",
						keyID, tx.Hash())
					return ruleError(ErrInvalidAdminOp, str)
				}
			}
		} else {
			keySet := keyView.adminKeySets[keySetType]
			pos := keySet.Pos(pubKey)
			if isAddOp {
				if pos >= 0 {
					str := fmt.Sprintf("key added in transaction %v "+
						"exists already in admin set at position %v. "+
						"Operation rejected.", tx.Hash(), pos)
					return ruleError(ErrInvalidAdminOp, str)
				}
				if len(keySet) >= MaxAdminKeySetSize {
					str := fmt.Sprintf("admin transaction %v tries to add "+
						"key to admin key set. Yet the set has reached max "+
						"size %v.", tx.Hash(), len(keySet))
					return ruleError(ErrInvalidAdminOp, str)
				}
			} else {
				if pos == -1 {
					str := fmt.Sprintf("admin transaction %v tries to remove "+
						"non-existing key %v. ", tx.Hash(), pubKey)
					return ruleError(ErrInvalidAdminOp, str)
				}
				// minLen describes the min amount of active admin keys
				// to keep in a set. This seems only critical for root keys,
				minLen := 0 // but root key set is fixed.
				if keySetType == btcec.ValidateKeySet {
					minLen = MinValidateKeySetSize
				}
				if len(keySet) <= minLen {
					str := fmt.Sprintf("admin transaction %v tries to remove "+
						"key from admin key set with length 2. At least 2 keys "+
						"have to stay provisioned.", tx.Hash())
					return ruleError(ErrInvalidAdminOp, str)
				}
			}
		}
	}
	return nil
}

// IsValidateKeyRateLimited determines whether using a specific pubkey in a
// future possible chain extension would create a validate rate limit error.
func (b *BlockChain) IsValidateKeyRateLimited(validatePubKey wire.BlockValidatingPubKey) (error, bool) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()
	return b.isValidateKeyRateLimited(b.bestNode, validatePubKey, true)
}

// isValidateKeyRateLimited determines whether or not a rate limiting violation
// is present with a given validate key.
func (b *BlockChain) isValidateKeyRateLimited(node *blockNode, validatePubKey wire.BlockValidatingPubKey, prospectiveInclusion bool) (error, bool) {
	// Get the previous block generators to check rate limiting rules.
	iterNode := node
	prevPubKeys := []wire.BlockValidatingPubKey{}
	window := b.chainParams.PowAveragingWindow
	if prospectiveInclusion {
		prevPubKeys = append(prevPubKeys, validatePubKey)
		window -= 1
	}
	for i := 0; iterNode != nil && i < window; i++ {
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("getPrevNodeFromNode: %v", err)
			return err, false
		}
		if iterNode != nil {
			prevPubKeys = append(prevPubKeys, iterNode.validatingPubKey)
		}
	}
	// Check if there is a run of too many blocks from a generator.
	if IsGenerationTrailingRateLimited(validatePubKey, prevPubKeys, b.chainParams.ChainTrailingSigKeyIdLimit) {
		return nil, true
	}
	// Check if there are too many blocks in a window from a generator.
	if IsGenerationShareRateLimited(validatePubKey, prevPubKeys, b.chainParams.ChainWindowShareLimit) {
		return nil, true
	}
	return nil, false
}

// checkConnectBlock performs several checks to confirm connecting the passed
// block to the chain represented by the passed view does not violate any rules.
// In addition, the passed view is updated to spend all of the referenced
// outputs and add all of the new utxos created by block.  Thus, the view will
// represent the state of the chain as if the block were actually connected and
// consequently the best hash for the view is also updated to passed block.
//
// The CheckConnectBlock function makes use of this function to perform the
// bulk of its work.  The only difference is this function accepts a node which
// may or may not require reorganization to connect it to the main chain whereas
// CheckConnectBlock creates a new node which specifically connects to the end
// of the current main chain and then calls this function with that node.
//
// See the comments for CheckConnectBlock for some examples of the type of
// checks performed by this function.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkConnectBlock(node *blockNode, block *rmgutil.Block, utxoView *UtxoViewpoint, keyView *KeyViewpoint, stxos *[]spentTxOut) error {
	// If the side chain blocks end up in the database, a call to
	// CheckBlockSanity should be done here in case a previous version
	// allowed a block that is no longer valid.  However, since the
	// implementation only currently uses memory for the side chain blocks,
	// it isn't currently necessary.

	// The coinbase for the Genesis block is not spendable, so just return
	// an error now.
	if node.hash.IsEqual(b.chainParams.GenesisHash) {
		str := "the coinbase for the genesis block is not spendable"
		return ruleError(ErrMissingTx, str)
	}

	// Ensure the view is for the node being checked.
	if !utxoView.BestHash().IsEqual(node.parentHash) {
		return AssertError(fmt.Sprintf("inconsistent view when "+
			"checking block connection: best hash is %v instead "+
			"of expected %v", utxoView.BestHash(), node.hash))
	}

	// BIP0030 added a rule to prevent blocks which contain duplicate
	// transactions that 'overwrite' older transactions which are not fully
	// spent.  See the documentation for checkBIP0030 for more details.
	err := b.checkBIP0030(node, block, utxoView)
	if err != nil {
		return err
	}

	// Load all of the utxos referenced by the inputs for all transactions
	// in the block don't already exist in the utxo view from the database.
	//
	// These utxo entries are needed for verification of things such as
	// transaction inputs, counting pay-to-script-hashes, and scripts.
	err = utxoView.fetchInputUtxos(b.db, block)
	if err != nil {
		return err
	}

	// BIP0016 describes a pay-to-script-hash type that is considered a
	// "standard" type.  The rules for this BIP only apply to transactions
	// after the timestamp defined by txscript.Bip16Activation.  See
	// https://en.bitcoin.it/wiki/BIP_0016 for more details.
	enforceBIP0016 := node.timestamp.After(txscript.Bip16Activation)

	// The number of signature operations must be less than the maximum
	// allowed per block.  Note that the preliminary sanity checks on a
	// block also include a check similar to this one, but this check
	// expands the count to include a precise count of pay-to-script-hash
	// signature operations in each of the input transaction public key
	// scripts.
	transactions := block.Transactions()
	totalSigOps := 0
	for i, tx := range transactions {
		numsigOps := CountSigOps(tx)
		if enforceBIP0016 {
			// Since the first (and only the first) transaction has
			// already been verified to be a coinbase transaction,
			// use i == 0 as an optimization for the flag to
			// countP2SHSigOps for whether or not the transaction is
			// a coinbase transaction rather than having to do a
			// full coinbase check again.
			numP2SHSigOps, err := CountP2SHSigOps(tx, i == 0, utxoView)
			if err != nil {
				return err
			}
			numsigOps += numP2SHSigOps
		}

		// Check for overflow or going over the limits.  We have to do
		// this on every loop iteration to avoid overflow.
		lastSigops := totalSigOps
		totalSigOps += numsigOps
		if totalSigOps < lastSigops || totalSigOps > MaxSigOpsPerBlock {
			str := fmt.Sprintf("block contains too many "+
				"signature operations - got %v, max %v",
				totalSigOps, MaxSigOpsPerBlock)
			return ruleError(ErrTooManySigOps, str)
		}
	}

	// Perform several checks on the inputs for each transaction.  Also
	// accumulate the total fees.  This could technically be combined with
	// the loop above instead of running another loop over the transactions,
	// but by separating it we can avoid running the more expensive (though
	// still relatively cheap as compared to running the scripts) checks
	// against all the inputs when the signature operations are out of
	// bounds.
	var totalFees int64
	for _, tx := range transactions {
		txFee, err := CheckTransactionInputs(tx, node.height, utxoView,
			b.chainParams)
		if err != nil {
			return err
		}

		// Sum the total fees and ensure we don't overflow the
		// accumulator.
		lastTotalFees := totalFees
		totalFees += txFee
		if totalFees < lastTotalFees {
			return ruleError(ErrBadFees, "total fees for block "+
				"overflows accumulator")
		}

		// CheckTransactionOutputs checks outputs for state violations.
		err = CheckTransactionOutputs(tx, keyView)
		if err != nil {
			return err
		}

		// Add all of the outputs for this transaction which are not
		// provably unspendable as available utxos.  Also, the passed
		// spent txos slice is updated to contain an entry for each
		// spent txout in the order each transaction spends them.
		err = utxoView.connectTransaction(tx, node.height, stxos)
		if err != nil {
			return err
		}
	}

	// The total output values of the coinbase transaction must not exceed
	// the expected subsidy value plus total transaction fees gained from
	// mining the block.  It is safe to ignore overflow and out of range
	// errors here because those error conditions would have already been
	// caught by checkTransactionSanity.
	var totalAtomsOut int64
	for _, txOut := range transactions[0].MsgTx().TxOut {
		totalAtomsOut += txOut.Value
	}
	expectedAtomsOut := CalcBlockSubsidy(node.height, b.chainParams) +
		totalFees
	if totalAtomsOut != expectedAtomsOut {
		str := fmt.Sprintf("coinbase transaction for block pays %v "+
			"which is not the expected value of %v",
			totalAtomsOut, expectedAtomsOut)
		return ruleError(ErrBadCoinbaseValue, str)
	}

	// Don't run scripts if this node is before the latest known good
	// checkpoint since the validity is verified via the checkpoints (all
	// transactions are included in the merkle root hash and any changes
	// will therefore be detected by the next checkpoint).  This is a huge
	// optimization because running the scripts is the most time consuming
	// portion of block handling.
	checkpoint := b.latestCheckpoint()
	runScripts := !b.noVerify
	if checkpoint != nil && node.height <= checkpoint.Height {
		runScripts = false
	}

	// Get the previous block node.  This function is used over simply
	// accessing node.parent directly as it will dynamically create previous
	// block nodes as needed.  This helps allow only the pieces of the chain
	// that are needed to remain in memory.
	prevNode, err := b.getPrevNodeFromNode(node)
	if err != nil {
		log.Errorf("getPrevNodeFromNode: %v", err)
		return err
	}

	// Blocks created after the BIP0016 activation time need to have the
	// pay-to-script-hash checks enabled.
	var scriptFlags txscript.ScriptFlags
	if enforceBIP0016 {
		scriptFlags |= txscript.ScriptBip16
	}

	// Enforce DER signatures for block versions 3+ once the majority of the
	// network has upgraded to the enforcement threshold.  This is part of
	// BIP0066.
	blockHeader := &block.MsgBlock().Header
	if blockHeader.Version >= 3 && b.isMajorityVersion(3, prevNode,
		b.chainParams.BlockEnforceNumRequired) {

		scriptFlags |= txscript.ScriptVerifyDERSignatures
	}

	// Check that the validate key used to sign the block is represented in
	// the current admin keyset state.
	validateKeySet := keyView.Keys()[btcec.ValidateKeySet]
	pubKey, err := btcec.ParsePubKey(blockHeader.ValidatingPubKey[:], btcec.S256())
	if err != nil {
		return err
	}
	if len(validateKeySet) > 0 && validateKeySet.Pos(pubKey) == -1 {
		str := fmt.Sprintf("invalid validate key %v", pubKey.SerializeCompressed())
		return ruleError(ErrInvalidValidateKey, str)
	}

	// Enforce CHECKLOCKTIMEVERIFY for block versions 4+ once the majority
	// of the network has upgraded to the enforcement threshold.  This is
	// part of BIP0065.
	if blockHeader.Version >= 4 && b.isMajorityVersion(4, prevNode,
		b.chainParams.BlockEnforceNumRequired) {

		scriptFlags |= txscript.ScriptVerifyCheckLockTimeVerify
	}

	// Check to see if there is a validate key rate limit breach.
	err, isRateLimited := b.isValidateKeyRateLimited(node, blockHeader.ValidatingPubKey, false)
	if err != nil {
		return err
	}
	if isRateLimited {
		str := fmt.Sprintf("Validate key rate limited %v", blockHeader.ValidatingPubKey)
		return ruleError(ErrExcessiveTrailing, str)
	}

	// Now that the inexpensive checks are done and have passed, verify the
	// transactions are actually allowed to spend the coins by running the
	// expensive ECDSA signature check scripts.  Doing this last helps
	// prevent CPU exhaustion attacks.
	if runScripts {
		err := checkBlockScripts(block, utxoView, keyView, scriptFlags, b.sigCache, b.hashCache)
		if err != nil {
			return err
		}
	}

	for _, tx := range transactions {
		err = keyView.connectTransaction(tx, node.height)
		if err != nil {
			return err
		}
	}

	// Update the best hash for utxoView to include this block since all of its
	// transactions have been connected.
	utxoView.SetBestHash(node.hash)

	return nil
}

// CheckConnectBlock performs several checks to confirm connecting the passed
// block to the main chain does not violate any rules.  An example of some of
// the checks performed are ensuring connecting the block would not cause any
// duplicate transaction hashes for old transactions that aren't already fully
// spent, double spends, exceeding the maximum allowed signature operations
// per block, invalid values in relation to the expected block subsidy, or fail
// transaction script validation.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckConnectBlock(block *rmgutil.Block) error {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	prevNode := b.bestNode
	newNode := newBlockNode(&block.MsgBlock().Header, block.Hash())
	newNode.parent = prevNode
	newNode.workSum.Add(prevNode.workSum, newNode.workSum)

	// Leave the spent txouts entry nil in the state since the information
	// is not needed and thus extra work can be avoided.
	utxoView := NewUtxoViewpoint()
	utxoView.SetBestHash(prevNode.hash)
	// checkConnectBlock will perform several checks to verify the block can be
	// connected  to the main chain without violating any rules and without
	// actually connecting the block.
	// To perform the verification, KeyViewpoint needs to provide the admin
	// state of the chain. The block can only be connected if:
	// - it is mined by an active validate key.
	// - all keyIDs used for outputs are provisioned.
	keyView := NewKeyViewpoint()
	keyView.SetThreadTips(b.threadTips)
	keyView.SetLastKeyID(b.lastKeyID)
	keyView.SetTotalSupply(b.totalSupply)
	keyView.SetKeys(b.adminKeySets)
	keyView.SetKeyIDs(b.aspKeyIdMap)
	return b.checkConnectBlock(newNode, block, utxoView, keyView, nil)
}
