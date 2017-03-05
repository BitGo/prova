# Overview

Prova is a decentralized digital asset platform.  It enables an issuer to
create, destroy, trade, and store digital tokens on a decentralized blockchain.

Like other blockchain technology, Prova provides transparency and cryptographic
proof of transactions via a decentralized and highly robust decentralized
ledger.

Prova differentiates from other blockchains in that:

1. Assets are transparently issued on chain via administrative transactions.
Tokens are not created as part of block creation.
1. Accounts are protected by M-of-N multi-signature for protection against theft
& loss.
2. Accounts are comprised of keys where a majority of keys are held by key
holders approved by the asset issuer.
3. Block creation is conducted by Validators that are approved by the asset
issuer.

Due to the fact that Prova does not issue tokens programmatically as part of
the block creation process (like Bitcoin, Ethereum, Litecoin, ...), tokens
are instead created by the Asset Issuer.  The Asset Issuer has permission
to create and destroy assets via administrative transactions which are 100%
visible on the blockchain by all participants.

As a model and a technical basis for Prova, the long-running distributed 
[Bitcoin](https://www.bitcoin.org) blockchain system is used as a basis.
The initial implementation of prova is based on the 
[btcd](https://github.com/btcsuite/btcd) implementation.

# Terminology

Prova uses the following terms as part of its documentation:

**Asset Issuer**
The Asset Issuer is responsible for asset issuance and 
destruction, as well as enabling validators and account service providers.

**Validator**
Validators verify correctness of transactions submitted
into the system and creates blocks in the blockchain.  Validators may
optionally collect fees and are similar to *miners* in other blockchains.

**Token**
A Token is an abstract notion of a digitized asset which can be traded
on the Prova blockchain.

**Admin Transaction**
Admin Transactions, or Administrative Transactions, are special purpose 
transactions used for
governance of the system.


# Token Movement

Prova tokens are exchanged in **Prova transactions**, which contain output 
scripts defining a locking condition required for spending expressed as a smart 
contract script.

In Prova transactions, tokens may *only* be sent to an 
[M-of-N](https://en.wikipedia.org/wiki/Multisignature) key locked destination, 
where the majority of keys are specifically authorized. The minority remainder 
keys are standard public key hashes.

Even though the majority of the required signing keys are held by authorized 
Account Service Providers, it should be considered that the holders of the
standard public key hashes are the intended owners of the tokens and in normal
cases they will instigate token transfers.

# Account Service Providers

Authorized holders of tokens are provisioned with an **Account Service Provider 
(ASP) key**.

There are two intended use-cases for ASP keys:

1. Account transaction service providers who co-sign transactions based on 
security policies.
2. Account recovery service providers who can assist in key-loss situations.

Other than being provisioned, the account service provider keys act much like 
standard transaction signing keys.

As there are a limited number of ASP keys and they are intended for the most 
standard use, an integer called a **key id** is assigned to each key to use as 
a shorthand for the longer public key value.

# Block Validation

The movement of Prova tokens is timestamped using a 
[proof-of-work](http://hashcash.org/) blockchain, as it is in the underlying 
model.  Every Prova node may sync the blockchain to independently verify the 
ownership of tokens, and that is the standard mechanism for use.

Prova alters the proof-of-work system to prevent denial-of-service or 
double-spending attacks by introducing the concept of provisioned block 
generators with consensus limits.  Blocks must be signed with these provsioned 
**validate keys** to be considered valid.

# Chain Share Limits

The blockchain design is suceptible to denial-of-service or double-spending 
attacks should an attack control a large ratio of solved blocks.  In the normal 
case the honest block generator majority will produce an eventually consistent 
version of the ledger, but given a sizeable enough minority or even a majority 
loss, these attacks become technically feasible.

To further safeguard against these issues, even provisioned validate keys are 
subject to consensus defined **share limits** that limit their allowed 
effective proof-of-work, or share of the blockchain.

# Asset Issuance

Asset tokens in Prova are not issued via coinbase rewards, instead they are 
allocated via **issue transactions**.

Issue transactions may only be signed by **issue keys** which are used to 
introduce and remove tokens from the system supply.

# Token Removal

Issue transactions are also used to remove tokens from the supply, however in 
this case the issue transaction must also include separate standard signatures 
authorizing the movement of the tokens being destroyed.

This prevents issue key holders from arbitrarily re-writing the token values, 
although they may add more tokens arbitrarily, taking value requires the 
cooperation of the normal token key holders.

# Admin Provisioning

Admin key provisioning, except for the starting set of genesis keys, occurs in 
special **admin transactions** that are included in the blockchain.

All admin transactions are signed with any two keys from the set of applicable 
keys.  This follows the overall design pattern of requiring multiple signatures 
to encourage independent and redundant verification of immutable actions.

## Thread Operations

To prevent admin transactions from being reordered, admin transactions must 
spend an output from the previous admin transaction.  These special outputs are 
referred to as **admin threads**, key provisioning and token issue operations 
may only occur as extensions of a thread with an origin point in the genesis 
block.

