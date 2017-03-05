# Prova Install

## Install Go

1. Follow installation instructions here: https://golang.org/doc/install
2. Confirm go works with the "hello world" example in the install

## Check Go Version

Check that the version is correct and the GOROOT and the GOPATH are defined and not the same path. Make you have setup the go directory with bin/pkg/src sub directories for the source and binary builds.

```bash
$ go version
$ go env GOROOT GOPATH
```

GOPATH should be in your user directory so that there will be no issue writing there. An example would be `~/dev/go`

You should add a line to your PATH environment variable to include the GOPATH binary builds path.

```
export PATH=$PATH:$GOPATH/bin
```

## Clone Prova

Make sure you have an SSH key registered with GitHub, then create in your `GOPATH` src/ directory the path `github.com/bitgo` and navigate to this path. There may be errors along this path, see below for how to resolve.

```bash
$ go get -u github.com/Masterminds/glide
$ git clone git@github.com:BitGo/rmgd.git
$ glide install
$ go install
```

When executing `go install` to install Prova, make sure that you are in the GOPATH directory (github.com/bitgo/rmgd) otherwise it will give you an error instructing you to install from within your GOPATH directory.

After you install you should be able to do `go test`, `go install` and this will successfully test the top level of rmgd and build it into the go/bin. Because the bin path is in your PATH you should be able to do `rmgd -h` successfully to confirm the app has been built and is in your path.

## Run Prova

To run Prova you will need a data directory to store the chain state. Create this directory where you like, like for testnet in your home directory you can create ~/testnet3/ - in this directory make a directory like `/1/` if you want to work with multiple chain states.

For Prova in regtest simulating 2 nodes, create directories `~/regtest/1` and `~/regtest/2` and the following test.sh bash script can run two nodes against each other in regtest.

```
#!/bin/bash 
LOCAL=127.0.0.1
nohup rmgd --listen=$LOCAL:6001 --rpcuser=user --rpcpass=pass --rpclisten=$LOCAL:7001 --datadir=$HOME/regtest/\
1/ --connect=$LOCAL:6002 --regtest --txindex > $HOME/regtest/1/btcd.log 2>&1 &
nohup rmgd --listen=$LOCAL:6002 --rpcuser=user --rpcpass=pass --rpclisten=$LOCAL:7002 --datadir=$HOME/regtest/\
2/ --connect=$LOCAL:6001 --regtest > $HOME/regtest/2/btcd.log 2>&1 &
sleep 1
btcctl -u user -P pass -s 127.0.0.1:7001 generate 101
```

For Prova in testnet mode, create a ~/testnet3/1 directory, use the following command line arguments to start the testnet node and connect to the remote node.

If you are on the internal dev VPN you can connect to a testnet node running master at 10.151.51.76:18333

```
rmgd
--addpeer=dev-rmgd-01:17979
--addrindex
--datadir=$YOUR_PATH_TO_DATA_DIRECTORY
--listen=127.0.0.1:6001
--notls
--rpclisten=127.0.0.1:7001
--rpcpass=pass
--rpcuser=user
--testnet
--txindex
```

These arguments will enable the tx and addr indexes, specify testnet and the data directory, and allow commands to the localhost RPC with user/pass on port 7001. Mainnet is not currently supported, so make sure to toggle testnet.

## Run Tests

If you want to confirm that everything is working in your build, you can create a commandline alias to run all the package tests.

In your home directory (or wherever you would like), create a new file `rmgpackages`

Edit the file to include the contents:

```
github.com/bitgo/rmgd/addrmgr
github.com/bitgo/rmgd/blockchain
github.com/bitgo/rmgd/btcec
github.com/bitgo/rmgd/btcjson
github.com/bitgo/rmgd/chaincfg
github.com/bitgo/rmgd/database
github.com/bitgo/rmgd/mempool
github.com/bitgo/rmgd/peer
github.com/bitgo/rmgd/rmgutil
github.com/bitgo/rmgd/txscript
github.com/bitgo/rmgd/wire
github.com/bitgo/rmgd
```

Now edit your bash profile to include the alias to test, replacing `~/rmgpackages` with the path to where you placed the file.

```
alias rmgtest='cat ~/rmgpackages | grep -v \# | xargs go test'
```

Running this alias will run each package's tests. You can also use `go test -v` if you wish to see verbose results.
