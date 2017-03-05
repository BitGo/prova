// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"bytes"
	"github.com/bitgo/rmgd/blockchain"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/txscript"
	"github.com/bitgo/rmgd/wire"
	"testing"
)

// TestCalcMinRequiredTxRelayFee tests the calcMinRequiredTxRelayFee API.
func TestCalcMinRequiredTxRelayFee(t *testing.T) {
	tests := []struct {
		name     string         // test description.
		size     int64          // Transaction size in bytes.
		relayFee rmgutil.Amount // minimum relay transaction fee.
		want     int64          // Expected fee.
	}{
		{
			// Ensure combination of size and fee that are less than 1000
			// produce a non-zero fee.
			"250 bytes with relay fee of 3",
			250,
			3,
			3,
		},
		{
			"100 bytes with default minimum relay fee",
			100,
			DefaultMinRelayTxFee,
			0,
		},
		{
			"max standard tx size with default minimum relay fee",
			maxStandardTxSize,
			DefaultMinRelayTxFee,
			0,
		},
		{
			"max standard tx size with max atoms relay fee",
			maxStandardTxSize,
			rmgutil.MaxAtoms,
			rmgutil.MaxAtoms,
		},
		{
			"1500 bytes with 5000 relay fee",
			1500,
			5000,
			7500,
		},
		{
			"1500 bytes with 3000 relay fee",
			1500,
			3000,
			4500,
		},
		{
			"782 bytes with 5000 relay fee",
			782,
			5000,
			3910,
		},
		{
			"782 bytes with 3000 relay fee",
			782,
			3000,
			2346,
		},
		{
			"782 bytes with 2550 relay fee",
			782,
			2550,
			1994,
		},
	}

	for _, test := range tests {
		got := calcMinRequiredTxRelayFee(test.size, test.relayFee)
		if got != test.want {
			t.Errorf("TestCalcMinRequiredTxRelayFee test '%s' "+
				"failed: got %v want %v", test.name, got,
				test.want)
			continue
		}
	}
}

// TestCheckPkScriptStandard tests the checkPkScriptStandard API.
func TestCheckPkScriptStandard(t *testing.T) {
	var pubKeys [][]byte
	var pubKeyHashes [][]byte
	for i := 0; i < 4; i++ {
		pk, err := btcec.NewPrivateKey(btcec.S256())
		if err != nil {
			t.Fatalf("TestCheckPkScriptStandard NewPrivateKey failed: %v",
				err)
			return
		}
		pubKeyHashes = append(pubKeyHashes, rmgutil.Hash160(pk.PubKey().SerializeCompressed()))
		pubKeys = append(pubKeys, pk.PubKey().SerializeCompressed())
	}
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})

	tests := []struct {
		name       string // test description.
		script     *txscript.ScriptBuilder
		isStandard bool
	}{
		{
			"2 of pkHash, keyID1, keyID2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeyHashes[0]).AddInt64(int64(keyId1)).AddInt64(int64(keyId2)).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			true,
		},
		{
			"2 of pkHash, keyID1, keyID2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeyHashes[0]).AddInt64(int64(keyId2)).AddInt64(int64(keyId1)).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			true,
		},
		{
			"2 of pkHash, keyID1",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeyHashes[0]).AddInt64(int64(keyId1)).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"keyID1 double",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeyHashes[0]).AddInt64(int64(keyId1)).AddInt64(int64(keyId1)).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"pkHash after KeyIDs",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddInt64(int64(keyId1)).AddInt64(int64(keyId1)).AddData(pubKeyHashes[0]).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"malformed2",
			txscript.NewScriptBuilder().AddOp(txscript.OP_2).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_3).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"malformed3",
			txscript.NewScriptBuilder().AddOp(txscript.OP_0).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_2).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"malformed4",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_0).AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"malformed5",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]).
				AddOp(txscript.OP_CHECKSAFEMULTISIG),
			false,
		},
		{
			"malformed6",
			txscript.NewScriptBuilder().AddOp(txscript.OP_1).
				AddData(pubKeys[0]).AddData(pubKeys[1]),
			false,
		},
	}

	for _, test := range tests {
		script, err := test.script.Script()
		if err != nil {
			t.Fatalf("TestCheckPkScriptStandard test '%s' "+
				"failed: %v", test.name, err)
			continue
		}
		scriptClass := txscript.GetScriptClass(script)
		got := checkPkScriptStandard(script, scriptClass)
		if (test.isStandard && got != nil) ||
			(!test.isStandard && got == nil) {

			t.Fatalf("TestCheckPkScriptStandard test '%s' failed",
				test.name)
			return
		}
	}
}

// TestDust tests the isDust API.
func TestDust(t *testing.T) {
	pkScript := []byte{0x76, 0xa9, 0x21, 0x03, 0x2f, 0x7e, 0x43,
		0x0a, 0xa4, 0xc9, 0xd1, 0x59, 0x43, 0x7e, 0x84, 0xb9,
		0x75, 0xdc, 0x76, 0xd9, 0x00, 0x3b, 0xf0, 0x92, 0x2c,
		0xf3, 0xaa, 0x45, 0x28, 0x46, 0x4b, 0xab, 0x78, 0x0d,
		0xba, 0x5e, 0x88, 0xac}

	tests := []struct {
		name     string // test description
		txOut    wire.TxOut
		relayFee rmgutil.Amount // minimum relay transaction fee.
		isDust   bool
	}{
		{
			// Any value is allowed with a zero relay fee.
			"zero value with zero relay fee",
			wire.TxOut{Value: 0, PkScript: pkScript},
			0,
			false,
		},
		{
			// Zero value is dust with any relay fee"
			"zero value with very small tx fee",
			wire.TxOut{Value: 0, PkScript: pkScript},
			1,
			true,
		},
		{
			"38 byte public key script with value 584",
			wire.TxOut{Value: 584, PkScript: pkScript},
			1000,
			true,
		},
		{
			"38 byte public key script with value 585",
			wire.TxOut{Value: 585, PkScript: pkScript},
			1000,
			false,
		},
		{
			// Maximum allowed value is never dust.
			"max atoms amount is never dust",
			wire.TxOut{Value: rmgutil.MaxAtoms, PkScript: pkScript},
			rmgutil.MaxAtoms,
			false,
		},
		{
			// Maximum int64 value causes overflow.
			"maximum int64 value",
			wire.TxOut{Value: 1<<63 - 1, PkScript: pkScript},
			1<<63 - 1,
			true,
		},
		{
			// Unspendable pkScript due to an invalid public key
			// script.
			"unspendable pkScript",
			wire.TxOut{Value: 5000, PkScript: []byte{0x01}},
			0, // no relay fee
			true,
		},
	}
	for _, test := range tests {
		res := isDust(&test.txOut, test.relayFee)
		if res != test.isDust {
			t.Fatalf("Dust test '%s' failed: want %v got %v",
				test.name, test.isDust, res)
			continue
		}
	}
}

// TestCheckTransactionStandard tests the checkTransactionStandard API.
func TestCheckTransactionStandard(t *testing.T) {
	// Create some dummy, but otherwise standard, data for transactions.
	prevOutHash, err := chainhash.NewHashFromStr("01")
	if err != nil {
		t.Fatalf("NewShaHashFromStr: unexpected error: %v", err)
	}
	dummyPrevOut := wire.OutPoint{Hash: *prevOutHash, Index: 1}
	dummySigScript := bytes.Repeat([]byte{0x00}, 65)
	dummyTxIn := wire.TxIn{
		PreviousOutPoint: dummyPrevOut,
		SignatureScript:  dummySigScript,
		Sequence:         wire.MaxTxInSequenceNum,
	}
	addrHash := [20]byte{0x01}
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})
	addr, err := rmgutil.NewAddressProva(addrHash[:],
		[]btcec.KeyID{keyId1, keyId2}, &chaincfg.TestNet3Params)
	if err != nil {
		t.Fatalf("NewAddressPubKeyHash: unexpected error: %v", err)
	}
	dummyPkScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("PayToAddrScript: unexpected error: %v", err)
	}
	dummyTxOut := wire.TxOut{
		Value:    100000000, // 100 RMG
		PkScript: dummyPkScript,
	}

	// Create some dummy admin op output.
	_, pubKey := btcec.PrivKeyFromBytes(btcec.S256(), []byte{
		0x2b, 0x8c, 0x52, 0xb7, 0x7b, 0x32, 0x7c, 0x75,
		0x5b, 0x9b, 0x37, 0x55, 0x00, 0xd3, 0xf4, 0xb2,
		0xda, 0x9b, 0x0a, 0x1f, 0xf6, 0x5f, 0x68, 0x91,
		0xd3, 0x11, 0xfe, 0x94, 0x29, 0x5b, 0xc2, 0x6a,
	})
	data := make([]byte, 1+btcec.PubKeyBytesLenCompressed)
	data[0] = txscript.AdminOpProvisionKeyAdd
	copy(data[1:], pubKey.SerializeCompressed())
	adminOpPkScript, _ := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data).Script()
	adminOpTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: adminOpPkScript,
	}
	// create root tx out
	rootPkScript, _ := txscript.ProvaThreadScript(rmgutil.RootThread)
	rootTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: rootPkScript,
	}
	// create provision tx out
	provisionPkScript, _ := txscript.ProvaThreadScript(rmgutil.ProvisionThread)
	provisionTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: provisionPkScript,
	}

	tests := []struct {
		name       string
		tx         wire.MsgTx
		height     uint32
		isStandard bool
		code       wire.RejectCode
	}{
		{
			name: "Typical pay-to-pubkey-hash transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Transaction version too high",
			tx: wire.MsgTx{
				Version:  wire.TxVersion + 1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Transaction is not finalized",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript:  dummySigScript,
					Sequence:         0,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 300001,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Transaction size is too large",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value: 0,
					PkScript: bytes.Repeat([]byte{0x00},
						maxStandardTxSize+1),
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Signature script size is too large",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript: bytes.Repeat([]byte{0x00},
						maxStandardSigScriptSize+1),
					Sequence: wire.MaxTxInSequenceNum,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Signature script that does more than push data",
			tx: wire.MsgTx{
				Version: 1,
				TxIn: []*wire.TxIn{{
					PreviousOutPoint: dummyPrevOut,
					SignatureScript: []byte{
						txscript.OP_CHECKSIGVERIFY},
					Sequence: wire.MaxTxInSequenceNum,
				}},
				TxOut:    []*wire.TxOut{&dummyTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Valid but non standard public key script",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    100000000,
					PkScript: []byte{txscript.OP_TRUE},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "More than one nulldata output",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectNonstandard,
		},
		{
			name: "Dust output",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: dummyPkScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "One nulldata output with 0 amount (standard)",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{{
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "Typical admin transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&rootTxOut, &adminOpTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "admin transaction with thread output at pos 1.",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&adminOpTxOut, &rootTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectInvalid,
		},
		{
			name: "admin transaction with non-zero output value.",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{&rootTxOut, {
					Value:    500,
					PkScript: adminOpPkScript,
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectInvalid,
		},
		{
			name: "admin transaction with more than 1 input.",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn, &dummyTxIn},
				TxOut:    []*wire.TxOut{&rootTxOut, &adminOpTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectInvalid,
		},
		{
			name: "Empty admin transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&rootTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectInvalid,
		},
		{
			name: "Admin transaction with operation on wrong thread",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&dummyTxIn},
				TxOut:    []*wire.TxOut{&provisionTxOut, &adminOpTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectInvalid,
		},
		{
			name: "Admin transaction with invalid operation",
			tx: wire.MsgTx{
				Version: 1,
				TxIn:    []*wire.TxIn{&dummyTxIn},
				TxOut: []*wire.TxOut{&rootTxOut, {
					Value:    0,
					PkScript: []byte{txscript.OP_RETURN},
				}},
				LockTime: 0,
			},
			height:     300000,
			isStandard: false,
			code:       wire.RejectInvalid,
		},
	}

	timeSource := blockchain.NewMedianTime()
	for _, test := range tests {
		// Ensure standardness is as expected.
		err := checkTransactionStandard(rmgutil.NewTx(&test.tx),
			test.height, timeSource, DefaultMinRelayTxFee)
		if err == nil && test.isStandard {
			// Test passes since function returned standard for a
			// transaction which is intended to be standard.
			continue
		}
		if err == nil && !test.isStandard {
			t.Errorf("checkTransactionStandard (%s): standard when "+
				"it should not be", test.name)
			continue
		}
		if err != nil && test.isStandard {
			t.Errorf("checkTransactionStandard (%s): nonstandard "+
				"when it should not be: %v", test.name, err)
			continue
		}

		// Ensure error type is a TxRuleError inside of a RuleError.
		rerr, ok := err.(RuleError)
		if !ok {
			t.Errorf("checkTransactionStandard (%s): unexpected "+
				"error type - got %T", test.name, err)
			continue
		}
		txrerr, ok := rerr.Err.(TxRuleError)
		if !ok {
			t.Errorf("checkTransactionStandard (%s): unexpected "+
				"error type - got %T", test.name, rerr.Err)
			continue
		}

		// Ensure the reject code is the expected one.
		if txrerr.RejectCode != test.code {
			t.Errorf("checkTransactionStandard (%s): unexpected "+
				"error code - got %v, want %v", test.name,
				txrerr.RejectCode, test.code)
			continue
		}
	}
}

// TestCheckAdminTransactionStandard tests the checkInputsStandard API
// with admin transactions.
func TestCheckAdminTransactionStandard(t *testing.T) {
	// Create some dummy sig scripts.
	coinbaseSigScript, _ := txscript.NewScriptBuilder().AddInt64(int64(300000)).
		AddInt64(int64(0)).Script()
	coinbaseTxIn := wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		SignatureScript: coinbaseSigScript,
		Sequence:        wire.MaxTxInSequenceNum,
	}
	testSigScript := bytes.Repeat([]byte{0x00}, 4)
	// Create some dummy admin op output.
	adminOpPkScript, _ := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(bytes.Repeat([]byte{0x00}, 4)).Script()
	adminOpTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: adminOpPkScript,
	}
	// create root tip tx
	rootPkScript, _ := txscript.ProvaThreadScript(rmgutil.RootThread)
	rootTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: rootPkScript,
	}
	rootTipTx := rmgutil.NewTx(&wire.MsgTx{
		Version:  1,
		TxIn:     []*wire.TxIn{&coinbaseTxIn},
		TxOut:    []*wire.TxOut{&rootTxOut},
		LockTime: 0,
	})
	rootPrevOut := wire.OutPoint{Hash: *rootTipTx.Hash(), Index: 0}
	rootTxIn := wire.TxIn{
		PreviousOutPoint: rootPrevOut,
		SignatureScript:  testSigScript,
		Sequence:         wire.MaxTxInSequenceNum,
	}
	// create provision tip tx
	provisionPkScript, _ := txscript.ProvaThreadScript(rmgutil.ProvisionThread)
	provisionTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: provisionPkScript,
	}
	provisionTipTx := rmgutil.NewTx(&wire.MsgTx{
		Version:  1,
		TxIn:     []*wire.TxIn{&coinbaseTxIn},
		TxOut:    []*wire.TxOut{&provisionTxOut},
		LockTime: 0,
	})
	provisionPrevOut := wire.OutPoint{Hash: *provisionTipTx.Hash(), Index: 0}
	provisionTxIn := wire.TxIn{
		PreviousOutPoint: provisionPrevOut,
		SignatureScript:  testSigScript,
		Sequence:         wire.MaxTxInSequenceNum,
	}
	// create issue tip tx
	issuePkScript, _ := txscript.ProvaThreadScript(rmgutil.IssueThread)
	issueTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: issuePkScript,
	}
	issueTipTx := rmgutil.NewTx(&wire.MsgTx{
		Version:  1,
		TxIn:     []*wire.TxIn{&coinbaseTxIn},
		TxOut:    []*wire.TxOut{&issueTxOut},
		LockTime: 0,
	})
	issuePrevOut := wire.OutPoint{Hash: *issueTipTx.Hash(), Index: 0}
	issueTxIn := wire.TxIn{
		PreviousOutPoint: issuePrevOut,
		SignatureScript:  testSigScript,
		Sequence:         wire.MaxTxInSequenceNum,
	}
	// Create prova txout
	keyId1 := btcec.KeyIDFromAddressBuffer([]byte{1, 0, 0, 0})
	keyId2 := btcec.KeyIDFromAddressBuffer([]byte{0, 0, 1, 0})
	payAddr, _ := rmgutil.NewAddressProva(make([]byte, 20), []btcec.KeyID{keyId1, keyId2}, &chaincfg.RegressionNetParams)
	provaPkScript, _ := txscript.PayToAddrScript(payAddr)
	provaTxOut := wire.TxOut{
		Value:    0, // 0 RMG
		PkScript: provaPkScript,
	}
	provaTx := rmgutil.NewTx(&wire.MsgTx{
		Version:  1,
		TxIn:     []*wire.TxIn{&coinbaseTxIn},
		TxOut:    []*wire.TxOut{&provaTxOut},
		LockTime: 0,
	})
	provaPrevOut := wire.OutPoint{Hash: *provaTx.Hash(), Index: 0}
	provaTxIn := wire.TxIn{
		PreviousOutPoint: provaPrevOut,
		SignatureScript:  testSigScript,
		Sequence:         wire.MaxTxInSequenceNum,
	}
	// add all tips to utxoview
	utxoView := blockchain.NewUtxoViewpoint()
	utxoView.AddTxOuts(rootTipTx, 0)
	utxoView.AddTxOuts(provisionTipTx, 0)
	utxoView.AddTxOuts(issueTipTx, 0)
	utxoView.AddTxOuts(provaTx, 0)

	tests := []struct {
		name       string
		tx         wire.MsgTx
		height     uint32
		isStandard bool
		code       wire.RejectCode
	}{
		{
			name: "empty root thread transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&rootTxIn},
				TxOut:    []*wire.TxOut{&rootTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "standard provision thread transaction",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&provisionTxIn},
				TxOut:    []*wire.TxOut{&provisionTxOut, &adminOpTxOut},
				LockTime: 0,
			},
			height:     300000,
			isStandard: true,
		},
		{
			name: "spend wrong thread",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&rootTxIn},
				TxOut:    []*wire.TxOut{&provisionTxOut},
				LockTime: 0,
			},
			height:     300000,
			code:       wire.RejectInvalidAdmin,
			isStandard: false,
		},
		{
			name: "spend prova output with admin thread",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&provaTxIn},
				TxOut:    []*wire.TxOut{&provisionTxOut},
				LockTime: 0,
			},
			height:     300000,
			code:       wire.RejectInvalidAdmin,
			isStandard: false,
		},
		{
			name: "spend with second input",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&provaTxIn, &issueTxIn},
				TxOut:    []*wire.TxOut{&issueTxOut},
				LockTime: 0,
			},
			height:     300000,
			code:       wire.RejectInvalidAdmin,
			isStandard: false,
		},
		{
			name: "spend admin thread with prova",
			tx: wire.MsgTx{
				Version:  1,
				TxIn:     []*wire.TxIn{&issueTxIn},
				TxOut:    []*wire.TxOut{&provaTxOut},
				LockTime: 0,
			},
			height:     300000,
			code:       wire.RejectInvalidAdmin,
			isStandard: false,
		},
	}

	for _, test := range tests {
		// Ensure standardness is as expected.
		err := checkInputsStandard(rmgutil.NewTx(&test.tx), utxoView)
		if err == nil && test.isStandard {
			// Test passes since function returned standard for a
			// transaction which is intended to be standard.
			continue
		}
		if err == nil && !test.isStandard {
			t.Errorf("checkTransactionStandard (%s): standard when "+
				"it should not be", test.name)
			continue
		}
		if err != nil && test.isStandard {
			t.Errorf("checkTransactionStandard (%s): nonstandard "+
				"when it should not be: %v", test.name, err)
			continue
		}

		// Ensure error type is a TxRuleError inside of a RuleError.
		rerr, ok := err.(RuleError)
		if !ok {
			t.Errorf("checkTransactionStandard (%s): unexpected "+
				"error type - got %T", test.name, err)
			continue
		}
		txrerr, ok := rerr.Err.(TxRuleError)
		if !ok {
			t.Errorf("checkTransactionStandard (%s): unexpected "+
				"error type - got %T", test.name, rerr.Err)
			continue
		}

		// Ensure the reject code is the expected one.
		if txrerr.RejectCode != test.code {
			t.Errorf("checkTransactionStandard (%s): unexpected "+
				"error code - got %v, want %v", test.name,
				txrerr.RejectCode, test.code)
			continue
		}
	}
}
