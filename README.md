Prova
====

Prova is a distributed consensus system for digital asset tokens written in Go (golang).
Prova was built to support RMG, a digitized form of gold introduced by [The Royal Mint](http://www.royalmint.com/rmg)
and CME Group.  Features include:

 * Asset Issuance

   Prova is a simple, single asset blockchain with asset issuance rather than mining.

 * Security

   All accounts are multi-signature wallets.

   * No “black holes”

     The system enforces rules making it impossible to send to invalid or non-recoverable addresses

   * Permissioned Participants

     Prova can enforce only authorized validators and accounts.

   * Trusted Technology

     Prova is based on mature blockchain technologies.

     The implementation is derived from btcd.

It downloads, validates, and serves a block chain using consensus rules for block acceptance.  It includes a full block validation testing framework.

It relays newly generated blocks, maintains a transaction memory pool, and
relays individual transactions that have not yet made it into a block.  It
ensures all individual transactions admitted to the pool follow the rules
required by the block chain.

Prova does *NOT* include wallet functionality.  This means you can't actually make or receive payments directly with Prova.  That functionality is provided by Prova wallet implementations.

## Requirements

[Go](http://golang.org) 1.6 or newer.

## Installation

#### Linux/BSD/MacOSX/POSIX - Build from Source

- Install Go according to the installation instructions here:
  http://golang.org/doc/install

- Ensure Go was installed properly and is a supported version:

```bash
$ go version
$ go env GOROOT GOPATH
```

NOTE: The `GOROOT` and `GOPATH` above must not be the same path.  It is
recommended that `GOPATH` is set to a directory in your home directory such as
`~/goprojects` to avoid write permission issues.  It is also recommended to add
`$GOPATH/bin` to your `PATH` at this point.

- Run the following commands to obtain prova, all dependencies, and install it:

```bash
$ go get -u github.com/Masterminds/glide
$ git clone https://github.com/bitgo/prova $GOPATH/src/github.com/bitgo/prova
$ cd $GOPATH/src/github.com/bitgo/prova
$ glide install
$ go install . ./cmd/...
```

- Prova (and utilities) will now be installed in ```$GOPATH/bin```.  If you did
  not already add the bin directory to your system path during Go installation,
  we recommend you do so now.

## Updating

#### Linux/BSD/MacOSX/POSIX - Build from Source

- Run the following commands to update Prova, all dependencies, and install it:

```bash
$ cd $GOPATH/src/github.com/bitgo/prova
$ git pull && glide install
$ go install . ./cmd/...
```

## Getting Started

Prova has several configuration options avilable to tweak how it runs, but all
of the basic operations described in the intro section work with zero
configuration.

#### Linux/BSD/POSIX/Source

```bash
$ ./prova
```

## Issue Tracker

The [integrated github issue tracker](https://github.com/bitgo/prova/issues)
is used for this project.

## Documentation

The documentation is a work-in-progress.  It is located in the [docs](https://github.com/bitgo/prova/tree/master/docs) folder.

## License

Prova is licensed under the [copyfree](http://copyfree.org) ISC License.
