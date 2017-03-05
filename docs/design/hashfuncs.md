# Exploration of Potential Hash Functions

What hash function should be used in Prova? We want a mature algorithm that is novel enough to provide resistance to existing ASICs or new dedicated hardware for a reasonable time period, which is the reason SHA256d must be replaced.

Selection goals:

1. Easy to implement, library maturity.
2. Well vetted: algorithm maturity, especially in the context of proof of work.
3. Must be suitable for [Hashcash](http://www.hashcash.org/): fast to verify, non-progressive: hash partial pre-image resistance.

Other problems to avoid:

1. A DOS vector where slow-to-verify incorrect hashes bog down node resources.
2. Excessively novel schemes that would be time intensive and risky to implement.
3. Complexity: in most cases we want a system that just makes generators politely cycle between themselves, the hash function can be altered should an ASIC appear.

## Obvious Contenders

### [SHA-3(Keccak)](https://en.wikipedia.org/wiki/SHA-3) (Chosen)

Benefits:

- Probably will have the widest future usage, most libraries
- History of partial in-production use by [Ethereum](https://www.ethereum.org/)
- Already a patch written by a Core Developer to alter Bitcoin Core to SHA3(Keccak), should very easy to just include a new GoLang SHA3 library and change the hash function.
- Recent algorithm, not yet widely used in proof of work
- Various language implementations exist

Problems:

- CPU bound: asymmetry between the optimizing attacker and the standard node. Should become commonly used very widely throughout various industries, leading to a high likelihood of the development of ASICs over the long run.

### [Blake256](https://en.wikipedia.org/wiki/BLAKE_(hash_function))

Benefits:

- Already used in production for Decred, which is also based off of [btcd](https://github.com/btcsuite/btcd)
- Drop in replacement for sha256 here: https://github.com/decred/dcrd/blob/master/chaincfg/chainhash/hashfuncs.go
- Top contender for SHA3 algorithm
- Various language implementations exist

Problems:

- CPU bound: asymmetry between a potential optimizing attacker and the normal users.

### [Scrypt](http://www.tarsnap.com/scrypt.html)

Benefits:

- Lots of mature implementations, due to its use as a password key stretching algorithm and then usage in [Litecoin](https://litecoin.org/) derivatives.

Problems:

- If you make the memory target large, you also increase the verification difficulty, which is not desirable for a Hashcash system like Bitcoin.
- Its common usage lends to ASIC development
- Memory hardness may be brute force bypassed with sufficient CPU, the memory variable is simply the most generally efficient route to the solution of Scrypt, but given enough CPU a less efficient method can be taken to overpower more technically efficient competition.

## Interesting Projects

### [Cuckoo Cycle](http://www.hashcash.org/papers/cuckoo.pdf)

Proof of work system based on graph theory, designed to be memory-hard.

https://github.com/tromp/cuckoo/blob/master/doc/cuckoo.pdf

Benefits:

- Novel and unlikely to have any ASIC made for it

Problems:

- Large and unfamiliar serialized format
- Relatively recent and potentially immature vetting, paper here claims an order of magnitude efficiency gain over the proposed algorithm: https://www.cs.cmu.edu/~dga/crypto/cuckoo/analysis.pdf

### [Equihash](https://www.internetsociety.org/sites/default/files/blogs-media/equihash-asymmetric-proof-of-work-based-generalized-birthday-problem.pdf)

Proof of work designed around the birthday paradox / birthday problem that is designed to be memory intensive.

Benefits:

- C/C++ implementation and Go bindings already done: https://github.com/sammy007/go-equihash and https://github.com/khovratovich/equihash
- Simple memory hard hash, designed for crypto-currency usage
- Selected for use in [Zcash](https://z.cash/blog/why-equihash.html)

Problems:

- Limited use and vetting

### Hashimoto

Proof of work system based on using the ledger as a shared data source. Alternative derivations have used generated data sources to follow the same purposes. Originally proposed by Thaddeus Dryja.

http://diyhpl.us/~bryan/papers2/bitcoin/meh/hashimoto.pdf

Benefits:

- Memory requirement by using the ledger data as a shared random challenge source, similar to Sergio Lerner's 2013 proposal of present memory oracles: https://bitslog.files.wordpress.com/2013/12/memohash-v0-3.pdf
- Developed into the Ethereum hash function

Problems:

- Adds a lot of additional complexity

