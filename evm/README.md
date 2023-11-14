# World Engine EVM Base Shard (world-evm)

## Installation

### From Source

Clone the repo

`git clone https://github.com/Argus-Labs/world-engine.git`

Run the `install-rollup` makefile command

`make install-rollup`

Verify installation success

`world-evm version`

## Running a Test Sequencer Node w/ Docker Compose

World Engine provides simple scripts to start a testing sequencer node with a local celestia devnet for DA. 

Assuming you have the repository cloned and are in the root directory, run the following make command:

`make rollup`

The rollups exposes the default ports that comes with the Cosmos SDK. Head over to the Cosmos SDK documentation to learn more about which ports are exposed: https://docs.cosmos.network/v0.50/learn/advanced/grpc_rest

### From Prebuilt Docker Image

If you want to make your own setup script, but still want to use the world-evm binary, you can grab a docker image of the world-evm here:

Prebuilt Docker Image:
```bash
us-docker.pkg.dev/argus-labs/world-engine/evm:<latest/tag_version>
```

## Features

### Game Shard Transaction Sequencer

The rollup is extended via a special gRPC server that game shards can connect to for the purpose of submitting and storing transactions to the base shard.

This gRPC server runs, by default, at port `9601`, but can be configured by setting the `SHARD_SEQUENCER_PORT` environment variable.

### Router

The rollup provides an extension to its underlying EVM environment with a specialized precompile that allows messages to be forwarded from smart contracts to game shards that implement the router server.

In order for the router to communicate with game shards, their namespaces must be mapped to their gRPC address. These are stored through the x/namespace module, and can be updated via an authority address. The authority address is loaded at the start of the application from an environment variable named `NAMESPACE_AUTHORITY_ADDR`. If unset, the authority for the namespace module will be set to the governance module address, allowing for namespaces to be added via governance. 

When namespace authority is set, you can update the namespaces via the `register` command provided by world-evm. 

```bash
world-evm tx namespace register foobar foo.bar.com:9020
```


#### Using the Router in Solidity

In order to use the precompile, you first need to copy over the precompile contract code. The contract lives at:

`evm/precompile/contracts/src/cosmos/precompile/router.sol`

The precompile address will always be `0x356833c4666fFB6bFccbF8D600fa7282290dE073`.

Instantiating the precompile:

```solidity
// the path of import will change depending on where you copied the 
// precompile contract code to.
import {IRouter} from "./precompile/router.sol";

contract SomeGame {
    IRouter private immutable router;

    constructor () {
        router = IRouter(0x356833c4666fFB6bFccbF8D600fa7282290dE073);
    }
}

```

#### Sending Messages

First, a smart contract needs structs that are mirrors of the EVM enabled message structs found in the game shard.

For example:

Game Shard Foo message:
```go
type Foo struct {
	Bar int64
	Baz string
}

type FooResult struct {
	Success bool
}
```

Solidity Mirror:
```solidity
struct Foo {
  int64 Bar;
  string Baz;
}

struct FooResult {
  bool Success;
}
```

Then, we simply abi encode an instance of the struct, and pass it along to the router, along with the name of the message we are sending, and the namespace of the game shard instance we want to send the message to:

```solidity
Foo memory fooMsg = Foo(420, "we are so back")
bytes memory encodedFoomsg = abi.encode(fooMsg)
bool ok = router.sendMessage(encodedFooMsg, "foo", "game-shard-1");
```

#### Receiving Message Results

Receiving message results must be done in a separate transaction, due to World Engine's asynchronous architecture. In order to get the results of a message, we use another precompile method from the router: `messageResult`.


messageResult takes in the transaction hash of the original EVM transaction that triggered the cross-shard transaction. It returns the abi encoded message result, an error message, and an arbitrary status code. 

```solidity
 (bytes memory txResult, string memory errMsg, uint32 code) =  router.messageResult(txHash);
```

To decode the result, use `abi.decode`

```solidity
FooResult memory res = abi.decode(txResult, (FooResult));
```
The following codes may be returned:

Cardinal Codes:
1: CodeSuccess
2: CodeTxFailed
3: CodeNoResult
4: CodeServerUnresponsive
5: CodeUnauthorized
6: CodeUnsupportedMessage
7: CodeInvalidFormat

EVM Base Shard Codes:
100: CodeConnectionError
101: CodeServerError

### Querying Game Shards

Game shards can be queried using the same contructs as above, however, the precompile will return the results synchronously. 
```solidity
  QueryLocation memory q = QueryLocation(name); 
  bytes memory queryBz = abi.encode(q);
  bytes memory bz = router.query(queryBz, queryLocationName, Namespace);
  QueryLocationResponse memory res = abi.decode(bz, (QueryLocationResponse));
```

# Running the Sequencer

Below are the following environment variables needed to run the sequencer.

## Faucet

- FAUCET_ADDR
The application is capable of supplying a faucet address with funds. Setting the `FAUCET_ADDR` will keep an account topped up to be used in a faucet.

## x/namespace
- NAMESPACE_AUTHORITY_ADDR=<world engine address>
  - the address of the account you want to be able to update namespace mappings with.

### Secure gRPC Connections
For production environments, you'll want to setup secure connections between gRPC servers handling cross-shard communication. To make use of these, set the following environment variables to the path of your SSL certification files:
- SERVER_CERT_PATH=<path/to/server/cert>
- SERVER_KEY_PATH=<path/to/server/key>
- CLIENT_CERT_PATH=<path/to/client/cert>

### DA Layer
The following variables are used to configure the connection to the Data Availability layer (Celestia).

Required:

- DA_AUTH_TOKEN=celetia-rpc-node-auth-token

   Get Authentication token from [rollkit/local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet) with `docker logs celestia_devnet | grep CELESTIA_NODE_AUTH_TOKEN -A 5 | tail -n 1`.
   For Celestia Arabica/Mocha testnet, follow the [RPC-API tutorial](https://docs.celestia.org/developers/rpc-tutorial/#auth-token).

Optional:

- BLOCK_TIME=(default: `10s`)

  Specify time to generate new block in the chain.

- DA_BLOCK_HEIGHT=(default: `0`)

  Configure block height in the DA layer at which the chain will start submitting data.

- DA_NAMESPACE_ID=(default: `67480c4a88c4d12935d4`)

  10 bytes hex encoded value, generate random value using: `openssl rand -hex 10`.

- DA_BASE_URL=(default: `http://celestia-devnet:26658`)

  URL for the base DA client.

- DA_CONFIG=(default: `{"base_url":"'$DA_BASE_URL'","timeout":60000000000,"fee":6000,"gas_limit":6000000,"fee":600000,"auth_token":"'$DA_AUTH_TOKEN'"}`

  Configuration for the DA.

### Cosmos SDK

The following variables are used to configure the Cosmos SDK node.

- VALIDATOR_NAME
  Moniker for the node.
- CHAIN_ID
  ChainID of the rollup.
- KEY_NAME
  The name of the key to use for the genesis account.
- KEY_MNEMONIC
  The mnemonic to use for the genesis account.
- KEY_BACKEND
  The backend type to use for the genesis account.
- TOKEN_AMOUNT
  The amount of tokens to supply the genesis account with.
- STAKING_AMOUNT
  The amount of tokens to use for stake.
- MIN_GAS_PRICE
  The minimum gas prices for accounts executing transactions on this node.
- TOKEN_DENOM
  The denom of the staking token.