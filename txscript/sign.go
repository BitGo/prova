// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"fmt"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/wire"
	"sort"
)

// RawTxInSignature returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
func RawTxInSignature(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, key *btcec.PrivateKey) ([]byte, error) {

	parsedScript, err := ParseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}
	hash := calcSignatureHash(parsedScript, hashType, tx, idx)
	signature, err := key.Sign(hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}

	return append(signature.Serialize(), byte(hashType)), nil
}

// RawTxInSignatureNew returns the serialized ECDSA signature for the input idx of
// the given transaction, with hashType appended to it.
// TODO(prova): need to cleanup the old/new versions
func RawTxInSignatureNew(tx *wire.MsgTx, idx int, txSigHashes *TxSigHashes, amt int64, subScript []byte,
	hashType SigHashType, key *btcec.PrivateKey) ([]byte, error) {

	parsedScript, err := ParseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}

	hash := calcSignatureHashNew(parsedScript, txSigHashes, hashType, tx, idx, amt)
	signature, err := key.Sign(hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}

	return append(signature.Serialize(), byte(hashType)), nil
}

// SignatureScript creates an input signature script for tx to spend RMG sent
// from a previous output to the owner of privKey. tx must include all
// transaction inputs and outputs, however txin scripts are allowed to be filled
// or empty. The returned script is calculated to be used as the idx'th txin
// sigscript for tx. subscript is the PkScript of the previous output being used
// as the idx'th input. privKey is serialized in either a compressed or
// uncompressed format based on compress. This format must match the same format
// used to generate the payment address, or the script validation will fail.
func SignatureScript(tx *wire.MsgTx, idx int, subscript []byte, hashType SigHashType, privKey *btcec.PrivateKey, compress bool) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subscript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	pk := (*btcec.PublicKey)(&privKey.PublicKey)
	var pkData []byte
	if compress {
		pkData = pk.SerializeCompressed()
	} else {
		pkData = pk.SerializeUncompressed()
	}

	return NewScriptBuilder().AddData(sig).AddData(pkData).Script()
}

func p2pkSignatureScript(tx *wire.MsgTx, idx int, subScript []byte, hashType SigHashType, privKey *btcec.PrivateKey) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subScript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	return NewScriptBuilder().AddData(sig).Script()
}

// signSafeMultiSig signs as many of the outputs in the provided multisig script as
// possible. It returns the generated script and a boolean if the script fulfils
// the contract (i.e. nrequired signatures are provided).  Since it is arguably
// legal to not be able to sign any of the outputs, no error is returned.
func signSafeMultiSig(tx *wire.MsgTx, idx int, txSigHashes *TxSigHashes, amt int64, subScript []byte, hashType SigHashType,
	keys []PrivateKey, nRequired int, kdb KeyDB) ([]byte, bool) {
	builder := NewScriptBuilder()
	signed := 0

	for _, key := range keys {

		// add pubKey
		pk := (*btcec.PublicKey)(&key.Key.PublicKey)
		builder.AddData(pk.SerializeCompressed())

		// add signature
		sig, err := RawTxInSignatureNew(tx, idx, txSigHashes, amt, subScript, hashType, key.Key)
		if err != nil {
			// we silently ignore errors, because not all keys need to sign for a valid tx.
			continue
		}
		builder.AddData(sig)
		signed++
		if signed == nRequired {
			break
		}

	}

	script, _ := builder.Script()
	return script, signed == nRequired
}

// signMultiSig signs as many of the outputs in the provided multisig script as
// possible. It returns the generated script and a boolean if the script fulfils
// the contract (i.e. nrequired signatures are provided).  Since it is arguably
// legal to not be able to sign any of the outputs, no error is returned.
func signMultiSig(tx *wire.MsgTx, idx int, subScript []byte, hashType SigHashType,
	addresses []rmgutil.Address, nRequired int, kdb KeyDB) ([]byte, bool) {
	// We start with a single OP_FALSE to work around the (now standard)
	// but in the reference implementation that causes a spurious pop at
	// the end of OP_CHECKMULTISIG.
	builder := NewScriptBuilder().AddOp(OP_FALSE)
	signed := 0
	for _, addr := range addresses {
		keys, err := kdb.GetKey(addr)
		if err != nil {
			continue
		}
		sig, err := RawTxInSignature(tx, idx, subScript, hashType, keys[0].Key)
		if err != nil {
			continue
		}

		builder.AddData(sig)
		signed++
		if signed == nRequired {
			break
		}

	}

	script, _ := builder.Script()
	return script, signed == nRequired
}

func sign(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int, inputAmt int64,
	subScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB) (
	[]byte, ScriptClass, []rmgutil.Address, int, error) {

	class, addresses, nrequired, err := ExtractPkScriptAddrs(subScript,
		chainParams)
	if err != nil {
		return nil, NonStandardTy, nil, 0, err
	}

	// Create a new HashCache adding the intermediate sigHashes of this tx to it.
	// The size of the HashCache is chosen big enough for any transaction.
	// TODO(prova) find a better way to set size of HashCache
	hashCache := NewHashCache(90)
	hashCache.AddSigHashes(tx)
	txHash := tx.TxHash()
	txSigHashes, found := hashCache.GetSigHashes(&txHash)
	if !found {
		return nil, class, nil, 0, errors.New("unable to find sighashes")
	}

	switch class {
	case PubKeyTy:
		// look up key for address
		keys, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := p2pkSignatureScript(tx, idx, subScript, hashType,
			keys[0].Key)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case PubKeyHashTy:
		// look up key for address
		keys, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := SignatureScript(tx, idx, subScript, hashType,
			keys[0].Key, keys[0].Compressed)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case ScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case MultiSigTy:
		script, _ := signMultiSig(tx, idx, subScript, hashType,
			addresses, nrequired, kdb)
		return script, class, addresses, nrequired, nil
	case ProvaTy:
		// We use the keysDb lookup to get a list of privKeys
		// that are needed for signing.
		keys, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}
		// do the signing
		script, _ := signSafeMultiSig(tx, idx, txSigHashes, inputAmt, subScript, hashType,
			keys, nrequired, kdb)
		return script, class, addresses, nrequired, nil
	case ProvaAdminTy:
		// We use the keysDb lookup to get a list of privKeys that are needed
		// for signing. Passing nil will give us all keys.
		keys, err := kdb.GetKey(nil)
		if err != nil {
			return nil, class, nil, 0, err
		}
		// do the signing
		script, _ := signSafeMultiSig(tx, idx, txSigHashes, inputAmt, subScript, hashType,
			keys, nrequired, kdb)
		return script, class, addresses, nrequired, nil
	case NullDataTy:
		return nil, class, nil, 0,
			errors.New("can't sign NULLDATA transactions")
	default:
		return nil, class, nil, 0,
			errors.New("can't sign unknown transactions")
	}
}

// mergeScripts merges sigScript and prevScript assuming they are both
// partial solutions for pkScript spending output idx of tx. class, addresses
// and nrequired are the result of extracting the addresses from pkscript.
// The return value is the best effort merging of the two scripts. Calling this
// function with addresses, class and nrequired that do not match pkScript is
// an error and results in undefined behaviour.
func mergeScripts(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, class ScriptClass, addresses []rmgutil.Address,
	nRequired int, sigScript, prevScript []byte) []byte {

	// TODO(oga) the scripthash and multisig paths here are overly
	// inefficient in that they will recompute already known data.
	// some internal refactoring could probably make this avoid needless
	// extra calculations.
	switch class {
	case ScriptHashTy:
		// Remove the last push in the script and then recurse.
		// this could be a lot less inefficient.
		sigPops, err := ParseScript(sigScript)
		if err != nil || len(sigPops) == 0 {
			return prevScript
		}
		prevPops, err := ParseScript(prevScript)
		if err != nil || len(prevPops) == 0 {
			return sigScript
		}

		// assume that script in sigPops is the correct one, we just
		// made it.
		script := sigPops[len(sigPops)-1].data

		// We already know this information somewhere up the stack.
		class, addresses, nrequired, err :=
			ExtractPkScriptAddrs(script, chainParams)

		// regenerate scripts.
		sigScript, _ := UnparseScript(sigPops)
		prevScript, _ := UnparseScript(prevPops)

		// Merge
		mergedScript := mergeScripts(chainParams, tx, idx, script,
			class, addresses, nrequired, sigScript, prevScript)

		// Reappend the script and return the result.
		builder := NewScriptBuilder()
		builder.AddOps(mergedScript)
		builder.AddData(script)
		finalScript, _ := builder.Script()
		return finalScript
	case MultiSigTy:
		return mergeMultiSig(tx, idx, addresses, nRequired, pkScript,
			sigScript, prevScript)
	case ProvaTy:
		return mergeProvaSig(tx, idx, addresses, nRequired, pkScript,
			sigScript, prevScript)
	case ProvaAdminTy:
		return mergeProvaAdminSig(tx, idx, addresses, nRequired, pkScript,
			sigScript, prevScript)

	// It doesn't actually make sense to merge anything other than multiig
	// and scripthash (because it could contain multisig). Everything else
	// has either zero signature, can't be spent, or has a single signature
	// which is either present or not. The other two cases are handled
	// above. In the conflict case here we just assume the longest is
	// correct (this matches behaviour of the reference implementation).
	default:
		if len(sigScript) > len(prevScript) {
			return sigScript
		}
		return prevScript
	}
}

// mergeMultiSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx. addresses
// and nRequired should be the results from extracting the addresses from
// pkScript. Since this function is internal only we assume that the arguments
// have come from other functions internally and thus are all consistent with
// each other, behaviour is undefined if this contract is broken.
func mergeMultiSig(tx *wire.MsgTx, idx int, addresses []rmgutil.Address,
	nRequired int, pkScript, sigScript, prevScript []byte) []byte {

	// This is an internal only function and we already parsed this script
	// as ok for multisig (this is how we got here), so if this fails then
	// all assumptions are broken and who knows which way is up?
	pkPops, _ := ParseScript(pkScript)

	sigPops, err := ParseScript(sigScript)
	if err != nil || len(sigPops) == 0 {
		return prevScript
	}

	prevPops, err := ParseScript(prevScript)
	if err != nil || len(prevPops) == 0 {
		return sigScript
	}

	// Convenience function to avoid duplication.
	extractSigs := func(pops []parsedOpcode, sigs [][]byte) [][]byte {
		for _, pop := range pops {
			if len(pop.data) != 0 {
				sigs = append(sigs, pop.data)
			}
		}
		return sigs
	}

	possibleSigs := make([][]byte, 0, len(sigPops)+len(prevPops))
	possibleSigs = extractSigs(sigPops, possibleSigs)
	possibleSigs = extractSigs(prevPops, possibleSigs)

	// Now we need to match the signatures to pubkeys, the only real way to
	// do that is to try to verify them all and match it to the pubkey
	// that verifies it. we then can go through the addresses in order
	// to build our script. Anything that doesn't parse or doesn't verify we
	// throw away.
	addrToSig := make(map[string][]byte)
sigLoop:
	for _, sig := range possibleSigs {

		// can't have a valid signature that doesn't at least have a
		// hashtype, in practise it is even longer than this. but
		// that'll be checked next.
		if len(sig) < 1 {
			continue
		}
		tSig := sig[:len(sig)-1]
		hashType := SigHashType(sig[len(sig)-1])

		pSig, err := btcec.ParseDERSignature(tSig, btcec.S256())
		if err != nil {
			continue
		}

		// We have to do this each round since hash types may vary
		// between signatures and so the hash will vary. We can,
		// however, assume no sigs etc are in the script since that
		// would make the transaction nonstandard and thus not
		// MultiSigTy, so we just need to hash the full thing.
		hash := calcSignatureHash(pkPops, hashType, tx, idx)

		for _, addr := range addresses {
			// All multisig addresses should be pubkey addreses
			// it is an error to call this internal function with
			// bad input.
			pkaddr := addr.(*rmgutil.AddressPubKey)

			pubKey := pkaddr.PubKey()

			// If it matches we put it in the map. We only
			// can take one signature per public key so if we
			// already have one, we can throw this away.
			if pSig.Verify(hash, pubKey) {
				aStr := addr.EncodeAddress()
				if _, ok := addrToSig[aStr]; !ok {
					addrToSig[aStr] = sig
				}
				continue sigLoop
			}
		}
	}

	// Extra opcode to handle the extra arg consumed (due to previous bugs
	// in the reference implementation).
	builder := NewScriptBuilder().AddOp(OP_FALSE)
	doneSigs := 0
	// This assumes that addresses are in the same order as in the script.
	for _, addr := range addresses {
		sig, ok := addrToSig[addr.EncodeAddress()]
		if !ok {
			continue
		}
		builder.AddData(sig)
		doneSigs++
		if doneSigs == nRequired {
			break
		}
	}

	// padding for missing ones.
	for i := doneSigs; i < nRequired; i++ {
		builder.AddOp(OP_0)
	}

	script, _ := builder.Script()
	return script
}

// mergeProvaSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx.
func mergeProvaSig(tx *wire.MsgTx, idx int, addresses []rmgutil.Address,
	nRequired int, pkScript, sigScript, prevScript []byte) []byte {
	return append(prevScript[:], sigScript[:]...)
}

// mergeProvaAdminSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx.
func mergeProvaAdminSig(tx *wire.MsgTx, idx int, addresses []rmgutil.Address,
	nRequired int, pkScript, sigScript, prevScript []byte) []byte {

	sigPops, err := ParseScript(sigScript)
	if err != nil || len(sigPops) == 0 {
		return prevScript
	}

	prevPops, err := ParseScript(prevScript)
	if err != nil || len(prevPops) == 0 {
		return sigScript
	}

	// create a map of pub to sig
	pubToOps := make(map[string][2]parsedOpcode)
	for i := 0; i < len(sigPops); i = i + 2 {
		pubKey, _ := btcec.ParsePubKey(sigPops[i].data, btcec.S256())
		pubKeyStr := fmt.Sprintf("%x", pubKey.SerializeCompressed())
		pubToOps[pubKeyStr] = [2]parsedOpcode{sigPops[i], sigPops[i+1]}
	}
	for i := 0; i < len(prevPops); i = i + 2 {
		pubKey, _ := btcec.ParsePubKey(prevPops[i].data, btcec.S256())
		pubKeyStr := fmt.Sprintf("%x", pubKey.SerializeCompressed())
		pubToOps[pubKeyStr] = [2]parsedOpcode{prevPops[i], prevPops[i+1]}
	}
	// sort pubs alphanumerically
	pubs := make([]string, 0, len(pubToOps))
	for pub := range pubToOps {
		pubs = append(pubs, pub)
	}
	sort.Strings(pubs)

	// create new script with right ordering
	builder := NewScriptBuilder()
	doneSigs := 0
	for _, pub := range pubs {
		builder.AddData(pubToOps[pub][0].data)
		builder.AddData(pubToOps[pub][1].data)
		doneSigs++
		if doneSigs == nRequired {
			break
		}
	}
	script, _ := builder.Script()
	return script
}

// PrivateKeys is a wrapper for the PrivateKey array to allow sorting.
type PrivateKeys []PrivateKey

// Len to implement the sort interface.
func (privs PrivateKeys) Len() int {
	return len(privs)
}

// Swap to implement the sort interface.
func (privs PrivateKeys) Swap(i, j int) {
	privs[i], privs[j] = privs[j], privs[i]
}

// ByPubKey implements sort.Interface by providing Less and using the Len and
// Swap methods of the embedded PrivateKeys value.
type ByPubKey struct {
	PrivateKeys
}

// Less compares two private keys to determine order. The key are compared by
// deriving the public keys, and comparing lexicographically.
func (s ByPubKey) Less(i, j int) bool {
	pubKeyI := (*btcec.PublicKey)(&s.PrivateKeys[i].Key.PublicKey)
	pubKeyStrI := fmt.Sprintf("%x", pubKeyI.SerializeCompressed())

	pubKeyJ := (*btcec.PublicKey)(&s.PrivateKeys[j].Key.PublicKey)
	pubKeyStrJ := fmt.Sprintf("%x", pubKeyJ.SerializeCompressed())
	return pubKeyStrI < pubKeyStrJ
}

type PrivateKey struct {
	Key        *btcec.PrivateKey
	Compressed bool
}

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(rmgutil.Address) ([]PrivateKey, error)
}

// KeyClosure implements KeyDB with a closure.
type KeyClosure func(rmgutil.Address) ([]PrivateKey, error)

// GetKey implements KeyDB by returning the result of calling the closure.
func (kc KeyClosure) GetKey(address rmgutil.Address) ([]PrivateKey, error) {
	return kc(address)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates any
// user state required to get the scripts for an pay-to-script-hash address.
type ScriptDB interface {
	GetScript(rmgutil.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure.
type ScriptClosure func(rmgutil.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure.
func (sc ScriptClosure) GetScript(address rmgutil.Address) ([]byte, error) {
	return sc(address)
}

// SignTxOutput signs output idx of the given tx to resolve the script given in
// pkScript with a signature type of hashType. Any keys required will be
// looked up by calling getKey() with the string of the given address.
// Any pay-to-script-hash signatures will be similarly looked up by calling
// getScript. If previousScript is provided then the results in previousScript
// will be merged in a type-dependent manner with the newly generated.
// signature script.
func SignTxOutput(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int, inputAmt int64,
	pkScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte) ([]byte, error) {

	sigScript, class, addresses, nrequired, err := sign(chainParams, tx,
		idx, inputAmt, pkScript, hashType, kdb, sdb)
	if err != nil {
		return nil, err
	}

	if class == ScriptHashTy {
		// TODO keep the sub addressed and pass down to merge.
		realSigScript, _, _, _, err := sign(chainParams, tx, idx, inputAmt,
			sigScript, hashType, kdb, sdb)
		if err != nil {
			return nil, err
		}

		// Append the p2sh script as the last push in the script.
		builder := NewScriptBuilder()
		builder.AddOps(realSigScript)
		builder.AddData(sigScript)

		sigScript, _ = builder.Script()
		// TODO keep a copy of the script for merging.
	}

	// Merge scripts. with any previous data, if any.
	mergedScript := mergeScripts(chainParams, tx, idx, pkScript, class,
		addresses, nrequired, sigScript, previousScript)
	return mergedScript, nil
}
