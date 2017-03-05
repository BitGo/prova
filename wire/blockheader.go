// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/hex"
	"io"
	"time"
	// "log"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
)

// BlockValidatingPubKeySize is the number of bytes for a compressed pubkey.
const BlockValidatingPubKeySize = 33

// BlockValidatingPubKey defines the block validating public key.
// TODO(prova): replace this with a struct
type BlockValidatingPubKey [BlockValidatingPubKeySize]byte

// String returns a hexadecimal string of the public key
func (p BlockValidatingPubKey) String() string {
	return hex.EncodeToString(p[:])
}

// BlockSignatureSize is the number of bytes for a signature
const BlockSignatureSize = 80

// BlockSignature defines the block validating signature.
// TODO(prova): replace this with a struct
type BlockSignature [BlockSignatureSize]byte

// String returns a hexadecimal string of the signature
func (s BlockSignature) String() string {
	return hex.EncodeToString(s[:])
}

// BlockVersion is the current latest supported block version.
// TODO(prova): change this
const BlockVersion = 4

// MaxBlockHeaderPayload is the maximum number of bytes a block header can be.
const MaxBlockHeaderPayload = 32 + (chainhash.HashSize * 2) + BlockValidatingPubKeySize + BlockSignatureSize

// BlockHeader defines information about a block and is used in the bitcoin
// block (MsgBlock) and headers (MsgHeaders) messages.
type BlockHeader struct {
	// Version of the block.  This is not the same as the protocol version.
	Version int32

	// Hash of the previous block in the block chain.
	PrevBlock chainhash.Hash

	// Merkle tree reference to hash of all transactions for the block.
	MerkleRoot chainhash.Hash

	// Time the block was created. Encoded as int64 on the wire.
	Timestamp time.Time

	// Difficulty target for the block.
	Bits uint32

	// Height is the block height in the block chain.
	Height uint32

	// Size is the size of the serialized block in its entirety.
	Size uint32

	// Nonce used to generate the block (64 bits, to avoid extraNonce)
	Nonce uint64

	// Public key of the validating key used to sign the block
	ValidatingPubKey BlockValidatingPubKey

	// Signature of (PrevBlock|Merkle root) by block validating key
	Signature BlockSignature
}

// blockHeaderLen is a constant that represents the number of bytes for a block
// header.
const blockHeaderLen = MaxBlockHeaderPayload

// BlockHash computes the block identifier hash for the given block header.
func (h *BlockHeader) BlockHash() chainhash.Hash {
	// Encode the header and double sha256 everything prior to the number of
	// transactions.  Ignore the error returns since there is no way the
	// encode could fail except being out of memory which would cause a
	// run-time panic.
	var buf bytes.Buffer
	_ = writeBlockHeader(&buf, 0, h)

	return chainhash.PowHashH(buf.Bytes())
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
// See Deserialize for decoding block headers stored to disk, such as in a
// database, as opposed to decoding block headers from the wire.
func (h *BlockHeader) BtcDecode(r io.Reader, pver uint32) error {
	return readBlockHeader(r, pver, h)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
// See Serialize for encoding block headers to be stored to disk, such as in a
// database, as opposed to encoding block headers for the wire.
func (h *BlockHeader) BtcEncode(w io.Writer, pver uint32) error {
	return writeBlockHeader(w, pver, h)
}

// Deserialize decodes a block header from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Version field.
func (h *BlockHeader) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of readBlockHeader.
	return readBlockHeader(r, 0, h)
}

// Serialize encodes a block header from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Version field.
func (h *BlockHeader) Serialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of writeBlockHeader.
	return writeBlockHeader(w, 0, h)
}

// hashForSigning gets the double SHA256 hash of (Version|Timestamp|PrevBlock|MerkleRoot)
// which is used for the validator's signature.
func (h *BlockHeader) hashForSigning() []byte {
	buf := bytes.NewBuffer(make([]byte, 0, 12+2*chainhash.HashSize))
	err := writeElements(buf, h.Version, h.Timestamp.Unix(), &h.PrevBlock, &h.MerkleRoot)
	if err != nil {
		return nil
	}
	return chainhash.PowHashB(buf.Bytes())
}

// Sign uses the supplied private key to sign the signing-hash of the block
// header, and sets it in the Signature field.
func (h *BlockHeader) Sign(key *btcec.PrivateKey) error {
	hash := h.hashForSigning()
	signature, err := key.Sign(hash)
	if err != nil {
		return err
	}
	serialized := signature.Serialize()
	// TODO(prova): Remove commented code.
	// log.Printf("SIGNED hash=%v sig=%v prevblock=%v merkle=%v ",
	// 	hex.EncodeToString(hash),
	// 	hex.EncodeToString(serialized),
	// 	hex.EncodeToString(h.PrevBlock[:]),
	// 	hex.EncodeToString(h.MerkleRoot[:]),
	// )

	// Mark the public key used to sign the block.
	pubKey := key.PubKey().SerializeCompressed()[:BlockValidatingPubKeySize]
	copy(h.ValidatingPubKey[:BlockValidatingPubKeySize], pubKey[:BlockValidatingPubKeySize])

	copy(h.Signature[:], serialized)
	return nil
}

// Verify checks the signature on the block using the supplied public key.
func (h *BlockHeader) Verify(pubKey *btcec.PublicKey) bool {
	sig, err := btcec.ParseDERSignature(h.Signature[:], btcec.S256())
	if err != nil {
		return false
	}
	hash := h.hashForSigning()
	ret := sig.Verify(hash, pubKey)
	// log.Printf("VERIFY result=%v, hash=%v sig=%v prevblock=%v merkle=%v, ",
	// 	ret,
	// 	hex.EncodeToString(hash),
	// 	hex.EncodeToString(sig.Serialize()),
	// 	hex.EncodeToString(h.PrevBlock[:]),
	// 	hex.EncodeToString(h.MerkleRoot[:]),
	// )
	return ret
}

// NewBlockHeader returns a new BlockHeader using the provided previous block
// hash, merkle root hash, difficulty bits, and nonce used to generate the
// block with defaults for the remaining fields.
func NewBlockHeader(prevHash *chainhash.Hash, merkleRootHash *chainhash.Hash,
	bits uint32, nonce uint64) *BlockHeader {

	// Limit the timestamp to one second precision since the protocol
	// doesn't support better.
	return &BlockHeader{
		Version:    BlockVersion,
		PrevBlock:  *prevHash,
		MerkleRoot: *merkleRootHash,
		Timestamp:  time.Unix(time.Now().Unix(), 0),
		Bits:       bits,
		Nonce:      nonce,
	}
}

// readBlockHeader reads a bitcoin block header from r.  See Deserialize for
// decoding block headers stored to disk, such as in a database, as opposed to
// decoding from the wire.
func readBlockHeader(r io.Reader, pver uint32, bh *BlockHeader) error {
	err := readElements(r, &bh.Version, &bh.PrevBlock, &bh.MerkleRoot,
		(*int64Time)(&bh.Timestamp), &bh.Bits, &bh.Height, &bh.Size, &bh.Nonce, &bh.ValidatingPubKey, &bh.Signature)
	if err != nil {
		return err
	}

	return nil
}

// writeBlockHeader writes a bitcoin block header to w.  See Serialize for
// encoding block headers to be stored to disk, such as in a database, as
// opposed to encoding for the wire.
func writeBlockHeader(w io.Writer, pver uint32, bh *BlockHeader) error {
	err := writeElements(w, bh.Version, &bh.PrevBlock, &bh.MerkleRoot,
		bh.Timestamp.Unix(), bh.Bits, bh.Height, bh.Size, bh.Nonce, bh.ValidatingPubKey, bh.Signature)
	if err != nil {
		return err
	}

	return nil
}
