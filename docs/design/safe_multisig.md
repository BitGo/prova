# Safe Multi-Sig

Standard Prova transactions use a new type of script & corresponding address format that provides chain-enforced safety against unauthorized fund movement, double-spends, fund loss, or un-authorized holding of funds.

These transactions are modeled after the original multi-sig design of Bitcoin transactions, specifically in a m-of-n signing configuration with a quorum of keys being explicitly-permissioned Account Service Provider (ASP) keys, referenced by 32-bit KeyIDs, along with a non-quorum set of public key hashes. Typically this means 2 KeyIDs and 1 key hash in a 2-of-3 configuration.

Account Service Providers are businesses or organizations explicitly vetted by the chain root admin key holder. The ASP keys are provisioned and assigned to KeyIDs on the chain itself using [administrative transactions](admin.md).

## Transactions

In Bitcoin transactions, payment is always done through payment to an *output scriptPub*, which is then later combined with a *scriptSig* signing script to evaluate a complete script that authorizes spending.

In the original multi-sig design of Bitcoin, an OP_CHECKMULTISIG output defined the total keys and required number of signers to authorize fund movements, allowing for multiple independent parties to collectively make fund movement decisions through cooperative signing.

With OP_CHECKMULTISIG, as with all standard original Bitcoin outputs, public keys were the norm for specifying transaction destinations. Over time, the desire for a more convenient way to direct payments towards destinations arose and the concept of a shortened address-friendly public-key-hash output was created. Standard Bitcoin transactions now use this pay-to-public-key-hash output formulation that substitutes a 20-byte public key ripe160 sha256 public key hash for the 33-byte compressed public key.

Prova transactions build on these ideas, substituting the public keys in OP_CHECKMULTISIG with shortened address-friendly identifiers of two types:

1. ASP KeyIDs: 4-byte, 32 bit unsigned int ids, saving 29 bytes in addresses vs. a pubkey
2. Public key hashes: 20-byte hashes, saving 13 bytes

The addition of the 2 4-byte KeyIDs makes addresses slightly longer than those used in Bitcoin, but still of reasonable size. When paying to a standard 2-of-3 safe multisig script, the output script consists of a listing of the required signatories and the safe multi-sig opcode. Two unique ASP keys are referenced by a *KeyID* number and one user key is referenced by a public key hash.

```
OP_2 <20 byte public key hash> <4 byte KeyID> <4 byte KeyID> OP_3 OP_CHECKSAFEMULTISIG
```

The following consensus rules apply to this formulation:

- The number of keys required to sign MUST be at least 2.
- It MUST NOT be possible for all keys defined by key hashes to form a quorum which authorizes a transaction (i.e. at least 1 signature from a ASP key is always required)
- A quorum MUST exist of registered ASP keys, allowing all the ASPs to jointly move funds if necessary

When signing a transaction which spends the 2-of-3 output above, a scriptSig of the following form must be presented to be combined with the OP_CHECKSAFEMULTISIG scriptPub for execution by the script VM:

```
<sig1> <pubKeyOrID1> <sig2> <pubKeyOrID2>
```

In the original Bitcoin `OP_CHECKMULTISIG` only the signatures are provided, since the public keys are present in the scriptPub, but we need the keys (or references to them by KeyID). The `OP_CHECKMULTISIG` operation also required a leading throw-away value in its signature, called by some as a "dummy" value, to compensate for a bug in the evaluation of `OP_CHECKMULTISIG`. This dummy value is eliminated in `OP_CHECKSAFEMULTISIG`.

The presented pubKeys must match either the public key hash or a ASP key id specified in the output for the spending scriptSig to be valid. The ordering of the signatures in the scriptSig must match the ordering from the scriptPub, as in Bitcoin.

Prova transactions are much stricter in the enforcement of what consists of a valid output. In Bitcoin, outputs may be made to any validly formed script, without regard to whether that script is spendable. This flexibility can lead to situations where a user accidentally sends funds permanently to a "black hole" from which they cannot be recovered. In the Prova blockchain, while the validators cannot know whether a particular key hash actually has a known public key as its pre-image, they are able to enforce that a quorum of professionally-held KeyIDs can control the funds. And indeed, this is enforced by consensus. This means it is impossible to lose funds by accidentally sending to a black hole. It also makes theft much more difficult and less lucrative, since funds can only move through addresses involving vetted and registered ASPs.

## Address Format

Standard Prova outputs in a 1 user key and 2 ASP key configuration are represented in a simple address format. Addresses are constructed using the standard base58 encoding format of the 3 identifying keys:

```
base58-encode(
  [one-byte version]
  [20-byte public key hash]
  [little endian 4-byte key id]
  [little endian 4-byte key id]
  [4-byte checksum]
)
```

- The 1 byte version number is 51 / 0x33 on the main network, resulting in addresses beginning with "G"
- The 1 byte version number is 88 / 0x58 on the test network, resulting in addresses beginning with "T"
- The 20-byte public hash is the ripe160 hash of the SHA256 hash of a standard public key.
- The subsequent two 4-byte sequences are the key ids for the ASP keys.
- The final 4-byte checksum is the first four bytes of the double SHA256 hash of the version, hash, and key ids.

Example addresses
```
Testnet: TCq7ZvyjTugZ3xDY8m1Mdgm95v4QmMpMcXWyipGfS8DME
Mainnet: GDLPrZnvGXwGcrAZgMWnfXbTnfnboo7k7ddggyBx5paJ6
```

## Privacy

Note that because of the inclusion of the KeyIDs, it is immediately evident from an address which ASPs are the responsible co-signers. This makes it trivial to contact the "provider" of an address if necessary. However, the inclusion of the raw key hash in addresses means that privacy is still afforded among individual customers of a ASP, since HD wallets can be constructed which produce new addresses for every transaction by rotating the user key. To determine which individual user controlled a given address or addresses, law enforcement would still need to serve a subpoena to the relevant ASP.
