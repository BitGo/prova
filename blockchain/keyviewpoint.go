// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/rmgutil"
	"github.com/bitgo/rmgd/txscript"
	"github.com/bitgo/rmgd/wire"
)

// KeyViewpoint represents a view into the set of admin keys from a specific
// point of view in the chain. For example, it could be for the end of the main
// chain, some point in the history of the main chain, or down a side chain.
type KeyViewpoint struct {
	threadTips   map[rmgutil.ThreadID]*wire.OutPoint
	lastKeyID    btcec.KeyID
	totalSupply  uint64
	adminKeySets map[btcec.KeySetType]btcec.PublicKeySet
	aspKeyIdMap  btcec.KeyIdMap
}

// ThreadTips returns
func (view *KeyViewpoint) ThreadTips() map[rmgutil.ThreadID]*wire.OutPoint {
	return view.threadTips
}

// SetThreadTips sets the tips of the admin threads.
// The passed reference is deep copied, so modification does not affect
// source data structures.
func (view *KeyViewpoint) SetThreadTips(
	threadTips map[rmgutil.ThreadID]*wire.OutPoint) {
	view.threadTips = rmgutil.CopyThreadTips(threadTips)
}

// LastKeyID
func (view *KeyViewpoint) LastKeyID() btcec.KeyID {
	return view.lastKeyID
}

// SetLastKeyID
func (view *KeyViewpoint) SetLastKeyID(lastKeyID btcec.KeyID) {
	view.lastKeyID = lastKeyID
}

// TotalSupply
func (view *KeyViewpoint) TotalSupply() uint64 {
	return view.totalSupply
}

// SetTotalSupply
func (view *KeyViewpoint) SetTotalSupply(totalSupply uint64) {
	view.totalSupply = totalSupply
}

// SetKeys sets the admin key sets at the position in the chain the view
// curretly represents.
func (view *KeyViewpoint) SetKeys(keys map[btcec.KeySetType]btcec.PublicKeySet) {
	if keys != nil {
		view.adminKeySets = btcec.DeepCopy(keys)
	}
}

// Keys returns the set current admin key sets.
func (view *KeyViewpoint) Keys() map[btcec.KeySetType]btcec.PublicKeySet {
	return view.adminKeySets
}

// GetAdminKeyHashes returns pubKeyHashes according to the provided threadID.
func (view *KeyViewpoint) GetAdminKeyHashes(threadID rmgutil.ThreadID) ([][]byte, error) {

	if threadID > rmgutil.IssueThread {
		return nil, fmt.Errorf("unknown threadID %v", threadID)
	}

	pubs := view.adminKeySets[btcec.KeySetType(threadID)]
	hashes := make([][]byte, len(pubs))
	for i, pubKey := range pubs {
		hashes[i] = rmgutil.Hash160(pubKey.SerializeCompressed())
	}
	return hashes, nil
}

// SetKeyIDs sets the mapping of keyIDs to ASP keys.
func (view *KeyViewpoint) SetKeyIDs(aspKeyIdMap btcec.KeyIdMap) {
	if aspKeyIdMap != nil {
		view.aspKeyIdMap = aspKeyIdMap.DeepCopy()
	}
}

// KeyIDs returns a mapping of keyIDs to ASP keys at the position in the chain
// the view currently represents.
func (view *KeyViewpoint) KeyIDs() btcec.KeyIdMap {
	return view.aspKeyIdMap
}

// LookupKeyIDs returns pubKeyHashes for all registered KeyIDs
func (view *KeyViewpoint) LookupKeyIDs(keyIDs []btcec.KeyID) map[btcec.KeyID][]byte {
	keyIdMap := make(map[btcec.KeyID][]byte)
	for _, keyID := range keyIDs {
		pubKey := view.aspKeyIdMap[keyID]
		if pubKey != nil {
			keyIdMap[keyID] = rmgutil.Hash160(pubKey.SerializeCompressed())
		}
	}
	return keyIdMap
}

// ProcessAdminOuts finds admin transactions and executes all ops in it.
// This function is called after the validity of the transaction has been
// verified.
func (view *KeyViewpoint) ProcessAdminOuts(tx *rmgutil.Tx, blockHeight uint32) {
	threadInt, adminOutputs := txscript.GetAdminDetails(tx)
	if threadInt < 0 {
		// not admin transaction
		return // so we skip.
	}
	if rmgutil.ThreadID(threadInt) == rmgutil.IssueThread {
		isDestruction := len(tx.MsgTx().TxIn) > 1
		if isDestruction {
			// if this is a destruction operation
			// look over all non-prova outputs and sum them up.
			for i := 0; i < len(adminOutputs); i++ {
				// if this output pk script is a NullDataTy, then,
				// according to previous validation, it must be
				// admin operation (destruction)
				scriptType := txscript.TypeOfScript(adminOutputs[i])
				if scriptType == txscript.NullDataTy {
					view.totalSupply -= uint64(tx.MsgTx().TxOut[i+1].Value)
				}
			}
		} else {
			// if it is an issuance operation, look over all but first
			// output and sum up values.
			// remember that a issuing transaction is not allow to also
			// destroy, as to previous validation.
			for i := 1; i < len(tx.MsgTx().TxOut); i++ {
				view.totalSupply += uint64(tx.MsgTx().TxOut[i].Value)
			}
		}
		view.threadTips[rmgutil.IssueThread] = wire.NewOutPoint(tx.Hash(), 0)
		return
	}
	for i := 0; i < len(adminOutputs); i++ {
		isAddOp, keySetType, pubKey,
			keyID := txscript.ExtractAdminOpData(adminOutputs[i])
		view.applyAdminOp(isAddOp, keySetType, pubKey, keyID)
	}
	// this becomes the new tip of the admin thread
	threadId := rmgutil.ThreadID(threadInt)
	view.threadTips[threadId] = wire.NewOutPoint(tx.Hash(), 0)
}

// applyAdminOp takes a single admin opp and applies it to the view.
func (view *KeyViewpoint) applyAdminOp(isAddOp bool,
	keySetType btcec.KeySetType, pubKey *btcec.PublicKey, keyID btcec.KeyID) {
	if keySetType == btcec.ASPKeySet {
		if isAddOp {
			view.aspKeyIdMap[keyID] = pubKey
			view.lastKeyID = keyID
		} else {
			delete(view.aspKeyIdMap, keyID)
		}
	} else {
		if isAddOp {
			view.adminKeySets[keySetType] = view.adminKeySets[keySetType].Add(pubKey)
		} else {
			pos := view.adminKeySets[keySetType].Pos(pubKey)
			view.adminKeySets[keySetType] = view.adminKeySets[keySetType].Remove(pos)
		}
	}
}

// connectTransaction updates the view by processing all new admin operations in
// the passed transaction.
func (view *KeyViewpoint) connectTransaction(tx *rmgutil.Tx, blockHeight uint32) error {
	// Process the admin outputs that are part of this tx.
	view.ProcessAdminOuts(tx, blockHeight)
	return nil
}

// connectTransactions updates the view by processing all the admin operations
// in created by all of the transactions in the passed block.
func (view *KeyViewpoint) connectTransactions(block *rmgutil.Block) error {
	for _, tx := range block.Transactions() {
		err := view.connectTransaction(tx, block.Height())
		if err != nil {
			return err
		}
	}
	return nil
}

// disconnectTransactions updates the view by undoing all admin operations in
// all of the transactions contained in the passed block, and setting the best
// hash for the view to the block before the passed block.
func (view *KeyViewpoint) disconnectTransactions(block *rmgutil.Block) error {

	// Loop backwards through all transactions so operations are undone in
	// reverse order.
	transactions := block.Transactions()
	for txIdx := len(transactions) - 1; txIdx >= 0; txIdx-- {
		tx := transactions[txIdx]

		// If an admin transaction is disconnected, undo what it did to chain state.
		threadInt, adminOutputs := txscript.GetAdminDetails(tx)
		if threadInt >= int(rmgutil.RootThread) {
			threadId := rmgutil.ThreadID(threadInt)
			if threadId == rmgutil.IssueThread {
				isDestruction := len(tx.MsgTx().TxIn) > 1
				if isDestruction {
					for i := 0; i < len(adminOutputs); i++ {
						// if this output pk script is a NullDataTy, then,
						// according to previous validation, it must be
						// admin operation (destruction)
						scriptType := txscript.TypeOfScript(adminOutputs[i])
						if scriptType == txscript.NullDataTy {
							view.totalSupply += uint64(tx.MsgTx().TxOut[i+1].Value)
						}
					}
				} else {
					for i := 1; i < len(tx.MsgTx().TxOut); i++ {
						view.totalSupply -= uint64(tx.MsgTx().TxOut[i].Value)
					}
				}
			} else {
				for i := 0; i < len(adminOutputs); i++ {
					isAddOp, keySetType, pubKey,
						keyID := txscript.ExtractAdminOpData(adminOutputs[i])
					isAddOp = !isAddOp
					view.applyAdminOp(isAddOp, keySetType, pubKey, keyID)
					// decrease lastKeyID counter, is an Add op is disconnected.
					if keySetType == btcec.ASPKeySet && isAddOp {
						view.lastKeyID = keyID - 1
					}
				}
			}
			// when an admin thread transaction is disconnected
			// we set the spent tx as new tip.
			view.threadTips[threadId] = &tx.MsgTx().TxIn[0].PreviousOutPoint
		}
	}

	return nil
}

// NewKeyViewpoint returns a new empty key view.
func NewKeyViewpoint() *KeyViewpoint {
	return &KeyViewpoint{
		threadTips:   make(map[rmgutil.ThreadID]*wire.OutPoint),
		lastKeyID:    btcec.KeyID(0),
		totalSupply:  uint64(0),
		adminKeySets: make(map[btcec.KeySetType]btcec.PublicKeySet),
		aspKeyIdMap:  make(map[btcec.KeyID]*btcec.PublicKey),
	}
}
