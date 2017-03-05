// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript_test

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/bitgo/rmgd/blockchain"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/txscript"
	"github.com/bitgo/rmgd/wire"
	"testing"
)

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

type addressToKey struct {
	key        *btcec.PrivateKey
	compressed bool
}

func mkGetKey(keys map[string]addressToKey) txscript.KeyDB {
	if keys == nil {
		return txscript.KeyClosure(func(addr rmgutil.Address) ([]txscript.PrivateKey, error) {
			return nil, errors.New("nope")
		})
	}
	return txscript.KeyClosure(func(addr rmgutil.Address) ([]txscript.PrivateKey, error) {
		a2k, ok := keys[addr.EncodeAddress()]
		if !ok {
			return nil, errors.New("nope")
		}
		return []txscript.PrivateKey{txscript.PrivateKey{a2k.key, a2k.compressed}}, nil
	})
}

func mkGetScript(scripts map[string][]byte) txscript.ScriptDB {
	if scripts == nil {
		return txscript.ScriptClosure(func(addr rmgutil.Address) (
			[]byte, error) {
			return nil, errors.New("nope")
		})
	}
	return txscript.ScriptClosure(func(addr rmgutil.Address) ([]byte,
		error) {
		script, ok := scripts[addr.EncodeAddress()]
		if !ok {
			return nil, errors.New("nope")
		}
		return script, nil
	})
}

func checkScripts(msg string, tx *wire.MsgTx, idx int, inputAmt int64, sigScript []byte, pkScript []byte) error {
	tx.TxIn[idx].SignatureScript = sigScript

	// Before passing the script to the VM, we check whether it is an Prova script.
	pops, err := txscript.ParseScript(pkScript)
	if err != nil {
		return fmt.Errorf("failed to parse script %s: %v", msg, err)
	}
	keyView := blockchain.NewKeyViewpoint()

	//key ids
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})
	pubKey1, _ := btcec.ParsePubKey(hexToBytes("025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1"), btcec.S256())
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})
	pubKey2, _ := btcec.ParsePubKey(hexToBytes("038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202"), btcec.S256())

	keyView.SetKeyIDs(map[btcec.KeyID]*btcec.PublicKey{keyId1: pubKey1, keyId2: pubKey2})

	//admin key sets
	keySets := make(map[btcec.KeySetType]btcec.PublicKeySet)
	keySet, _ := btcec.ParsePubKeySet(btcec.S256(),
		"038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202", // priv 2b8c52b77b327c755b9b375500d3f4b2da9b0a1ff65f6891d311fe94295bc26a
		"025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1", // priv eaf02ca348c524e6392655ba4d29603cd1a7347d9d65cfe93ce1ebffdca22694
	)
	keySets[btcec.RootKeySet] = keySet
	keySets[btcec.ProvisionKeySet] = keySet
	keySets[btcec.IssueKeySet] = keySet

	keyView.SetKeys(keySets)
	// If script is Prova script, we replace all keyIDs with pubKeyHashes.
	if txscript.TypeOfScript(pops) == txscript.ProvaTy {
		keyIDs, err := txscript.ExtractKeyIDs(pops)
		keyIdMap := keyView.LookupKeyIDs(keyIDs)
		txscript.ReplaceKeyIDs(pops, keyIdMap)
		pkScript, err = txscript.UnparseScript(pops)
		if err != nil {
			return err
		}
	}

	// If script is Prova admin script, we replace the threadID with pubKeyHashes.
	if txscript.TypeOfScript(pops) == txscript.ProvaAdminTy {
		threadID, err := txscript.ExtractThreadID(pops)
		keyHashes, err := keyView.GetAdminKeyHashes(threadID)
		pkScript, err = txscript.ThreadPkScript(keyHashes)
		if err != nil {
			return err
		}
	}

	vm, err := txscript.NewEngine(pkScript, tx, idx,
		txscript.ScriptBip16|txscript.ScriptVerifyDERSignatures, nil, nil, inputAmt)
	if err != nil {
		return fmt.Errorf("failed to make script engine for %s: %v",
			msg, err)
	}

	err = vm.Execute()
	if err != nil {
		return fmt.Errorf("invalid script signature for %s: %v", msg,
			err)
	}

	return nil
}

func signAndCheck(msg string, tx *wire.MsgTx, idx int, inputAmt int64, pkScript []byte,
	hashType txscript.SigHashType, kdb txscript.KeyDB, sdb txscript.ScriptDB,
	previousScript []byte) error {
	sigScript, err := txscript.SignTxOutput(&chaincfg.TestNet3Params, tx,
		idx, inputAmt, pkScript, hashType, kdb, sdb, nil)
	if err != nil {
		return fmt.Errorf("failed to sign output %s: %v", msg, err)
	}

	return checkScripts(msg, tx, idx, inputAmt, sigScript, pkScript)
}

func TestSignTxOutput(t *testing.T) {
	t.Parallel()

	// make key
	// make script based on key.
	// sign with magic pixie dust.
	inputAmounts := []int64{5000000000, 5000000000, 5000000000}
	hash01, _ := chainhash.NewHashFromStr("08886fe11cc704bc617ebaf50f8bed16a66da84141d26d786a054f2c361c905a")
	hash02, _ := chainhash.NewHashFromStr("7fd6b408c31e2e1551c6285d9d3249e6263b6f2bc33c30d61a75f240caba902e")
	hash03, _ := chainhash.NewHashFromStr("9420d59e07d26a39eebadd79dea7f7e2f72d2fe0203fc8fb9a9e08f43fa9731e")
	tx := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  *hash01,
					Index: 0,
				},
				Sequence: 4294967295,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  *hash02,
					Index: 0,
				},
				Sequence: 4294967295,
			},
			{
				PreviousOutPoint: wire.OutPoint{
					Hash:  *hash03,
					Index: 0,
				},
				Sequence: 4294967295,
			},
		},
		TxOut: []*wire.TxOut{
			{
				Value: int64(1000000000),
			},
			{
				Value: int64(2000000000),
			},
			{
				Value: int64(11900000000),
			},
		},
		LockTime: 0,
	}

	//Prova Multisig
	//KeyID #1
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})
	key1, _ := btcec.PrivKeyFromBytes(btcec.S256(), []byte{
		0xea, 0xf0, 0x2c, 0xa3, 0x48, 0xc5, 0x24, 0xe6,
		0x39, 0x26, 0x55, 0xba, 0x4d, 0x29, 0x60, 0x3c,
		0xd1, 0xa7, 0x34, 0x7d, 0x9d, 0x65, 0xcf, 0xe9,
		0x3c, 0xe1, 0xeb, 0xff, 0xdc, 0xa2, 0x26, 0x94,
	})

	//KeyID #2
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})
	key2, _ := btcec.PrivKeyFromBytes(btcec.S256(), []byte{
		0x2b, 0x8c, 0x52, 0xb7, 0x7b, 0x32, 0x7c, 0x75,
		0x5b, 0x9b, 0x37, 0x55, 0x00, 0xd3, 0xf4, 0xb2,
		0xda, 0x9b, 0x0a, 0x1f, 0xf6, 0x5f, 0x68, 0x91,
		0xd3, 0x11, 0xfe, 0x94, 0x29, 0x5b, 0xc2, 0x6a,
	})
	hashType := txscript.SigHashAll
	for i := range tx.TxIn {
		msg := fmt.Sprintf("%d:%d", hashType, i)

		//dynamic key definition as usual
		key3, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Errorf("failed to make privKey for %s: %v",
				msg, err)
			break
		}
		pk3 := (*btcec.PublicKey)(&key3.PublicKey)
		pkHash := rmgutil.Hash160(pk3.SerializeCompressed())

		//Creation of Prova address
		addr, err := rmgutil.NewAddressProva(pkHash,
			[]btcec.KeyID{keyId1, keyId2}, &chaincfg.TestNet3Params)
		if err != nil {
			t.Errorf("failed to make Prova address for %s: %v",
				msg, err)
			break
		}

		scriptPkScript, err := txscript.PayToAddrScript(
			addr)
		if err != nil {
			t.Errorf("failed to make script pkscript for "+
				"%s: %v", msg, err)
			break
		}

		lookupKey := func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
			return []txscript.PrivateKey{
				txscript.PrivateKey{key1, true},
				txscript.PrivateKey{key2, true},
				txscript.PrivateKey{key3, true},
			}, nil
		}

		if err := signAndCheck(msg, tx, i, inputAmounts[i], scriptPkScript,
			hashType, txscript.KeyClosure(lookupKey), nil, nil); err != nil {
			t.Error(err)
			break
		}
	}

	// Two part Prova Multisig, sign with one key then the other.
	for i := range tx.TxIn {
		msg := fmt.Sprintf("%d:%d", hashType, i)

		//dynamic key definition as usual
		key3, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Errorf("failed to make privKey for %s: %v",
				msg, err)
			break
		}
		pk3 := (*btcec.PublicKey)(&key3.PublicKey)
		pkHash := rmgutil.Hash160(pk3.SerializeCompressed())

		//Creation of Prova address
		addr, err := rmgutil.NewAddressProva(pkHash,
			[]btcec.KeyID{keyId1, keyId2}, &chaincfg.TestNet3Params)
		if err != nil {
			t.Errorf("failed to make Prova address for %s: %v",
				msg, err)
			break
		}

		scriptPkScript, err := txscript.PayToAddrScript(
			addr)
		if err != nil {
			t.Errorf("failed to make script pkscript for "+
				"%s: %v", msg, err)
			break
		}

		lookupKey := func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
			return []txscript.PrivateKey{
				txscript.PrivateKey{key1, true},
			}, nil
		}

		sigScript, err := txscript.SignTxOutput(
			&chaincfg.TestNet3Params, tx, i, inputAmounts[i], scriptPkScript,
			hashType, txscript.KeyClosure(lookupKey), nil, nil)
		if err != nil {
			t.Errorf("failed to sign output %s: %v", msg,
				err)
			break
		}

		// Only 1 out of 2 signed, this *should* fail.
		if checkScripts(msg, tx, i, inputAmounts[i], sigScript,
			scriptPkScript) == nil {
			t.Errorf("part signed script valid for %s", msg)
			break
		}

		lookupKey = func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
			return []txscript.PrivateKey{
				txscript.PrivateKey{key2, true},
			}, nil
		}

		// Sign with the other key and merge
		sigScript, err = txscript.SignTxOutput(
			&chaincfg.TestNet3Params, tx, i, inputAmounts[i], scriptPkScript,
			hashType, txscript.KeyClosure(lookupKey), nil, sigScript)
		if err != nil {
			t.Errorf("failed to sign output %s: %v", msg, err)
			break
		}

		err = checkScripts(msg, tx, i, inputAmounts[i], sigScript,
			scriptPkScript)
		if err != nil {
			t.Errorf("fully signed script invalid for "+
				"%s: %v", msg, err)
			break
		}
	}

	// Basic Check Thread
	for i := range tx.TxIn {
		threadID := rmgutil.ThreadID(i)
		msg := fmt.Sprintf("%d:%d", hashType, i)

		scriptPkScript, err := txscript.ProvaThreadScript(threadID)
		if err != nil {
			t.Errorf("failed to make pkscript "+
				"for %s: %v", msg, err)
		}

		lookupKey := func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
			return []txscript.PrivateKey{
				txscript.PrivateKey{key1, true},
				txscript.PrivateKey{key2, true},
			}, nil
		}

		if err := signAndCheck(msg, tx, i, inputAmounts[i], scriptPkScript,
			hashType, txscript.KeyClosure(lookupKey), nil, nil); err != nil {
			t.Error(err)
			break
		}
	}

	// Two part Check Thread, sign with one key then the other.
	for i := range tx.TxIn {
		threadID := rmgutil.ThreadID(i)
		msg := fmt.Sprintf("%d:%d", hashType, i)

		scriptPkScript, err := txscript.ProvaThreadScript(threadID)
		if err != nil {
			t.Errorf("failed to make pkscript "+
				"for %s: %v", msg, err)
		}

		lookupKey := func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
			return []txscript.PrivateKey{
				txscript.PrivateKey{key1, true},
			}, nil
		}

		sigScript, err := txscript.SignTxOutput(
			&chaincfg.TestNet3Params, tx, i, inputAmounts[i], scriptPkScript,
			hashType, txscript.KeyClosure(lookupKey), nil, nil)
		if err != nil {
			t.Errorf("failed to sign output %s: %v", msg,
				err)
			break
		}

		// Only 1 out of 2 signed, this *should* fail.
		if checkScripts(msg, tx, i, inputAmounts[i], sigScript,
			scriptPkScript) == nil {
			t.Errorf("part signed script valid for %s", msg)
			break
		}

		lookupKey = func(a rmgutil.Address) ([]txscript.PrivateKey, error) {
			return []txscript.PrivateKey{
				txscript.PrivateKey{key2, true},
			}, nil
		}

		// Sign with the other key and merge
		sigScript, err = txscript.SignTxOutput(
			&chaincfg.TestNet3Params, tx, i, inputAmounts[i], scriptPkScript,
			hashType, txscript.KeyClosure(lookupKey), nil, sigScript)
		if err != nil {
			t.Errorf("failed to sign output %s: %v", msg, err)
			break
		}

		err = checkScripts(msg, tx, i, inputAmounts[i], sigScript,
			scriptPkScript)
		if err != nil {
			t.Errorf("fully signed script invalid for "+
				"%s: %v", msg, err)
			break
		}
	}
}

type tstInput struct {
	txout              *wire.TxOut
	sigscriptGenerates bool
	inputValidates     bool
	indexOutOfRange    bool
}

type tstSigScript struct {
	name               string
	inputs             []tstInput
	hashType           txscript.SigHashType
	compress           bool
	scriptAtWrongIndex bool
}

var coinbaseOutPoint = &wire.OutPoint{
	Index: (1 << 32) - 1,
}

// Pregenerated private key, with associated public key and pkScripts
// for the uncompressed and compressed hash160.
var (
	privKeyD = []byte{0x6b, 0x0f, 0xd8, 0xda, 0x54, 0x22, 0xd0, 0xb7,
		0xb4, 0xfc, 0x4e, 0x55, 0xd4, 0x88, 0x42, 0xb3, 0xa1, 0x65,
		0xac, 0x70, 0x7f, 0x3d, 0xa4, 0x39, 0x5e, 0xcb, 0x3b, 0xb0,
		0xd6, 0x0e, 0x06, 0x92}
	pubkeyX = []byte{0xb2, 0x52, 0xf0, 0x49, 0x85, 0x78, 0x03, 0x03, 0xc8,
		0x7d, 0xce, 0x51, 0x7f, 0xa8, 0x69, 0x0b, 0x91, 0x95, 0xf4,
		0xf3, 0x5c, 0x26, 0x73, 0x05, 0x05, 0xa2, 0xee, 0xbc, 0x09,
		0x38, 0x34, 0x3a}
	pubkeyY = []byte{0xb7, 0xc6, 0x7d, 0xb2, 0xe1, 0xff, 0xc8, 0x43, 0x1f,
		0x63, 0x32, 0x62, 0xaa, 0x60, 0xc6, 0x83, 0x30, 0xbd, 0x24,
		0x7e, 0xef, 0xdb, 0x6f, 0x2e, 0x8d, 0x56, 0xf0, 0x3c, 0x9f,
		0x6d, 0xb6, 0xf8}
	uncompressedPkScript = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x86, 0xc9, 0xfa, 0x88, 0xac}
	compressedPkScript = []byte{0x76, 0xa9, 0x14, 0x27, 0x4d, 0x9f, 0x7f,
		0x61, 0x7e, 0x7c, 0x7a, 0x1c, 0x1f, 0xb2, 0x75, 0x79, 0x10,
		0x43, 0x65, 0x68, 0x27, 0x9d, 0x86, 0x88, 0xac}
	shortPkScript = []byte{0x76, 0xa9, 0x14, 0xd1, 0x7c, 0xb5,
		0xeb, 0xa4, 0x02, 0xcb, 0x68, 0xe0, 0x69, 0x56, 0xbf, 0x32,
		0x53, 0x90, 0x0e, 0x0a, 0x88, 0xac}
	uncompressedAddrStr = "1L6fd93zGmtzkK6CsZFVVoCwzZV3MUtJ4F"
	compressedAddrStr   = "14apLppt9zTq6cNw8SDfiJhk9PhkZrQtYZ"
)

// Pretend output amounts.
const coinbaseVal = 2500000000
const fee = 5000000

var sigScriptTests = []tstSigScript{
	{
		name: "one input uncompressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs uncompressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "one input compressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "two inputs compressed",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, compressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashNone",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashNone,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashSingle",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashSingle,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType SigHashAnyoneCanPay",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAnyOneCanPay,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "hashType non-standard",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           0x04,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "invalid compression",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     false,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           true,
		scriptAtWrongIndex: false,
	},
	{
		name: "short PkScript",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, shortPkScript),
				sigscriptGenerates: false,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: false,
	},
	{
		name: "valid script at wrong index",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
	{
		name: "index out of range",
		inputs: []tstInput{
			{
				txout:              wire.NewTxOut(coinbaseVal, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
			{
				txout:              wire.NewTxOut(coinbaseVal+fee, uncompressedPkScript),
				sigscriptGenerates: true,
				inputValidates:     true,
				indexOutOfRange:    false,
			},
		},
		hashType:           txscript.SigHashAll,
		compress:           false,
		scriptAtWrongIndex: true,
	},
}

// Test the sigscript generation for valid and invalid inputs, all
// hashTypes, and with and without compression.  This test creates
// sigscripts to spend fake coinbase inputs, as sigscripts cannot be
// created for the MsgTxs in txTests, since they come from the blockchain
// and we don't have the private keys.
func TestSignatureScript(t *testing.T) {
	t.Parallel()

	privKey, _ := btcec.PrivKeyFromBytes(btcec.S256(), privKeyD)

nexttest:
	for i := range sigScriptTests {
		tx := wire.NewMsgTx()

		output := wire.NewTxOut(500, []byte{txscript.OP_RETURN})
		tx.AddTxOut(output)

		for range sigScriptTests[i].inputs {
			txin := wire.NewTxIn(coinbaseOutPoint, nil)
			tx.AddTxIn(txin)
		}

		var script []byte
		var err error
		for j := range tx.TxIn {
			var idx int
			if sigScriptTests[i].inputs[j].indexOutOfRange {
				t.Errorf("at test %v", sigScriptTests[i].name)
				idx = len(sigScriptTests[i].inputs)
			} else {
				idx = j
			}
			script, err = txscript.SignatureScript(tx, idx,
				sigScriptTests[i].inputs[j].txout.PkScript,
				sigScriptTests[i].hashType, privKey,
				sigScriptTests[i].compress)

			if (err == nil) != sigScriptTests[i].inputs[j].sigscriptGenerates {
				if err == nil {
					t.Errorf("passed test '%v' incorrectly",
						sigScriptTests[i].name)
				} else {
					t.Errorf("failed test '%v': %v",
						sigScriptTests[i].name, err)
				}
				continue nexttest
			}
			if !sigScriptTests[i].inputs[j].sigscriptGenerates {
				// done with this test
				continue nexttest
			}

			tx.TxIn[j].SignatureScript = script
		}

		// If testing using a correct sigscript but for an incorrect
		// index, use last input script for first input.  Requires > 0
		// inputs for test.
		if sigScriptTests[i].scriptAtWrongIndex {
			tx.TxIn[0].SignatureScript = script
			sigScriptTests[i].inputs[0].inputValidates = false
		}

		// Validate tx input scripts
		scriptFlags := txscript.ScriptBip16 | txscript.ScriptVerifyDERSignatures
		for j := range tx.TxIn {
			vm, err := txscript.NewEngine(sigScriptTests[i].
				inputs[j].txout.PkScript, tx, j, scriptFlags, nil, nil, 0)
			if err != nil {
				t.Errorf("cannot create script vm for test %v: %v",
					sigScriptTests[i].name, err)
				continue nexttest
			}
			err = vm.Execute()
			if (err == nil) != sigScriptTests[i].inputs[j].inputValidates {
				if err == nil {
					t.Errorf("passed test '%v' validation incorrectly: %v",
						sigScriptTests[i].name, err)
				} else {
					t.Errorf("failed test '%v' validation: %v",
						sigScriptTests[i].name, err)
				}
				continue nexttest
			}
		}
	}
}
