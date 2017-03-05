# Prova Admin Transaction Design

This document describes Prova extensions to update the administrative state layer of the Prova chain.

## Design Goals

Prova is a Bitcoin-derivative consensus system that includes a blockchain mechanism to provide for updates to a synchronized ledger.  The synchronized state of the Bitcoin blockchain consists of its UTXO set and other states such as hashing difficulty and block subsidy.

For a permissioned block chain like the Prova chain, the set of mutable chain states Bitcoin provides for is not sufficient: Bitcoin is a permissionless system and lacks any mutable permission states beyond fund ownership.

In Prova the issuance of tokens, permissioning of block publishing, and other administrative functions need various special operations and provisioning.  To that end, the following special high level administrative keys are defined:

- **Root:** Add and revoke provision and issue keys.
- **Provision:** Add and revoke validate and ASP keys.
- **Issue:** Issue and de-issue tokens.

In addition, the Prova chain defines the following operational keys for permissioned operation:

- **Validate:**	Publish signed updates to the ledger in the form of blocks in the Prova block chain.
- **Account Service Provider (ASP):** Move funds with user participation, recovering lost funds, reassigning fund ownership without user participation.

This document defines the provisioning of these keys and their related discrete administrative operations in special transactions published to the Prova block chain. The design includes the following parts:

- The different sets of permissioned keys
- Opcodes for admin operations
- A design for ordering admin transactions to modify chain state in a deterministic order
- Special validation rules for permissioned transactions
- A definition of the initial parameters that establish the chain’s root authority and initial block validate keys

## Keys

To execute a permissioned administrative operation against the chain state, a pair of unique admin keys must have co-signed a special blockchain transaction authorizing that operation.  Each administrative key type is empowered with specific permissions defining the nature of administrative transactions it may sign.

### Root Keys

The purpose of root keys is to add and revoke provision keys and issue keys. Specifically root keys sign transactions that include the following operations:

- **Issue Key:** Add
- **Issue Key:** Revoke
- **Provision Key:** Add
- **Provision Key:** Revoke

Operations signed by root keys are the most critical to security and operability of the chain.  It is expected that the private keys used to sign root-level operations will be held in cold storage and used infrequently.

### Issue Keys

Issue keys mutate the outstanding amount of spendable tokens in the chain through the following operations:

- **Tokens:** Issue
- **Tokens:** De-issue

When introducing new tokens to the chain, issue keys are essentially unbounded in the amounts they can introduce.  When destroying tokens, appropriate spending signatures from the standard owner of the tokens is required in addition to the issue key signatures that authenticate the de-issue operation.

### Provision Keys

A provision key is used to authorize and revoke ASP keys and validate keys. This services the design goal of limiting the ultimate control of all chain funds and the overall ledger state to permissioned keys.  Provision keys are used when signing the following operations:

- **ASP Key:** Add
- **ASP Key:** Revoke
- **Validate Key:** Add
- **Validate Key:** Revoke

**NB:** Ownership of the provisioning keys must be strictly controlled. A malicious actor using a plurality of keys would be allowed to authorize enough validate keys to reorganize the block chain to an arbitrary depth, limited only by hard-coded block checkpoints.

### Key IDs

To enable shorter bare multisig style addresses, and as a defensive mechanism for potential use in future hard forks, Prova uses 4-byte key ids for ASP keys. These short IDs map back into a record on the chain of ASP public keys.

When an admin transaction adds an ASP public key to the list of authorized ASP keys, the transaction includes a 4-byte key id. The key id specified in the provisioning of a ASP key must be determined by incrementing the previous highest key id and incrementing it by one, or in the absence of a previous key id, it must be one.

Key IDs are only relevant for ASP Keys, not validate Keys.

## Admin Transactions

### Threads

To prevent reordering of administrative operations, operations occur in an unbroken chain, which we call a **thread**, of transactions, starting from the genesis block, each consuming the output of the last and creating a single new output which will be consumed by the next administrative transaction on that thread.

There will initially be 3 separate threads of administrative transactions:

- **Root thread:** Add and revoke provision and issue keys.
- **Provision thread:** Add and remove ASP and validate keys.
- **Issue thread:** Issue and de-issue tokens.

In each thread, only transactions whose thread input is signed by two keys out of the set of valid keys for that thread may spend the unspent output at the tip of the thread.  Only relevant actions may be taken: a root chain cannot contain issuing operations.  Keys may sign in any order.

To solve bootstrapping problems, an initial set of root, ASP and validate keys are hard-coded in the consensus parameters of the chain.  Differing keys should be selected for mainnet and the testing chains.

Admin transactions in the chain may not be played out of their original order, because each transaction has a backwards looking pointer towards the last action: the spending of the previous admin thread’s output.

The rationale for including a replay protection mechanism is to prevent any possible signal manipulation of admin transactions by manipulating the order of the transaction in the block chain.  Block generators can still delay updating of admin state changes, but as long as admin transactions are tied together via a chain of hashes the generators cannot reorder or replay them.

### Structure

A transaction extending a thread contains:

- One input at the first index spending the unspent output (tip) of the last thread transaction.
- One output creating the new tip of the thread.
- The keys and signatures authorizing the action.

The admin thread output must always be at the first index, except in the genesis block.

In the case of key updating operations, one or more outputs must be present containing operations to modify the state.  These are represented as special provably-unspendable outputs.

### Key Operations

To modify the admin state, admin transactions include outputs with unspendable `OP_RETURN` outputs of serialized state change statements.  The possible statements include:

```
ISSUE_KEY_ADD <issue pub key>
ISSUE_KEY_REVOKE <issue pub key>
PROVISION_KEY_ADD <provision pub key>
PROVISION_KEY_REVOKE <provision pub key>
VALIDATE_KEY_ADD <validate pub key>
VALIDATE_KEY_REVOKE <validate pub key>
ASP_KEY_ADD <asp pub key> <key id>
ASP_KEY_REVOKE <asp pub key> <key id>
```

When encoded into a transaction, the operations and their keys will be represented as:

```
<operation (1 byte)> <compressed public key (33-bytes)> <key id (only for ASPs): 4-bytes>
```

### Example Transaction

As an example example, authorizing a new provisioning key would result in a transaction that looks like:

```
Input 0: <previous thread transaction id/output>
ScriptSig 0: <root-pubkey-0> <root-sig-0> <root-pubkey-1> <root-sig-1>
Output 0: <ROOT_THREAD_ID (1 byte)> OP_CHECKTHREAD
Output 1: OP_RETURN <serialized data: PROVISION_KEY_ADD <provision pub key>>
```

The first output serves as the hook for the next transaction in the thread.  The first pushed value of the output defines which thread we’re on:

- **Thread 0:** Root thread
- **Thread 1:**	Provision thread
- **Thread 2:**	Issue thread

The opcode `OP_CHECKTHREAD` then enforces that this output can only be spent by keys out of the current set of keys (root keys, in this example).

The second output contains the admin operation: add the given public key to the set of authorized provisioning keys.  It is preceded by `OP_RETURN`, marking it as an unspendable output followed by some application specific, non-utxo impacting data.

### Issuance

To issue new tokens, transactions on the issue thread include an output to a standard address with the issued value.  When adding funds, issue transactions have the special case that their output values are permitted to be greater than their input values.

Example:

```
Input 0: <previous issue thread id/output>
ScriptSig 0: <issue-key-0> <issue-sig-0> <issue-key-1> <issue-sig-1>
Output 0: <ISSUE_THREAD> OP_CHECK_THREAD
Output 1: <standard token transfer output>
```

When de-issuing tokens, a standard key authorizes the spending value from a standard unspent output, then spends to an output that marks the tokens as permanently unspendable:

Example:

```
Input 0: <previous issue thread id/output>
ScriptSig 0: <issue pub key 0><issue sig 0><issue pub key 1><issue sig 1>
Input 1: <previous standard unspent transaction output>
ScriptSig 1: <standard spending signatures>
Output 0: <ISSUE THREAD> OP_CHECKTHREAD
Output 1: OP_RETURN
```

Fund removals must consume all funds of the unspent transaction output they are spending, no issue outputs or fees are valid.  The scriptSig authorizing the spend of a standard UTXO must be valid.

## Indexing

When a node evaluates the chain, it keeps a working set of admin keys to evaluate the validity of transactions against.  Updates to the admin keys take
effect on the block immediately after their provisioning transaction is included.

An interface is exposed on the set of these transactions and starting values that summarizes them into a final state: lists of public keys, one for each role / active status.

## Validation Rules

- An addition of a specially provisioned key may only occur once.  It is invalid to add an already provisioned key multiple times.
- Admin transactions in the genesis block do not need to provide a meaningful input or script-sig, they are automatically valid.
- Admin operations performed in the transaction must match the thread and be signed with two appropriate and matching keys: issue operations must be performed on the issue thread, etc.
- Aside from the exceptions for issuance, no non-admin outputs and inputs are allowed in an admin transaction.
- Issue transactions may not also de-issue tokens.
- The Admin thread output and input must always exist at the zero index, except for the outputs in the genesis block.
- Admin key provisioning transactions must be present in the block chain for 1 block before they go into effect.
- Admin transactions must have an associated admin operation, empty transactions are not valid.
- Aside from issuance outputs, all admin transactions must have 0 value inputs and 0 value outputs.

## Memory Pool Ordering

Any admin action should always takes precedence in the mempool.  This helps prevent a situation where the network is busy, potentially due to a DOS from a compromised co-signer key and the confirmation of the admin action takes longer than expected.
