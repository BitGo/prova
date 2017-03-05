// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2016 The Zcash developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/bitgo/rmgd/chaincfg/chainhash"
	"math/big"
	"time"
)

var (
	// bigOne is 1 represented as a big.Int.  It is defined here to avoid
	// the overhead of creating it multiple times.
	bigOne = big.NewInt(1)

	// oneLsh256 is 1 shifted left 256 bits.  It is defined here to avoid
	// the overhead of creating it multiple times.
	oneLsh256 = new(big.Int).Lsh(bigOne, 256)
)

// HashToBig converts a chainhash.Hash into a big.Int that can be used to
// perform math comparisons.
func HashToBig(hash *chainhash.Hash) *big.Int {
	// A Hash is in little-endian, but the big package wants the bytes in
	// big-endian, so reverse them.
	buf := *hash
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}

	return new(big.Int).SetBytes(buf[:])
}

// CompactToBig converts a compact representation of a whole number N to an
// unsigned 32-bit number.  The representation is similar to IEEE754 floating
// point numbers.
//
// Like IEEE754 floating point, there are three basic components: the sign,
// the exponent, and the mantissa.  They are broken out as follows:
//
//	* the most significant 8 bits represent the unsigned base 256 exponent
// 	* bit 23 (the 24th bit) represents the sign bit
//	* the least significant 23 bits represent the mantissa
//
//	-------------------------------------------------
//	|   Exponent     |    Sign    |    Mantissa     |
//	-------------------------------------------------
//	| 8 bits [31-24] | 1 bit [23] | 23 bits [22-00] |
//	-------------------------------------------------
//
// The formula to calculate N is:
// 	N = (-1^sign) * mantissa * 256^(exponent-3)
//
// This compact form is only used in bitcoin to encode unsigned 256-bit numbers
// which represent difficulty targets, thus there really is not a need for a
// sign bit, but it is implemented here to stay consistent with bitcoind.
func CompactToBig(compact uint32) *big.Int {
	// Extract the mantissa, sign bit, and exponent.
	mantissa := compact & 0x007fffff
	isNegative := compact&0x00800000 != 0
	exponent := uint(compact >> 24)

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes to represent the full 256-bit number.  So,
	// treat the exponent as the number of bytes and shift the mantissa
	// right or left accordingly.  This is equivalent to:
	// N = mantissa * 256^(exponent-3)
	var bn *big.Int
	if exponent <= 3 {
		mantissa >>= 8 * (3 - exponent)
		bn = big.NewInt(int64(mantissa))
	} else {
		bn = big.NewInt(int64(mantissa))
		bn.Lsh(bn, 8*(exponent-3))
	}

	// Make it negative if the sign bit is set.
	if isNegative {
		bn = bn.Neg(bn)
	}

	return bn
}

// BigToCompact converts a whole number N to a compact representation using
// an unsigned 32-bit number.  The compact representation only provides 23 bits
// of precision, so values larger than (2^23 - 1) only encode the most
// significant digits of the number.  See CompactToBig for details.
func BigToCompact(n *big.Int) uint32 {
	// No need to do any work if it's zero.
	if n.Sign() == 0 {
		return 0
	}

	// Since the base for the exponent is 256, the exponent can be treated
	// as the number of bytes.  So, shift the number right or left
	// accordingly.  This is equivalent to:
	// mantissa = mantissa / 256^(exponent-3)
	var mantissa uint32
	exponent := uint(len(n.Bytes()))
	if exponent <= 3 {
		mantissa = uint32(n.Bits()[0])
		mantissa <<= 8 * (3 - exponent)
	} else {
		// Use a copy to avoid modifying the caller's original number.
		tn := new(big.Int).Set(n)
		mantissa = uint32(tn.Rsh(tn, 8*(exponent-3)).Bits()[0])
	}

	// When the mantissa already has the sign bit set, the number is too
	// large to fit into the available 23-bits, so divide the number by 256
	// and increment the exponent accordingly.
	if mantissa&0x00800000 != 0 {
		mantissa >>= 8
		exponent++
	}

	// Pack the exponent, sign bit, and mantissa into an unsigned 32-bit
	// int and return it.
	compact := uint32(exponent<<24) | mantissa
	if n.Sign() < 0 {
		compact |= 0x00800000
	}
	return compact
}

// CalcWork calculates a work value from difficulty bits.  Bitcoin increases
// the difficulty for generating a block by decreasing the value which the
// generated hash must be less than.  This difficulty target is stored in each
// block header using a compact representation as described in the documentation
// for CompactToBig.  The main chain is selected by choosing the chain that has
// the most proof of work (highest difficulty).  Since a lower target difficulty
// value equates to higher actual difficulty, the work value which will be
// accumulated must be the inverse of the difficulty.  Also, in order to avoid
// potential division by zero and really small floating point numbers, the
// result adds 1 to the denominator and multiplies the numerator by 2^256.
func CalcWork(bits uint32) *big.Int {
	// Return a work value of zero if the passed difficulty bits represent
	// a negative number. Note this should not happen in practice with valid
	// blocks, but an invalid block could trigger it.
	difficultyNum := CompactToBig(bits)
	if difficultyNum.Sign() <= 0 {
		return big.NewInt(0)
	}

	// (1 << 256) / (difficultyNum + 1)
	denominator := new(big.Int).Add(difficultyNum, bigOne)
	return new(big.Int).Div(oneLsh256, denominator)
}

// calcEasiestDifficulty calculates the easiest possible difficulty that a block
// can have given starting difficulty bits and a duration.  It is mainly used to
// verify that claimed proof of work by a block is sane as compared to a
// known good checkpoint.
// TODO(Prova): re-add with adjusted logic for the new difficulty calulation.
func (b *BlockChain) calcEasiestDifficulty(bits uint32, duration time.Duration) uint32 {
	return b.chainParams.PowLimitBits
}

// findPrevTestNetDifficulty returns the difficulty of the previous block which
// did not have the special testnet minimum difficulty rule applied.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) findPrevTestNetDifficulty(startNode *blockNode) (uint32, error) {
	// Search backwards through the chain for the last block without
	// the special rule applied.
	iterNode := startNode
	for iterNode != nil && int32(iterNode.height)%b.blocksPerRetarget != 0 &&
		iterNode.bits == b.chainParams.PowLimitBits {

		// Get the previous block node.  This function is used over
		// simply accessing iterNode.parent directly as it will
		// dynamically create previous block nodes as needed.  This
		// helps allow only the pieces of the chain that are needed
		// to remain in memory.
		var err error
		iterNode, err = b.getPrevNodeFromNode(iterNode)
		if err != nil {
			log.Errorf("getPrevNodeFromNode: %v", err)
			return 0, err
		}
	}

	// Return the found difficulty or the minimum difficulty if no
	// appropriate block was found.
	lastBits := b.chainParams.PowLimitBits
	if iterNode != nil {
		lastBits = iterNode.bits
	}
	return lastBits, nil
}

// calcNextRequiredDifficulty calculates the required difficulty for the block
// after the passed previous block node based on the difficulty retarget rules.
// This function differs from the exported CalcNextRequiredDifficulty in that
// the exported version uses the current best chain as the previous block node
// while this function accepts any block node.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) calcNextRequiredDifficulty(lastNode *blockNode) (uint32, error) {
	// Genesis block.
	if lastNode == nil {
		return b.chainParams.PowLimitBits, nil
	}

	// Find the first node in the averaging interval, sum the total bits
	// to use when averaging the difficulty over the interval.
	firstNode := lastNode
	avgDifficulty := big.NewInt(0)
	var err error
	for i := 0; firstNode != nil && i < b.chainParams.PowAveragingWindow; i++ {
		avgDifficulty.Add(avgDifficulty, CompactToBig(firstNode.bits))

		firstNode, err = b.getPrevNodeFromNode(firstNode)

		if err != nil {
			return 0, err
		}
	}

	// Exit early when there are not enough nodes to fill the window.
	if firstNode == nil {
		return b.chainParams.PowLimitBits, nil
	}

	avgDifficulty.Div(avgDifficulty, big.NewInt(int64(b.chainParams.PowAveragingWindow)))

	var medianFirstNodeTime time.Time

	medianFirstNodeTime, err = b.calcPastMedianTime(firstNode)

	if err != nil {
		return 0, err
	}

	var medianLastNodeTime time.Time

	medianLastNodeTime, err = b.calcPastMedianTime(lastNode)

	if err != nil {
		return 0, err
	}

	return b.nextRequiredDifficulty(medianFirstNodeTime, medianLastNodeTime, avgDifficulty), nil
}

// nextRequiredDifficulty calculates the required difficulty for the block
// after the passed previous block node based on a moving difficulty window.
func (b *BlockChain) nextRequiredDifficulty(firstNodeTime time.Time, lastNodeTime time.Time, avgDifficulty *big.Int) uint32 {
	// Limit adjustment step
	// Make sure to use medians to prevent time-warp attacks
	timespan := time.Duration(lastNodeTime.UnixNano() - firstNodeTime.UnixNano())

	// Limit the amount of adjustment that can occur to the previous
	// difficulty.
	timespan = b.chainParams.AveragingWindowTimespan() +
		(timespan-b.chainParams.AveragingWindowTimespan())/4
	if timespan < b.chainParams.MinActualTimespan() {
		timespan = b.chainParams.MinActualTimespan()
	} else if timespan > b.chainParams.MaxActualTimespan() {
		timespan = b.chainParams.MaxActualTimespan()
	}

	// Calculate new target difficulty as:
	//  averageDifficulty / averagingWindowTimespan * timespan
	// The result uses integer division which means it will be slightly
	// rounded down.
	avgWindowTimespan := big.NewInt(int64(b.chainParams.AveragingWindowTimespan() / time.Millisecond))
	avgDifficulty.Div(avgDifficulty, avgWindowTimespan)
	avgDifficulty.Mul(avgDifficulty, big.NewInt(int64(timespan/time.Millisecond)))

	// Limit new value to the proof of work limit.
	if avgDifficulty.Cmp(b.chainParams.PowLimit) > 0 {
		avgDifficulty.Set(b.chainParams.PowLimit)
	}

	return BigToCompact(avgDifficulty)
}

// CalcNextRequiredDifficulty calculates the required difficulty for the block
// after the end of the current best chain based on the difficulty retarget
// rules.
//
// This function is safe for concurrent access.
func (b *BlockChain) CalcNextRequiredDifficulty() (uint32, error) {
	b.chainLock.Lock()
	difficulty, err := b.calcNextRequiredDifficulty(b.bestNode)
	b.chainLock.Unlock()
	return difficulty, err
}
