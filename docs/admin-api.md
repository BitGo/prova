# RMGd Admin RPC API

## Goals

The Admin RPC API is intended to be a rough simulation of how admin transactions should be crafted and propagated to the network.

Admin Tasks:

1. Create, collaboratively sign, and publish admin role key set transactions
2. Create, collaboratively sign, and publish issuance transactions
3. Create, collaboratively sign, and publish de-issuance transactions

## Change Admin Keys

### Create Raw Unsigned Admin Transaction

**Method**: *createrawadmintransaction*

Parameters:

1. keyType (string, required)
2. hexEncodedPublicKey (string, required)
3. active (boolean, required) - true to enable the key, false to disable it
4. unspentAdminTxId (string, required)

Returns: A hex encoded raw admin transaction for key provisioning.

Valid key types:

1. "issuing"
2. "provisioning"
3. "validate"
4. "asp"

**Add ASP Key Example**:

```
createrawadmintransaction
[
  "asp",
  "$aspPublicKey",   // Public key provided by the ASP
  1,                 // Make active
  "$lastUnspentTxId"
]

Returns
"$rawTransactionHexEncoded"
```

### Sign Raw Admin Transaction

**Method**: *signrawadmintransaction*

Parameters:

1. hexEncodedRawTransaction (string, required)
2. adminHexEncodedPrivateKey (string, required)

Returns: A hex encoded raw admin transaction that is signed.

**Sign Raw Admin Transaction Example:**

```
signrawadmintransaction
[
  "$rawTransactionFromCreateRawTransaction",
  "$adminPrivateKey"
]
```

## Issue Funds

### Create Raw Issuance Transaction

**Method**: *createrawissuancetransaction*

Parameters:

1. outputAddress (string, required)
2. amountToIssue (number, required)
3. unspentIssuanceTransactionId (string, required)

Returns: A hex encoded raw unsigned issuance transaction

**Issue Funds Example:**

```
createrawissuancetransaction
[
  "$addressToReceiveNewFunds"
  3.50
  "$lastIssuanceTxId"
]

Returns
"$unsignedRawIssuanceTransaction"
```

### Sign Raw Issuance Transaction

**Method**: *signrawissuancetransaction*

Parameters:

1. hexEncodedRawIssuanceTransaction (string, required)
2. privateKey (string, required)

Returns: A hex encoded raw signed issuance transaction

**Sign Issuance Example**

```
signrawissuancetransaction
[
  "$hexEncodedRawTransaction"
  "$hexEncodedPrivateKey"
]

Returns:
"signedIssuanceTransaction"
```

## Remove Funds

### Create Raw Deissuance Transaction

**Method**: *createrawdeissuancetransaction*

Parameters:

1. unspentOutputTxId (string, required)
2. unspentOutputVout (numeric, required)
3. valueToRemove (numeric, required)
3. unspentIssuanceTransactionId (string, required)

Returns: A hex encoded raw unsigned de-issuance transaction

**Create Deissuance Tx Example:**

```
createrawdeissuancetransaction
[
  "$unspentTxIdWithFundsToDeissue",
  "$unspentTxVoutNumber",
  3.50, // Unspent funds related to the output
  "$lastIssuanceTxId"
]
```

### Sign Raw Deissuance Transaction

**Method**: *signrawdeissuancetransaction*

Parameters:

1. hexEncodedRawDeissuanceTransaction (string, required)
2. privateKey (string, required) // This can be a ASP key or an admin key

Returns: A hex encoded raw signed issuance transaction

**Sign Issuance Transaction Example:**

```
signrawdeissuancetransaction
[
  "$rawIssuanceTransaction"
  "$private Key"
]
```
