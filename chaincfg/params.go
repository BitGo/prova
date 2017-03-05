// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2016 The Zcash developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"encoding/hex"
	"errors"
	"github.com/bitgo/rmgd/btcec"
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"github.com/bitgo/rmgd/wire"
	"math/big"
	"time"
)

// These variables are the chain proof-of-work limit parameters for each default
// network.
var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// mainPowLimit is the highest proof of work value a block can
	// have for the main network.  It is the value 2^243 - 1.
	mainPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 243), bigOne)

	// regressionPowLimit is the highest proof of work value a block
	// can have for the regression test network.
	regressionPowLimit = powLimitFromStr("0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f0f")

	// testNet3PowLimit is the highest proof of work value a block can have
	// for the test network (version 3).
	testNet3PowLimit = powLimitFromStr("07ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff")

	// simNetPowLimit is the highest proof of work value a Bitcoin block
	// can have for the simulation test network.  It is the value 2^255 - 1.
	simNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)
)

// Checkpoint identifies a known good point in the block chain.  Using
// checkpoints allows a few optimizations for old blocks during initial download
// and also prevents forks from old blocks.
//
// Each checkpoint is selected based upon several factors.  See the
// documentation for blockchain.IsCheckpointCandidate for details on the
// selection criteria.
type Checkpoint struct {
	Height uint32
	Hash   *chainhash.Hash
}

// Params defines a Bitcoin network by its parameters.  These parameters may be
// used by Bitcoin applications to differentiate networks as well as addresses
// and keys for one network from those intended for use on another network.
type Params struct {
	// Name defines a human-readable identifier for the network.
	Name string

	// Net defines the magic bytes used to identify the network.
	Net wire.BitcoinNet

	// DefaultPort defines the default peer-to-peer port for the network.
	DefaultPort string

	// DNSSeeds defines a list of DNS seeds for the network that are used
	// as one method to discover peers.
	DNSSeeds []string

	// GenesisBlock defines the first block of the chain.
	GenesisBlock *wire.MsgBlock

	// GenesisHash is the starting block hash.
	GenesisHash *chainhash.Hash

	// AdminKeySets is the set of keys governing the chain state.
	AdminKeySets map[btcec.KeySetType]btcec.PublicKeySet

	// ASPKeyIdMap are the provisioned keyIDs and respective pubKeys
	ASPKeyIdMap btcec.KeyIdMap

	// PowLimit defines the highest allowed proof of work value for a block
	// as a uint256.
	PowLimit *big.Int

	// PowLimitBits defines the highest allowed proof of work value for a
	// block in compact form.
	PowLimitBits uint32

	// CoinbaseMaturity is the number of blocks required before newly mined
	// coins (coinbase transactions) can be spent.
	CoinbaseMaturity uint16

	// SubsidyReductionInterval is the interval of blocks before the subsidy
	// is reduced.
	SubsidyReductionInterval uint32

	// TargetTimespan is the desired amount of time that should elapse
	// before the block difficulty requirement is examined to determine how
	// it should be changed in order to maintain the desired block
	// generation rate.
	TargetTimespan time.Duration

	// TargetTimePerBlock is the desired amount of time to generate each
	// block.
	TargetTimePerBlock time.Duration

	// ReduceMinDifficulty defines whether the network should reduce the
	// minimum required difficulty after a long enough period of time has
	// passed without finding a block.  This is really only useful for test
	// networks and should not be set on a main network.
	ReduceMinDifficulty bool

	// GenerateSupported specifies whether or not CPU mining is allowed.
	GenerateSupported bool

	// Checkpoints ordered from oldest to newest.
	Checkpoints []Checkpoint

	// Enforce current block version once network has
	// upgraded.  This is part of BIP0034.
	BlockEnforceNumRequired uint64

	// Reject previous block versions once network has
	// upgraded.  This is part of BIP0034.
	BlockRejectNumRequired uint64

	// The number of nodes to check.  This is part of BIP0034.
	BlockUpgradeNumToCheck uint64

	// Mempool parameters
	RelayNonStdTxs bool

	// Address encoding magics
	PubKeyHashAddrID byte // First byte of a P2PKH address
	ScriptHashAddrID byte // First byte of a P2SH address
	ProvaAddrID      byte // First byte of an Prova address
	PrivateKeyID     byte // First byte of a WIF private key

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID [4]byte
	HDPublicKeyID  [4]byte

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType uint32

	// Number of blocks for the moving window of difficulty adjustment.
	PowAveragingWindow int

	// Maximum downward adjustment in pow difficulty, as a percentage.
	PowMaxAdjustDown int64

	// Maximum upward adjustment in pow difficulty, as a percentage.
	PowMaxAdjustUp int64

	// Number of consecutive trailing blocks allowed
	ChainTrailingSigKeyIdLimit int

	// Percentage limit of blocks from a single sig key id allowed
	ChainWindowShareLimit int
}

// MaxActualTimespan returns a timespan with the down-dampening factor applied.
func (p Params) MaxActualTimespan() time.Duration {
	dampenPercentage := time.Duration(100 + p.PowMaxAdjustDown)
	return (p.AveragingWindowTimespan() * dampenPercentage) / 100
}

// MinActualTimespan returns a timespan with the up dampening factor applied.
func (p Params) MinActualTimespan() time.Duration {
	dampenPercentage := time.Duration(100 - p.PowMaxAdjustUp)
	return (p.AveragingWindowTimespan() * dampenPercentage) / 100
}

// AveragingWindowTimespan returns the difficulty timespan to be averaged over.
func (p Params) AveragingWindowTimespan() time.Duration {
	return time.Duration(p.PowAveragingWindow) * p.TargetTimePerBlock
}

// MainNetParams defines the network parameters for the main Bitcoin network.
var MainNetParams = Params{
	Name:        "mainnet",
	Net:         wire.MainNet,
	DefaultPort: "7979",
	DNSSeeds:    []string{},

	// Chain parameters
	GenesisBlock:             &genesisBlock,
	GenesisHash:              &genesisHash,
	PowLimit:                 mainPowLimit,
	PowLimitBits:             0x1f07ffff,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute,         // 1 minute
	ReduceMinDifficulty:      false,
	GenerateSupported:        false,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{},

	// Enforce current block version once majority of the network has
	// upgraded.
	// 75% (750 / 1000)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 95% (950 / 1000)
	BlockEnforceNumRequired: 750,
	BlockRejectNumRequired:  950,
	BlockUpgradeNumToCheck:  1000,

	// Mempool parameters
	RelayNonStdTxs: false,

	// Address encoding magics
	PubKeyHashAddrID: 0x00, // starts with 1
	ScriptHashAddrID: 0x05, // starts with 3
	PrivateKeyID:     0x80, // starts with 5 (uncompressed) or K (compressed)
	ProvaAddrID:      0x33, // starts with G

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x88, 0xad, 0xe4}, // starts with xprv
	HDPublicKeyID:  [4]byte{0x04, 0x88, 0xb2, 0x1e}, // starts with xpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 0,

	// Number of blocks for the moving window of difficulty adjustment
	PowAveragingWindow: 17,

	// Maximum downward adjustment in pow difficulty, as a percentage
	PowMaxAdjustDown: 32,

	// Maximum upward adjustment in pow difficulty, as a percentage
	PowMaxAdjustUp: 16,

	// Number of consecutive trailing blocks allowed
	ChainTrailingSigKeyIdLimit: 2,

	// Percentage limit of blocks from a single sig key id allowed
	ChainWindowShareLimit: 25,
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

// RegressionNetParams defines the network parameters for the regression test
// Bitcoin network.  Not to be confused with the test Bitcoin network (version
// 3), this network is sometimes simply called "testnet".
var RegressionNetParams = Params{
	Name:        "regtest",
	Net:         wire.TestNet,
	DefaultPort: "18444",
	DNSSeeds:    []string{},

	// Chain parameters
	GenesisBlock: &regTestGenesisBlock,
	GenesisHash:  &regTestGenesisHash,
	AdminKeySets: func() map[btcec.KeySetType]btcec.PublicKeySet {
		keySets := make(map[btcec.KeySetType]btcec.PublicKeySet)

		//root keys
		keySets[btcec.RootKeySet], _ = btcec.ParsePubKeySet(btcec.S256(),
			"025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1", // priv eaf02ca348c524e6392655ba4d29603cd1a7347d9d65cfe93ce1ebffdca22694
			"038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202", // priv 2b8c52b77b327c755b9b375500d3f4b2da9b0a1ff65f6891d311fe94295bc26a
		)

		//validate keys
		keySets[btcec.ValidateKeySet], _ = btcec.ParsePubKeySet(btcec.S256(),
			"035f5103852bd7d9c9c28e44caf1f7188941e16295062ca4c89928a8ccff993cd3", // TODO(prova) add priv
			"0265de49399e78020026219492e2a6e1a41e93591b87220ae8a2f3ebf3473dbeef", // TODO(prova) add priv
			"039cb94c99c4700918250c40fa35b7fa0a75a967c9366aa19b8fc354373368beef", // TODO(prova) add priv
			"031337ab09070254638075c7b59643dce2d60c5260bf5841d2f8cc6f75f6790d4e", // TODO(prova) add priv
		)

		return keySets
	}(),
	ASPKeyIdMap: func() btcec.KeyIdMap {
		pubKey1, _ := btcec.ParsePubKey(hexToBytes("025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1"), btcec.S256())
		pubKey2, _ := btcec.ParsePubKey(hexToBytes("038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202"), btcec.S256())
		return map[btcec.KeyID]*btcec.PublicKey{btcec.KeyID(1): pubKey1, btcec.KeyID(2): pubKey2}
	}(),
	PowLimit:                 regressionPowLimit,
	PowLimitBits:             0x200f0f0f,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 150,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute,         // 1 minute
	ReduceMinDifficulty:      true,
	GenerateSupported:        true,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Enforce current block version once majority of the network has
	// upgraded.
	// 75% (750 / 1000)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 95% (950 / 1000)
	BlockEnforceNumRequired: 750,
	BlockRejectNumRequired:  950,
	BlockUpgradeNumToCheck:  1000,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	PubKeyHashAddrID: 0x6f, // starts with m or n
	ScriptHashAddrID: 0xc4, // starts with 2
	ProvaAddrID:      0x58, // starts with T
	PrivateKeyID:     0xef, // starts with 9 (uncompressed) or c (compressed)

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 1,

	// Number of blocks for the moving window of difficulty adjustment
	PowAveragingWindow: 17,

	// Maximum downward adjustment in pow difficulty, as a percentage
	PowMaxAdjustDown: 32,

	// Maximum upward adjustment in pow difficulty, as a percentage
	PowMaxAdjustUp: 16,
}

// TestNet3Params defines the network parameters for the test Bitcoin network
// (version 3).  Not to be confused with the regression test network, this
// network is sometimes simply called "testnet".
var TestNet3Params = Params{
	Name:        "testnet3",
	Net:         wire.TestNet3,
	DefaultPort: "17979",
	DNSSeeds:    []string{},

	// Chain parameters
	GenesisBlock: &testNet3GenesisBlock,
	GenesisHash:  &testNet3GenesisHash,
	AdminKeySets: func() map[btcec.KeySetType]btcec.PublicKeySet {
		keySets := make(map[btcec.KeySetType]btcec.PublicKeySet)

		//root keys
		keySets[btcec.RootKeySet], _ = btcec.ParsePubKeySet(btcec.S256(),
			"025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1", // priv eaf02ca348c524e6392655ba4d29603cd1a7347d9d65cfe93ce1ebffdca22694
			"038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202", // priv 2b8c52b77b327c755b9b375500d3f4b2da9b0a1ff65f6891d311fe94295bc26a
		)

		//validator keys
		keySets[btcec.ValidateKeySet], _ = btcec.ParsePubKeySet(btcec.S256(),
			"035f5103852bd7d9c9c28e44caf1f7188941e16295062ca4c89928a8ccff993cd3", // TODO(prova) add priv
			"0265de49399e78020026219492e2a6e1a41e93591b87220ae8a2f3ebf3473dbeef", // TODO(prova) add priv
			"039cb94c99c4700918250c40fa35b7fa0a75a967c9366aa19b8fc354373368beef", // TODO(prova) add priv
			"031337ab09070254638075c7b59643dce2d60c5260bf5841d2f8cc6f75f6790d4e", // TODO(prova) add priv
		)

		return keySets
	}(),
	ASPKeyIdMap: func() btcec.KeyIdMap {
		pubKey1, _ := btcec.ParsePubKey(hexToBytes("025ceeba2ab4a635df2c0301a3d773da06ac5a18a7c3e0d09a795d7e57d233edf1"), btcec.S256())
		pubKey2, _ := btcec.ParsePubKey(hexToBytes("038ef4a121bcaf1b1f175557a12896f8bc93b095e84817f90e9a901cd2113a8202"), btcec.S256())
		return map[btcec.KeyID]*btcec.PublicKey{btcec.KeyID(1): pubKey1, btcec.KeyID(2): pubKey2}
	}(),
	PowLimit:                 testNet3PowLimit,
	PowLimitBits:             0x2007ffff,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute,         // 1 minute
	ReduceMinDifficulty:      true,
	GenerateSupported:        false,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: []Checkpoint{},

	// Enforce current block version once majority of the network has
	// upgraded.
	// 51% (51 / 100)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 75% (75 / 100)
	BlockEnforceNumRequired: 51,
	BlockRejectNumRequired:  75,
	BlockUpgradeNumToCheck:  100,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	PubKeyHashAddrID: 0x6f, // starts with m or n
	ScriptHashAddrID: 0xc4, // starts with 2
	PrivateKeyID:     0xef, // starts with 9 (uncompressed) or c (compressed)
	ProvaAddrID:      0x58, // starts with T

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x35, 0x83, 0x94}, // starts with tprv
	HDPublicKeyID:  [4]byte{0x04, 0x35, 0x87, 0xcf}, // starts with tpub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 1,

	// Number of blocks for the moving window of difficulty adjustment
	PowAveragingWindow: 17,

	// Maximum downward adjustment in pow difficulty, as a percentage
	PowMaxAdjustDown: 64,

	// Maximum upward adjustment in pow difficulty, as a percentage
	PowMaxAdjustUp: 64,

	// Number of consecutive trailing blocks allowed
	ChainTrailingSigKeyIdLimit: 45,

	// Percentage limit of blocks from a single sig key id allowed
	ChainWindowShareLimit: 50,
}

// SimNetParams defines the network parameters for the simulation test Bitcoin
// network.  This network is similar to the normal test network except it is
// intended for private use within a group of individuals doing simulation
// testing.  The functionality is intended to differ in that the only nodes
// which are specifically specified are used to create the network rather than
// following normal discovery rules.  This is important as otherwise it would
// just turn into another public testnet.
var SimNetParams = Params{
	Name:        "simnet",
	Net:         wire.SimNet,
	DefaultPort: "18555",
	DNSSeeds:    []string{}, // NOTE: There must NOT be any seeds.

	// Chain parameters
	GenesisBlock:             &simNetGenesisBlock,
	GenesisHash:              &simNetGenesisHash,
	PowLimit:                 simNetPowLimit,
	PowLimitBits:             0x207fffff,
	CoinbaseMaturity:         100,
	SubsidyReductionInterval: 210000,
	TargetTimespan:           time.Hour * 24 * 14, // 14 days
	TargetTimePerBlock:       time.Minute,         // 1 minutes
	ReduceMinDifficulty:      true,
	GenerateSupported:        true,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Enforce current block version once majority of the network has
	// upgraded.
	// 51% (51 / 100)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 75% (75 / 100)
	BlockEnforceNumRequired: 51,
	BlockRejectNumRequired:  75,
	BlockUpgradeNumToCheck:  100,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	PubKeyHashAddrID: 0x3f, // starts with S
	ScriptHashAddrID: 0x7b, // starts with s
	PrivateKeyID:     0x64, // starts with 4 (uncompressed) or F (compressed)

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x00}, // starts with sprv
	HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3a}, // starts with spub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 115, // ASCII for s

	// Number of blocks for the moving window of difficulty adjustment
	PowAveragingWindow: 17,

	// Maximum downward adjustment in pow difficulty, as a percentage
	PowMaxAdjustDown: 32,

	// Maximum upward adjustment in pow difficulty, as a percentage
	PowMaxAdjustUp: 16,

	// Number of consecutive trailing blocks allowed
	ChainTrailingSigKeyIdLimit: 2,

	// Percentage limit of blocks from a single sig key id allowed
	ChainWindowShareLimit: 25,
}

var (
	// ErrDuplicateNet describes an error where the parameters for a Bitcoin
	// network could not be set due to the network already being a standard
	// network or previously-registered into this package.
	ErrDuplicateNet = errors.New("duplicate Bitcoin network")

	// ErrUnknownHDKeyID describes an error where the provided id which
	// is intended to identify the network for a hierarchical deterministic
	// private extended key is not registered.
	ErrUnknownHDKeyID = errors.New("unknown hd private extended key bytes")
)

var (
	registeredNets    = make(map[wire.BitcoinNet]struct{})
	pubKeyHashAddrIDs = make(map[byte]struct{})
	scriptHashAddrIDs = make(map[byte]struct{})
	provaAddrIDs      = make(map[byte]struct{})
	hdPrivToPubKeyIDs = make(map[[4]byte][]byte)
)

// Register registers the network parameters for a Bitcoin network.  This may
// error with ErrDuplicateNet if the network is already registered (either
// due to a previous Register call, or the network being one of the default
// networks).
//
// Network parameters should be registered into this package by a main package
// as early as possible.  Then, library packages may lookup networks or network
// parameters based on inputs and work regardless of the network being standard
// or not.
func Register(params *Params) error {
	if _, ok := registeredNets[params.Net]; ok {
		return ErrDuplicateNet
	}
	registeredNets[params.Net] = struct{}{}
	pubKeyHashAddrIDs[params.PubKeyHashAddrID] = struct{}{}
	scriptHashAddrIDs[params.ScriptHashAddrID] = struct{}{}
	if params.ProvaAddrID != 0 {
		provaAddrIDs[params.ProvaAddrID] = struct{}{}
	}
	hdPrivToPubKeyIDs[params.HDPrivateKeyID] = params.HDPublicKeyID[:]
	return nil
}

// mustRegister performs the same function as Register except it panics if there
// is an error.  This should only be called from package init functions.
func mustRegister(params *Params) {
	if err := Register(params); err != nil {
		panic("failed to register network: " + err.Error())
	}
}

// IsPubKeyHashAddrID returns whether the id is an identifier known to prefix a
// pay-to-pubkey-hash address on any default or registered network.  This is
// used when decoding an address string into a specific address type.  It is up
// to the caller to check both this and IsScriptHashAddrID and decide whether an
// address is a pubkey hash address, script hash address, neither, or
// undeterminable (if both return true).
func IsPubKeyHashAddrID(id byte) bool {
	_, ok := pubKeyHashAddrIDs[id]
	return ok
}

// IsScriptHashAddrID returns whether the id is an identifier known to prefix a
// pay-to-script-hash address on any default or registered network.  This is
// used when decoding an address string into a specific address type.  It is up
// to the caller to check both this and IsPubKeyHashAddrID and decide whether an
// address is a pubkey hash address, script hash address, neither, or
// undeterminable (if both return true).
func IsScriptHashAddrID(id byte) bool {
	_, ok := scriptHashAddrIDs[id]
	return ok
}

// IsProvaAddrID returns whether the id is an identifier known to prefix a
// standard Prova address on any default or registered network.  This is
// used when decoding an address string into a specific address type.
func IsProvaAddrID(id byte) bool {
	_, ok := provaAddrIDs[id]
	return ok
}

// HDPrivateKeyToPublicKeyID accepts a private hierarchical deterministic
// extended key id and returns the associated public key id.  When the provided
// id is not registered, the ErrUnknownHDKeyID error will be returned.
func HDPrivateKeyToPublicKeyID(id []byte) ([]byte, error) {
	if len(id) != 4 {
		return nil, ErrUnknownHDKeyID
	}

	var key [4]byte
	copy(key[:], id)
	pubBytes, ok := hdPrivToPubKeyIDs[key]
	if !ok {
		return nil, ErrUnknownHDKeyID
	}

	return pubBytes, nil
}

// newHashFromStr converts the passed big-endian hex string into a
// chainhash.Hash.  It only differs from the one available in chainhash in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		// Ordinarily I don't like panics in library code since it
		// can take applications down without them having a chance to
		// recover which is extremely annoying, however an exception is
		// being made in this case because the only way this can panic
		// is if there is an error in the hard-coded hashes.  Thus it
		// will only ever potentially panic on init and therefore is
		// 100% predictable.
		panic(err)
	}
	return hash
}

// powLimitFromStr returns a pow limit based on a difficulty hex value.
func powLimitFromStr(hexStr string) *big.Int {
	limit := big.NewInt(0)
	limit.SetString(hexStr, 16)
	return limit
}

func init() {
	// Register all default networks when the package is initialized.
	mustRegister(&MainNetParams)
	mustRegister(&TestNet3Params)
	mustRegister(&RegressionNetParams)
	mustRegister(&SimNetParams)
}
