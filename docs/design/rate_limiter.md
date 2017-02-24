# RMG Validator Share Limiter

With RMG we need a way to guarantee against a situation in which a certain validator ever has a larger than acceptable share of hash power. We want a diverse set of miners so that no one validator is ever in a position where they can possibly exert unwanted control over the system or capture all of the transaction fees.

## Requirements

Each block will be signed with a key that has a chain of signing authority that leads back to the genesis block.

1. The network should be participatory with regard to validator. The ongoing chain should always be composed of a diverse set of independent contributors.
2. Should a validator's key ever be used in an aggressive way towards the network, the validator's key may be revoked through administrative action. This administrative transaction should not be able to be blocked by the validator himself.
3. We want to avoid a hardware arms race among validators.

## Algorithm Options

## Deterministic Block Numbers

Each Validator Key that is allowed to generate blocks can only make blocks at certain deterministic block numbers. Block allowances cover distributed intervals, like every n blocks.

Assuming a block time of ten minutes, and block generators Alice and Bob, Alice can only mine even blocks and Bob can only create odd blocks.

Pros:

- Deterministically and simply guarantees that neither Alice nor Bob can ever have a 51% majority of the network to censor remediating administration transactions.
- Should be fairly straightforward to implement and audit the function that maps a key to an interval.

Cons:

- Potentially brittle: a sufficient degree of non-participating members is enough to stop the chain completely, particularly problematic should there be few block generators.

## Consensus Block Limit

All blocks are inspected to see the number and run length of the block generators. These values are compared against maximum consensus constants to determine chain validity. Miners with excessive numbers of blocks in an adjustment period should stop hashing until the next period.

Pros:

- Simple constants allow for fine grained control of block distribution: what percentage of blocks can a single miner claim, how many blocks in a row can they claim? These values are exposed as pure constants.
- Allows for easy dynamic redistribution of hash power and minimizes "stuck chain" issues relating to a validator not producing blocks. Any subset of validators up to a certain limit could disappear forever and there would be little impact.

Cons:

- Block generation needs to be able to inspect the chain state to determine whether hashing should be done
- Could result in irregular timing of some blocks: should a miner have a very outsized hash power, they will "fast follow" a normal block with their full complement of blocks, meaning the normal gap between blocks will be slower than expected.

## Moving Micro-Time Windows

Instead of deterministic blocks, miners could be given deterministic time windows in which to mine, based on the block timestamp value. Given block generation between Alice and Bob, Alice could attempt to mine for the first five minutes, then hand it off to Bob to mine for the last five minutes, based on a deterministic assignment from their random key value.

Pros:

- A time round-robin may closely model the desired behaviour of every miner getting a turn at bat.
- It should be simple to fairly evaluate in a stateless manner whether hashing should proceed and whether a block is valid

Cons:

- Not simple regarding fairness or resistance to time manipulation
- Could cause wasted hashing or consensus issues near the time boundaries

## Individualized Difficulty

Each validator key or deterministic grouping of validator keys could have an individualized difficulty rating. Every validator then has an equal chance at a block regardless of their hash power.

Pros:

- Egalitarian solution that provides rewards for all miners from large to small with an identical participation reward.

Cons:

- This creates an incentive problem: more hash power does not map directly to more rewards.
- Breaks the concept of "most work chain" as the correct chain
- Requires storing and synchronizing individualized difficulties

## Unwanted Behaviour

1. Small percentage of non representative miners are super efficient: what if the first portion of the difficulty period has super fast blocks and the final portion has super slow blocks?
2. What if there are not enough miners mining to complete a mining cycle?

## Mining Proof of Work Impact

Because miners will be limited in the number of blocks they are allowed to mine in a given period, the proof of work becomes more of a redundancy safety check and an assignment operator.

Because there is no free market for the proof of work in this chain, the signal quality of the proof of work should be expected to be fairly low. We'd expect chain participants to not be strongly incentivized to optimize their proof of work methods, meaning that should someone apply optimizations to the proof of work, they would make proof of work values from non optimizing participants look small by comparison.

Instead of orienting difficulty around a novel Hashcash proof of energy algorithm which should have a high noise factor due to the development of possible optimizations and hardware, Bitcoin's existing portable Hashcash could be used as a replacement.

This would work by having every generator provably burn or pay Bitcoin to a known output, in order to proxy as the non-reversible cost of an internal hash power function. Nodes would then verify that each block represented a certain well known cost to the generator and resolve the longest chain by the chain with the most Bitcoin spent. This would just serve as an independent safety check: an attacker bypassing somehow the other security features and reorganizing the chain would have to pay a cumulative cost in Bitcoin to do so.

## Recommendation

The simplest and most effective algorithm would be a straightforward addition of two consensus rules: one for total blocks and another for block runs.

The total blocks limit determines the permissible share of a mining difficulty period. A good value for this would be something like 25%: that would prevent someone from owning the end of one period and the beginning of another period to try and get a reorganizing majority of blocks.

The block run limit determines the permissible share of consecutive blocks, to prevent a short lucky run or someone with outsized hash power distorting the expected spread of blocks.

Still, this algorithm is vulnerable to an optimizing miner. Proof of work is supposed to represent proof of energy, meaning an expenditure of real world value. Optimization in hardware and software threaten to make that proof minimally meaningful in a non-market environment. However, in the near to medium term, we don't expect problems here, as the set of validators will be hand-selected, and since there is little direct financial incentive to dramatically increase hash power (given no block rewards and minimal transaction fees.)

For a longer term algorithm, adopting a multi-stage round-robin block generation assignment protocol with aggregation of validation votes would be more efficient and meaningful. A middle step that would more closely approximate the simple guarantees of the Bitcoin Blockchain would be to use proof-of-burn through an integration with the Bitcoin Blockchain.
