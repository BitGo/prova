# RMG Admin Transaction Design

This document describes extensions to update the administrative state layer of the RMG chain.

## Design Goals

RMG is a Bitcoin-derivative consensus system that includes a blockchain mechanism to provide for updates to a synchronized ledger. The synchronized state of the Bitcoin blockchain consists of its UTXO set and other states such as hashing difficulty and block subsidy.

For a permissioned block chain like the RMG chain, the set of mutable chain states Bitcoin provides for is not sufficient: Bitcoin is a permissible system and lacks any mutable permission states beyond fund ownership. In RMG issuance of funds, permitting of block publishing, and other administrative functions need various special operations and provisioning. To that end, the following special high level administrative keys are defined:

- **Root:**   Approve and revoke provisioning and issuing keys.
- **Provisioning:**   Provision and revoke Validator and WSP keys.
- **Issuing:** 		Issue new funds and destroy existing funds.

In addition, the RMG chain defines the following operational keys for permissioned operation:

- **Validate:**	Publish updates to the ledger in the form of blocks in the RMG block chain.
- **Wallet Service Provider (WSP):** Move funds with user participation, recovering lost funds, reassigning fund ownership without user participation.

This document defines the provisioning of these keys and their related discrete administrative operations in special transactions published to the RMG block chain. The design includes the following parts:

- The different sets of permissioned keys
- Opcodes for admin operations
- A design for ordering admin transactions to modify chain state in a deterministic order
- Special validation rules for permissioned transactions
- A definition of the initial parameters that establish the chain’s root authority and initial block validate keys

## Keys

To execute a permissioned administrative operation against the chain state, a pair of unique admin keys must have co-signed a special blockchain transaction authorizing that operation. Each administrative key type is empowered with specific permissions defining the nature of administrative transactions it may sign.

### Root Keys

The purpose of Root Keys is to add and revoke Provisioning Keys and Issuing Keys. Specifically Root Keys sign transactions that include the following operations:

- **Issuing Key:** Add
- **Issuing Key:** Remove
- **Provisioning Key:** Add
- **Provisioning Key:** Remove

Operations signed by the Root Keys are the most critical to security and operability of the chain. It is expected that the private keys used to sign root-level operations will be held in cold storage and used infrequently.

### Issuing Keys

Issuing keys mutate the outstanding amount of spendable funds in the chain through the following operations:

1. **Funds:** Issue
2. **Funds:** Destroy

When introducing new funds to the chain, Issuing Keys are unbounded in the value they can introduce. However, when destroying funds, only funds already under control of the Issuer may be destroyed. The inputs destroyed in a destruction operation must be signed in the normal way.

### Provisioning Keys

A provisioning key may be used to authorize and revoke WSP keys and Validator keys. This services the design goal of limiting the ultimate control of all chain funds and the overall ledger state to permissioned keys. Provisioning keys are used when signing the following operations:

- **WSP Key:** Add
- **WSP Key:** Remove
- **Validate Key:** Add
- **Validate Key:** Remove

**NB:** Ownership of the provisioning keys must be strictly controlled. A malicious actor using a plurality of keys would be allowed to authorize enough validate keys to reorganize the block chain to an arbitrary depth, limited only by hard-coded block checkpoints.

### Key IDs

To enable shorter bare MultiSig style addresses, and as a defensive mechanism for potential use in future hard forks, RMG uses 4-byte key ids for WSP keys. These short IDs map back into a record on the chain of WSP public keys.

When a provisioning transaction adds a WSP public key to the list of authorized WSP keys, the transaction includes a 4-byte key id. The key id specified in the provisioning of a WSP key must be determined by incrementing the previous highest key id and incrementing it by one, or in the absence of a previous key id, it must be one.

Key IDs are only relevant for WSP Keys, not Validate Keys.

## Admin Transactions

### Threads

To prevent reordering of administrative operations, operations occur in an unbroken chain, which we call a **thread**, of transactions, starting from the genesis block, each consuming the output of the last and creating a single new output which will be consumed by the next administrative transaction on that thread.

There will initially be 3 separate threads of administrative transactions:

- **Root thread:** Add and remove provisioning and issuing keys.
- **Provisioning thread:** Add and remove WSP and validate keys.
- **Issuing thread:** Introduce and remove funds.

In each thread, only transactions whose thread input is signed by two keys out of the set of valid keys for that thread may spend the unspent output at the tip of the thread. Only relevant actions may be taken: a root chain cannot contain issuing operations.

To solve the bootstrapping problem, an initial set of root, provisioning and validate keys will be added to the consensus parameters for the chain, in the code itself. Differing keys should be selected for mainnet and the testing chains.

Admin transactions in the chain may not be played out of their original order, because each transaction has a backwards looking pointer towards the last action: the spending of the previous admin thread’s output.

The rationale for including a replay protection mechanism is to prevent any possible signal manipulation of admin transactions by manipulating the order of the transaction in the block chain. Block generators can still delay updating of admin state changes, but as long as admin transactions are tied together via a chain of hashes the generators cannot reorder or replay them.

### Structure

A thread transaction contains:

- Exactly 1 input spending the unspent output (tip) of the last thread transaction.
- Exactly 1 output creating the new tip of the thread.
- One or more outputs containing operations to modify the state, represented as special provably unspendable outputs.
- The keys and signatures authorizing the action.

### Key Operations

To modify the admin state, admin transactions include outputs with provably unspendable `OP_RETURN` outputs of serialized state change statements. The possible statements include:

```
ISSUING_KEY_ADD <issuing pub key>
ISSUING_KEY_REVOKE <issuing pub key>
PROVISIONING_KEY_ADD <provisioning pub key>
PROVISIONING_KEY_REVOKE <provisioning pub key>
VALIDATE_KEY_ADD <validate pub key>
VALIDATE_KEY_REVOKE <validate pub key>
WSP_KEY_ADD <wsp pub key> <key id>
WSP_KEY_REVOKE <wsp pub key> <key id>
```

When encoded into a transaction, the operations and their keys will be represented as:

```
<operation (1-byte)> <compressed public key (33-bytes)> <key id (only for WSPs): 4-bytes>
```

### Example Transaction

As an example example, authorizing a new provisioning key would result in a transaction that looks like:

```
Input 0: <previous thread id/output>
ScriptSig 0: <root-pubkey-0> <root-sig-0> <root-pubkey-1> <root-sig-1>
Output 0: <ROOT_THREAD> OP_CHECK_THREAD
Output 1: OP_RETURN <serialized data: PROVISIONING_KEY_ADD <provisioning pub key>>
```

The first output serves as the hook for the next transaction in the thread. The first pushed value of the output defines which thread we’re on:

- **Thread 0:** Root thread
- **Thread 1:**	Provisioning thread
- **Thread 2:**	Issuing thread

The opcode `OP_CHECK_THREAD` then enforces that this output can only be spent by keys out of the current set of keys (root keys, in this example).

The second output contains the admin operation: add the given public key to the set of authorized provisioning keys. It is preceded by `OP_RETURN`, marking it as an unspendable output followed by some application specific, non-UTXO impacting data.

### Issuance

To issue new tokens, transactions on the Issuing thread include an output to a standard Aztec 2-of-3 address with the issued value. (When adding funds, issuance transactions have the special case that their output values are permitted to be greater than their input values.)

Example:

```
Input 0: <previous issuance thread id/output>
ScriptSig 0: <issuing-key-0> <issuing-sig-0> <issuing-key-1> <issuing-sig-1>
Output 0: <ISSUING_THREAD> OP_CHECK_THREAD
Output 1: <standard aztec 2-of-3 output>
```

When destroying funds, a standard key authorizes the spending funds from a standard unspent output, then spends them to an output that marks them as unspendable:

Example:

```
Input 0: <previous issuance thread id/output>
ScriptSig 0: <issuing pub key 0><issuing sig 0><issuing pub key 1><issuer sig 1>
Input 1: <previous standard aztec 2-of-3 unspent transaction output>
ScriptSig 1: <standard aztec 2-of-3 spending signatures>
Output 0: <ISSUING THREAD> OP_CHECK_THREAD
Output 1: OP_ISSUER_REMOVE_FUNDS (with associated value to remove)
```

Fund removals must consume all funds of the unspent transaction output they are spending, no issuance outputs or fees are valid. The scriptSig authorizing the spend of a 2-of-3 UTOX must be valid.

## Indexing

When a node evaluates the chain, initially it is expected that the chain daemon would keep the entire set of admin update transactions in memory at all times, as well as the initial key set written into the codebase.

An interface can be exposed on the set of these transactions and starting values that summarizes them into a final state: lists of public keys, one for each role / active status.

In a future iteration or as an optimization, the consensus state can be condensed in the same way the UTXO state is, however overall the number of consensus actions should be expected to be small enough that they can easily be kept around in their entirety.

## Validation Rules

- Any addition of any specially provisioned key may only occur once, it is invalid to add any already provisioned key multiple times.
- Any admin transactions in the genesis block do not need to provide a meaningful input or script-sig, they are automatically valid.
- The admin operations performed in the transaction must match the thread and be signed with two appropriate and matching keys: issuance operations must be performed on the issuance thread, etc.
- Only one admin operation output is permitted in a transaction. Aside from the exceptions for issuance, no non-admin outputs and inputs are allowed in an admin transaction.
- The admin thread output and inputs must always exist at the zero index.
- All admin transactions must be present in the block chain for 100 blocks before they go into effect.
- Admin transactions must have an associated admin-operation, empty transactions are not valid.
- Aside from issuance outputs, all admin transactions must have 0 value inputs and 0 value outputs.

## Memory Pool Ordering

Any admin action always takes precedence in the memory pool. This helps prevent a situation where the network is busy, potentially due to a DOS from a compromised co-signer key and the confirmation of the admin action takes longer than expected.


