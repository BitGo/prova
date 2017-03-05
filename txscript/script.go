// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/wire"
	"time"
)

// Bip16Activation is the timestamp where BIP0016 is valid to use in the
// blockchain.  To be used to determine if BIP0016 should be called for or not.
// This timestamp corresponds to Sun Apr 1 00:00:00 UTC 2012.
var Bip16Activation = time.Unix(1333238400, 0)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType uint32

// Hash type bits from the end of a signature.
const (
	SigHashOld          SigHashType = 0x0
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAnyOneCanPay SigHashType = 0x80

	// sigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	sigHashMask = 0x1f
)

// These are the constants specified for maximums in individual scripts.
const (
	MaxOpsPerScript       = 201 // Max number of non-push operations.
	MaxPubKeysPerMultiSig = 20  // Multisig can't have more sigs than this.
	MaxScriptElementSize  = 520 // Max bytes pushable to the stack.
)

// isSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
func isSmallInt(op *opcode) bool {
	if op.value == OP_0 || (op.value >= OP_1 && op.value <= OP_16) {
		return true
	}
	return false
}

// isUint32 returns whether or not the opcode can represent a 32-bit integer
func isUint32(op *opcode) bool {
	if isSmallInt(op) {
		return true
	}
	if op.value == OP_DATA_1 || op.value == OP_DATA_2 || op.value == OP_DATA_3 || op.value == OP_DATA_4 {
		return true
	}
	return false
}

// isScriptHash returns true if the script passed is a pay-to-script-hash
// transaction, false otherwise.
func isScriptHash(pops []parsedOpcode) bool {
	return len(pops) == 3 &&
		pops[0].opcode.value == OP_HASH160 &&
		pops[1].opcode.value == OP_DATA_20 &&
		pops[2].opcode.value == OP_EQUAL
}

// IsPayToScriptHash returns true if the script is in the standard
// pay-to-script-hash (P2SH) format, false otherwise.
func IsPayToScriptHash(script []byte) bool {
	pops, err := ParseScript(script)
	if err != nil {
		return false
	}
	return isScriptHash(pops)
}

// isPushOnly returns true if the script only pushes data, false otherwise.
func isPushOnly(pops []parsedOpcode) bool {
	// NOTE: This function does NOT verify opcodes directly since it is
	// internal and is only called with parsed opcodes for scripts that did
	// not have any parse errors.  Thus, consensus is properly maintained.

	for _, pop := range pops {
		// All opcodes up to OP_16 are data push instructions.
		// NOTE: This does consider OP_RESERVED to be a data push
		// instruction, but execution of OP_RESERVED will fail anyways
		// and matches the behavior required by consensus.
		if pop.opcode.value > OP_16 {
			return false
		}
	}
	return true
}

// IsPushOnlyScript returns whether or not the passed script only pushes data.
//
// False will be returned when the script does not parse.
func IsPushOnlyScript(script []byte) bool {
	pops, err := ParseScript(script)
	if err != nil {
		return false
	}
	return isPushOnly(pops)
}

// parseScriptTemplate is the same as parseScript but allows the passing of the
// template list for testing purposes.  When there are parse errors, it returns
// the list of parsed opcodes up to the point of failure along with the error.
func parseScriptTemplate(script []byte, opcodes *[256]opcode) ([]parsedOpcode, error) {
	retScript := make([]parsedOpcode, 0, len(script))
	for i := 0; i < len(script); {
		instr := script[i]
		op := &opcodes[instr]
		pop := parsedOpcode{opcode: op}

		// Parse data out of instruction.
		switch {
		// No additional data.  Note that some of the opcodes, notably
		// OP_1NEGATE, OP_0, and OP_[1-16] represent the data
		// themselves.
		case op.length == 1:
			i++

		// Data pushes of specific lengths -- OP_DATA_[1-75].
		case op.length > 1:
			if len(script[i:]) < op.length {
				return retScript, ErrStackShortScript
			}

			// Slice out the data.
			pop.data = script[i+1 : i+op.length]
			i += op.length

		// Data pushes with parsed lengths -- OP_PUSHDATAP{1,2,4}.
		case op.length < 0:
			var l uint
			off := i + 1

			if len(script[off:]) < -op.length {
				return retScript, ErrStackShortScript
			}

			// Next -length bytes are little endian length of data.
			switch op.length {
			case -1:
				l = uint(script[off])
			case -2:
				l = ((uint(script[off+1]) << 8) |
					uint(script[off]))
			case -4:
				l = ((uint(script[off+3]) << 24) |
					(uint(script[off+2]) << 16) |
					(uint(script[off+1]) << 8) |
					uint(script[off]))
			default:
				return retScript,
					fmt.Errorf("invalid opcode length %d",
						op.length)
			}

			// Move offset to beginning of the data.
			off += -op.length

			// Disallow entries that do not fit script or were
			// sign extended.
			if int(l) > len(script[off:]) || int(l) < 0 {
				return retScript, ErrStackShortScript
			}

			pop.data = script[off : off+int(l)]
			i += 1 - op.length + int(l)
		}

		retScript = append(retScript, pop)
	}

	return retScript, nil
}

// ParseScript preparses the script in bytes into a list of parsedOpcodes while
// applying a number of sanity checks.
func ParseScript(script []byte) ([]parsedOpcode, error) {
	return parseScriptTemplate(script, &opcodeArray)
}

// UnparseScript reversed the action of parseScript and returns the
// parsedOpcodes as a list of bytes
func UnparseScript(pops []parsedOpcode) ([]byte, error) {
	script := make([]byte, 0, len(pops))
	for _, pop := range pops {
		b, err := pop.bytes()
		if err != nil {
			return nil, err
		}
		script = append(script, b...)
	}
	return script, nil
}

// DisasmString formats a disassembled script for one line printing.  When the
// script fails to parse, the returned string will contain the disassembled
// script up to the point the failure occurred along with the string '[error]'
// appended.  In addition, the reason the script failed to parse is returned
// if the caller wants more information about the failure.
func DisasmString(buf []byte) (string, error) {
	var disbuf bytes.Buffer
	opcodes, err := ParseScript(buf)
	for _, pop := range opcodes {
		disbuf.WriteString(pop.print(true))
		disbuf.WriteByte(' ')
	}
	if disbuf.Len() > 0 {
		disbuf.Truncate(disbuf.Len() - 1)
	}
	if err != nil {
		disbuf.WriteString("[error]")
	}
	return disbuf.String(), err
}

// removeOpcode will remove any opcode matching ``opcode'' from the opcode
// stream in pkscript
func removeOpcode(pkscript []parsedOpcode, opcode byte) []parsedOpcode {
	retScript := make([]parsedOpcode, 0, len(pkscript))
	for _, pop := range pkscript {
		if pop.opcode.value != opcode {
			retScript = append(retScript, pop)
		}
	}
	return retScript
}

// asInt32 will convert an opcode to a int32. make sure to use isUint32
// before, to check that opcode can be converted.
func asInt32(pop parsedOpcode) (int32, error) {
	if isSmallInt(pop.opcode) {
		return int32(asSmallInt(pop.opcode)), nil
	}
	result, err := makeScriptNum(pop.data, true, 4)
	return result.Int32(), err
}

// ExtractKeyIDs takes an Prova pkScript and extracts the keyIDs from it.
// We assume a Prova address structure like this:
// basic: <2 hash keyID1 keyID2 3 OP_CHECKSAFEMULTISIG>
// general: <x hash/keyID hash/keyID y OP_CHECKSAFEMULTISIG>
func ExtractKeyIDs(pkScript []parsedOpcode) ([]btcec.KeyID, error) {
	// the basic structure has 6 elements, as described above
	if len(pkScript) < 6 || !isSmallInt(pkScript[len(pkScript)-2].opcode) {
		return nil, fmt.Errorf("unable to extract keyIDs from script, "+
			"unexpected script structure %v", pkScript)
	}
	pkHashCount := asSmallInt(pkScript[len(pkScript)-2].opcode)
	keyIDs := make([]btcec.KeyID, 0, pkHashCount)
	for i := 2; i <= pkHashCount; i++ {
		if !isUint32(pkScript[i].opcode) {
			return nil, fmt.Errorf("unable to extract keyIDs from script, "+
				"unexpected script structure at opcode %v", pkScript[i])
		}
		keyID, err := asInt32(pkScript[i])
		if err != nil {
			return nil, err
		}
		keyIDs = append(keyIDs, btcec.KeyID(keyID))
	}
	return keyIDs, nil
}

// ReplaceKeyIds replaces keyIds in a pkScript with pubKeyHashes.
// We assume a Prova address structure like this:
// basic: <2 hash keyID1 keyID2 3 OP_CHECKSAFEMULTISIG>
// general: <x hash/keyID hash/keyID y OP_CHECKSAFEMULTISIG>
func ReplaceKeyIDs(pkScript []parsedOpcode, keyIdMap map[btcec.KeyID][]byte) error {
	// the basic structure has 6 elements, as described above
	if len(pkScript) < 6 || !isSmallInt(pkScript[len(pkScript)-2].opcode) {
		return fmt.Errorf("unable to extract keyIDs from script, "+
			"unexpected script structure %v", pkScript)
	}
	// no work to be done
	if len(keyIdMap) == 0 {
		return fmt.Errorf("no keyHashes provided to replace keyIDs")
	}
	pkHashCount := asSmallInt(pkScript[len(pkScript)-2].opcode)
	for i := 2; i <= pkHashCount; i++ {
		pop := &pkScript[i]
		if !isUint32(pop.opcode) {
			return fmt.Errorf("unable to replace keyIDs in script, "+
				"unexpected script structure at opcode %v", pop)
		}
		keyID, err := asInt32(*pop)
		if err != nil {
			return fmt.Errorf("unable to parse keyIDs from opcode %v",
				pkScript[i])
		}
		if val, ok := keyIdMap[btcec.KeyID(keyID)]; ok {
			pop.data = val
			pop.opcode = &opcodeArray[OP_DATA_20]
		}
	}
	return nil
}

// ExtractThreadID takes an Prova admin pkScript and extracts the threadID from it.
// We assume an Prova admin pkScript structure like this:
// <threadID> OP_CHECKTHREAD
func ExtractThreadID(pkScript []parsedOpcode) (rmgutil.ThreadID, error) {
	if len(pkScript) != 2 || !isSmallInt(pkScript[0].opcode) {
		return rmgutil.ThreadID(0),
			fmt.Errorf("unable to extract threadID from script, "+
				"unknown script structure %v", pkScript)
	}
	return rmgutil.ThreadID(asSmallInt(pkScript[0].opcode)), nil
}

// ThreadPkScript creates a new pkScript with all keyHashes.
// 2 <pkHash> ... <pkHash> X OP_CHECKTHREAD
func ThreadPkScript(keyHashes [][]byte) ([]byte, error) {
	if len(keyHashes) < 2 {
		return nil, fmt.Errorf("invalid chain state, at least 2 keys required" +
			" for thread.")
	}
	// build the new pkScript with 2 of x multi-sig
	pkScript := NewScriptBuilder().AddOp(OP_2)
	for i := range keyHashes {
		pkScript.AddData(keyHashes[i])
	}
	return pkScript.AddInt64(int64(len(keyHashes))).AddOp(OP_CHECKTHREAD).Script()
}

// ExtractAdminData can read OP_*KEYADD and OP_*KEYREVOKE from admin outputs.
// An admin op script of structure <OP_RETURN><OP_DATA> can be assumed from
// previous validation.
// This function returns the admin operation type byte, and the parsed
// public key.
func ExtractAdminData(pkScript []parsedOpcode) (byte, *btcec.PublicKey, error) {
	pubKey, err := btcec.ParsePubKey(pkScript[1].data[1:1+btcec.PubKeyBytesLenCompressed], btcec.S256())
	if err != nil {
		return 0, nil, err
	}
	return pkScript[1].data[0], pubKey, nil
}

// ExtractASPData can read AdminOpASPKeyAdd and AdminOpASPKeyRevoke from admin outputs.
// An admin op script of structure <OP_RETURN><OP_DATA> can be assumed from
// previous validation.
// This function returns the admin operation type byte, the parsed keyID, and
// the parsed public key.
func ExtractASPData(pkScript []parsedOpcode) (byte, *btcec.PublicKey, btcec.KeyID, error) {
	pubKey, err := btcec.ParsePubKey(pkScript[1].data[1:1+btcec.PubKeyBytesLenCompressed], btcec.S256())
	if err != nil {
		return 0, nil, 0, err
	}
	dataLen := len(pkScript[1].data)
	keyID := btcec.KeyIDFromAddressBuffer(pkScript[1].data[dataLen-btcec.KeyIDSize : dataLen])
	return pkScript[1].data[0], pubKey, keyID, nil
}

// ExtractAdminOpData extract operation type and values from admin operations
// in admin transactions.
// The function assumes previous validation of all passed opcodes as admin ops.
func ExtractAdminOpData(pkScript []parsedOpcode) (bool, btcec.KeySetType, *btcec.PublicKey, btcec.KeyID) {
	pubKey, _ := btcec.ParsePubKey(pkScript[1].data[1:1+btcec.PubKeyBytesLenCompressed], btcec.S256())
	dataLen := len(pkScript[1].data)
	keyID := btcec.KeyID(0)
	if dataLen > 1+btcec.PubKeyBytesLenCompressed {
		keyID = btcec.KeyIDFromAddressBuffer(pkScript[1].data[dataLen-btcec.KeyIDSize : dataLen])
	}
	var isAddOp bool
	keySetType := btcec.KeySetType(0)
	switch pkScript[1].data[0] {
	case AdminOpProvisionKeyAdd:
		isAddOp = true
		keySetType = btcec.ProvisionKeySet
	case AdminOpProvisionKeyRevoke:
		isAddOp = false
		keySetType = btcec.ProvisionKeySet
	case AdminOpIssueKeyAdd:
		isAddOp = true
		keySetType = btcec.IssueKeySet
	case AdminOpIssueKeyRevoke:
		isAddOp = false
		keySetType = btcec.IssueKeySet
	case AdminOpValidateKeyAdd:
		isAddOp = true
		keySetType = btcec.ValidateKeySet
	case AdminOpValidateKeyRevoke:
		isAddOp = false
		keySetType = btcec.ValidateKeySet
	case AdminOpASPKeyAdd:
		isAddOp = true
		keySetType = btcec.ASPKeySet
	case AdminOpASPKeyRevoke:
		isAddOp = false
		keySetType = btcec.ASPKeySet
	}
	return isAddOp, keySetType, pubKey, keyID
}

// AdminOpString gives a human-readable version of an admin op script.
// The function assumes previous validation as an actual valid admin op script.
func AdminOpString(buf []byte) string {
	opcodes, err := ParseScript(buf)
	if err != nil {
		return ""
	}
	isAddOp, keySetType, pubKey, keyID := ExtractAdminOpData(opcodes)
	op := "REVOKE_KEY"
	if isAddOp {
		op = "ADD_KEY"
	}
	result := fmt.Sprintf("%s %s %s",
		op,
		keySetType.String(),
		hex.EncodeToString(pubKey.SerializeCompressed()))
	if keyID > 0 {
		result = fmt.Sprintf("%s %d", result, uint32(keyID))
	}
	return result
}

// canonicalPush returns true if the object is either not a push instruction
// or the push instruction contained wherein is matches the canonical form
// or using the smallest instruction to do the job. False otherwise.
func canonicalPush(pop parsedOpcode) bool {
	opcode := pop.opcode.value
	data := pop.data
	dataLen := len(pop.data)
	if opcode > OP_16 {
		return true
	}

	if opcode < OP_PUSHDATA1 && opcode > OP_0 && (dataLen == 1 && data[0] <= 16) {
		return false
	}
	if opcode == OP_PUSHDATA1 && dataLen < OP_PUSHDATA1 {
		return false
	}
	if opcode == OP_PUSHDATA2 && dataLen <= 0xff {
		return false
	}
	if opcode == OP_PUSHDATA4 && dataLen <= 0xffff {
		return false
	}
	return true
}

// removeOpcodeByData will return the script minus any opcodes that would push
// the passed data to the stack.
func removeOpcodeByData(pkscript []parsedOpcode, data []byte) []parsedOpcode {
	retScript := make([]parsedOpcode, 0, len(pkscript))
	for _, pop := range pkscript {
		if !canonicalPush(pop) || !bytes.Contains(pop.data, data) {
			retScript = append(retScript, pop)
		}
	}
	return retScript

}

// calcSignatureHash will, given a script and hash type for the current script
// engine instance, calculate the signature hash to be used for signing and
// verification.
// TODO(prova): Redefine this completely to eliminate malleability (segwit)
func calcSignatureHash(script []parsedOpcode, hashType SigHashType, tx *wire.MsgTx, idx int) []byte {
	// The SigHashSingle signature type signs only the corresponding input
	// and output (the output with the same index number as the input).
	//
	// Since transactions can have more
	// inputs than outputs, this means it
	// is improper to use SigHashSingle on input indices that don't have a
	// corresponding output.
	//
	// A bug in the original Satoshi client implementation means specifying
	// an index that is out of range results in a signature hash of 1 (as a
	// uint256 little endian).  The original intent appeared to be to
	// indicate failure, but unfortunately, it was never checked and thus is
	// treated as the actual signature hash.  This buggy behavior is now
	// part of the consensus and a hard fork would be required to fix it.
	//
	// Due to this, care must be taken by software that creates transactions
	// which make use of SigHashSingle because it can lead to an extremely
	// dangerous situation where the invalid inputs will end up signing a
	// hash of 1.  This in turn presents an opportunity for attackers to
	// cleverly construct transactions which can steal those coins provided
	// they can reuse signatures.
	if hashType&sigHashMask == SigHashSingle && idx >= len(tx.TxOut) {
		var hash chainhash.Hash
		hash[0] = 0x01
		return hash[:]
	}

	// Remove all instances of OP_CODESEPARATOR from the script.
	script = removeOpcode(script, OP_CODESEPARATOR)

	// Make a deep copy of the transaction, zeroing out the script for all
	// inputs that are not currently being processed.
	txCopy := tx.Copy()
	for i := range txCopy.TxIn {
		if i == idx {
			// UnparseScript cannot fail here because removeOpcode
			// above only returns a valid script.
			sigScript, _ := UnparseScript(script)
			txCopy.TxIn[idx].SignatureScript = sigScript
		} else {
			txCopy.TxIn[i].SignatureScript = nil
		}
	}

	switch hashType & sigHashMask {
	case SigHashNone:
		txCopy.TxOut = txCopy.TxOut[0:0] // Empty slice.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}

	case SigHashSingle:
		// Resize output array to up to and including requested index.
		txCopy.TxOut = txCopy.TxOut[:idx+1]

		// All but current output get zeroed out.
		for i := 0; i < idx; i++ {
			txCopy.TxOut[i].Value = -1
			txCopy.TxOut[i].PkScript = nil
		}

		// Sequence on all other inputs is 0, too.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}

	default:
		// Consensus treats undefined hashtypes like normal SigHashAll
		// for purposes of hash generation.
		fallthrough
	case SigHashOld:
		fallthrough
	case SigHashAll:
		// Nothing special here.
	}
	if hashType&SigHashAnyOneCanPay != 0 {
		txCopy.TxIn = txCopy.TxIn[idx : idx+1]
		idx = 0
	}

	// The final hash is the double sha256 of both the serialized modified
	// transaction and the hash type (encoded as a 4-byte little-endian
	// value) appended.
	var wbuf bytes.Buffer
	txCopy.Serialize(&wbuf)
	binary.Write(&wbuf, binary.LittleEndian, hashType)
	return chainhash.DoubleHashB(wbuf.Bytes())
}

// calcHashPrevOuts calculates a single hash of all the previous outputs
// (txid:index) referenced within the passed transaction. This calculated hash
// can be re-used when validating all inputs with signature hash type of
// SigHashAll. This allows validation to re-use previous hashing computation,
// reducing the complexity of validating SigHashAll inputs from  O(N^2) to O(N).
func calcHashPrevOuts(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, in := range tx.TxIn {
		// First write out the 32-byte transaction ID one of whose
		// outputs are being referenced by this input.
		b.Write(in.PreviousOutPoint.Hash[:])

		// Next, we'll encode the index of the referenced output as a
		// little endian integer.
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], in.PreviousOutPoint.Index)
		b.Write(buf[:])
	}

	return chainhash.DoubleHashH(b.Bytes())
}

// calcHashSequence computes an aggregated hash of each of the sequence numbers
// within the inputs of the passed transaction. This single hash can be re-used
// when validating all inputs with signature hash type of
// SigHashAll. This allows validation to re-use previous hashing computation,
// reducing the complexity of validating SigHashAll inputs from  O(N^2) to O(N).
func calcHashSequence(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, in := range tx.TxIn {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], in.Sequence)
		b.Write(buf[:])
	}
	return chainhash.DoubleHashH(b.Bytes())
}

// calcHashOutputs computes a hash digest of all outputs created by the
// transaction encoded using the wire format. This single hash can be re-used
// when validating all inputs with signature hash type of
// SigHashAll. This allows validation to re-use previous hashing computation,
// reducing the complexity of validating SigHashAll inputs from  O(N^2) to O(N).
func calcHashOutputs(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, out := range tx.TxOut {
		wire.WriteTxOut(&b, 0, 0, out)
	}
	return chainhash.DoubleHashH(b.Bytes())
}

// calcSignatureHashNew computes the sighash digest of a transaction's input
// using the new, optimized digest calculation algorithm defined in BIP0143:
// https://github.com/bitcoin/bips/blob/master/bip-0143.mediawiki.
// This function makes use of pre-calculated sighash fragments stored within
// the passed HashCache to eliminate duplicate hashing computations when
// calculating the final digest, reducing the complexity from O(N^2) to O(N).
// Additionally, signatures now cover the input value of the referenced unspent
// output. This allows offline, or hardware wallets to compute the exact amount
// being spent, in addition to the final transaction fee. In the case the
// wallet if fed an invalid input amount, the real sighash will differ causing
// the produced signature to be invalid.
func calcSignatureHashNew(subScript []parsedOpcode, sigHashes *TxSigHashes,
	hashType SigHashType, tx *wire.MsgTx, idx int, amt int64) []byte {

	// As a sanity check, ensure the passed input index for the transaction
	// is valid.
	if idx >= len(tx.TxIn) {
		fmt.Errorf("calcSignatureHashNew error: idx %d but %d txins",
			idx, len(tx.TxIn))
		return nil
	}

	// For now we only accept SigHashAll transactions
	if hashType != SigHashAll {
		fmt.Errorf("calcSignatureHashNew error: idx %d with wrong hashType %v.",
			idx, hashType)
	}

	// We'll utilize this buffer throughout to incrementally calculate
	// the signature hash for this transaction.
	var sigHash bytes.Buffer

	// First write out, then encode the transaction's version number.
	var bVersion [4]byte
	binary.LittleEndian.PutUint32(bVersion[:], uint32(tx.Version))
	sigHash.Write(bVersion[:])

	// Next, write the cached hashPrevOuts.
	sigHash.Write(sigHashes.HashPrevOuts[:])

	// Next, write the cached hashSequence
	sigHash.Write(sigHashes.HashSequence[:])

	// Next, write the outpoint being spent.
	sigHash.Write(tx.TxIn[idx].PreviousOutPoint.Hash[:])
	var bIndex [4]byte
	binary.LittleEndian.PutUint32(bIndex[:], tx.TxIn[idx].PreviousOutPoint.Index)
	sigHash.Write(bIndex[:])

	// In BIP 143, we would write the scriptCode of the input itself here.
	// The script code can be relevant to certain hardware wallets.
	// There is no use-case for this in the RMG chain.

	// Next, add the input amount, and sequence number of the input being
	// signed.
	var bAmount [8]byte
	binary.LittleEndian.PutUint64(bAmount[:], uint64(amt))
	sigHash.Write(bAmount[:])
	var bSequence [4]byte
	binary.LittleEndian.PutUint32(bSequence[:], tx.TxIn[idx].Sequence)
	sigHash.Write(bSequence[:])

	// Next, add the  pre-generated hashoutputs sighash fragment.
	sigHash.Write(sigHashes.HashOutputs[:])

	// Finally, write out the transaction's locktime, and the sig hash
	// type.
	var bLockTime [4]byte
	binary.LittleEndian.PutUint32(bLockTime[:], tx.LockTime)
	sigHash.Write(bLockTime[:])
	var bHashType [4]byte
	binary.LittleEndian.PutUint32(bHashType[:], uint32(hashType))
	sigHash.Write(bHashType[:])

	return chainhash.DoubleHashB(sigHash.Bytes())
}

// asSmallInt returns the passed opcode, which must be true according to
// isSmallInt(), as an integer.
func asSmallInt(op *opcode) int {
	if op.value == OP_0 {
		return 0
	}

	return int(op.value - (OP_1 - 1))
}

// getSigOpCount is the implementation function for counting the number of
// signature operations in the script provided by pops. If precise mode is
// requested then we attempt to count the number of operations for a multisig
// op. Otherwise we use the maximum.
func getSigOpCount(pops []parsedOpcode, precise bool) int {
	nSigs := 0
	for i, pop := range pops {
		switch pop.opcode.value {
		case OP_CHECKSIG:
			fallthrough
		case OP_CHECKSIGVERIFY:
			nSigs++
		case OP_CHECKMULTISIG:
			fallthrough
		case OP_CHECKMULTISIGVERIFY:
			// If we are being precise then look for familiar
			// patterns for multisig, for now all we recognize is
			// OP_1 - OP_16 to signify the number of pubkeys.
			// Otherwise, we use the max of 20.
			if precise && i > 0 &&
				pops[i-1].opcode.value >= OP_1 &&
				pops[i-1].opcode.value <= OP_16 {
				nSigs += asSmallInt(pops[i-1].opcode)
			} else {
				nSigs += MaxPubKeysPerMultiSig
			}
		case OP_CHECKSAFEMULTISIG:
			// TODO(prova): implement
			fallthrough
		default:
			// Not a sigop.
		}
	}

	return nSigs
}

// GetSigOpCount provides a quick count of the number of signature operations
// in a script. a CHECKSIG operations counts for 1, and a CHECK_MULTISIG for 20.
// If the script fails to parse, then the count up to the point of failure is
// returned.
func GetSigOpCount(script []byte) int {
	// Don't check error since parseScript returns the parsed-up-to-error
	// list of pops.
	pops, _ := ParseScript(script)
	return getSigOpCount(pops, false)
}

// GetPreciseSigOpCount returns the number of signature operations in
// scriptPubKey.  If bip16 is true then scriptSig may be searched for the
// Pay-To-Script-Hash script in order to find the precise number of signature
// operations in the transaction.  If the script fails to parse, then the count
// up to the point of failure is returned.
func GetPreciseSigOpCount(scriptSig, scriptPubKey []byte, bip16 bool) int {
	// Don't check error since parseScript returns the parsed-up-to-error
	// list of pops.
	pops, _ := ParseScript(scriptPubKey)

	// Treat non P2SH transactions as normal.
	if !(bip16 && isScriptHash(pops)) {
		return getSigOpCount(pops, true)
	}

	// The public key script is a pay-to-script-hash, so parse the signature
	// script to get the final item.  Scripts that fail to fully parse count
	// as 0 signature operations.
	sigPops, err := ParseScript(scriptSig)
	if err != nil {
		return 0
	}

	// The signature script must only push data to the stack for P2SH to be
	// a valid pair, so the signature operation count is 0 when that is not
	// the case.
	if !isPushOnly(sigPops) || len(sigPops) == 0 {
		return 0
	}

	// The P2SH script is the last item the signature script pushes to the
	// stack.  When the script is empty, there are no signature operations.
	shScript := sigPops[len(sigPops)-1].data
	if len(shScript) == 0 {
		return 0
	}

	// Parse the P2SH script and don't check the error since parseScript
	// returns the parsed-up-to-error list of pops and the consensus rules
	// dictate signature operations are counted up to the first parse
	// failure.
	shPops, _ := ParseScript(shScript)
	return getSigOpCount(shPops, true)
}

// IsUnspendable returns whether the passed public key script is unspendable, or
// guaranteed to fail at execution.  This allows inputs to be pruned instantly
// when entering the UTXO set.
func IsUnspendable(pkScript []byte) bool {
	pops, err := ParseScript(pkScript)
	if err != nil {
		return true
	}

	return len(pops) > 0 && pops[0].opcode.value == OP_RETURN
}
