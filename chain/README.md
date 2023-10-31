# Polaris Integrated Cosmos Chain

## Installation

### From Binary

The easiest way to install a Cosmos-SDK Blockchain running Polaris is to download a pre-built binary. You can find the latest binaries on the [releases](https://github.com/polaris/releases) page.

### Makefile

To install the World Engine blockchain in your bin, making it globally accessable from your terminal, run this command in the `chain` directory:

```bash
make install
```

To verify installation was successful, run:

```bash
world version
```

### From Prebuilt Docker Image

Pull `chain` prebuild Docker Image:
```bash
docker pull us-docker.pkg.dev/argus-labs/world-engine/chain:<latest/tag_version>
```

Run `chain` container, supply the DA_BASE_URL and DA_AUTH_TOKEN environment variables accordingly:
```bash
docker run -it --rm -e DA_BASE_URL=http://celestia-da-layer-url:26658 -e DA_AUTH_TOKEN=celestia-da-later-token  us-docker.pkg.dev/argus-labs/world-engine/chain:latest
```

See the Docker Compose section below for instructions on running both the `chain` and `Celestia Devnet` stack.

### From Source

**Step 1: Install Golang & Foundry**

Go v1.20+ or higher is required for Polaris

1. Install [Go 1.20+ from the official site](https://go.dev/dl/) or the method of your choice. Ensure that your `GOPATH` and `GOBIN` environment variables are properly set up by using the following commands:

   For Ubuntu:

   ```sh
   cd $HOME
   sudo apt-get install golang -y
   export PATH=$PATH:/usr/local/go/bin
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

   For Mac:

   ```sh
   cd $HOME
   brew install go
   export PATH=$PATH:/opt/homebrew/bin/go
   export PATH=$PATH:$(go env GOPATH)/bin
   ```

2. Confirm your Go installation by checking the version:

   ```sh
   go version
   ```

[Foundry](https://book.getfoundry.sh/getting-started/installation) is required for Polaris

3. Install Foundry:
   ```sh
   curl -L https://foundry.paradigm.xyz | bash
   ```

**Step 2: Get Polaris source code**

Clone the `polaris` repo from the [official repo](https://github.com/berachain/polaris/) and check
out the `main` branch for the latest stable release.
Build the binary.

```bash
cd $HOME
git clone https://github.com/berachain/polaris
cd polaris
git checkout main
```

**Step 3: Build the Node Software**

Run the following command to install `world` to your `GOPATH` and build the node. `world` is the node daemon and CLI for interacting with a polaris node.

```bash
make install
```

**Step 4: Verify your installation**

Verify your installation with the following command:

```bash
world version --long
```

A successful installation will return the following:

```bash
name: world-engine
server_name: world
version: <x.x.x>
commit: <Commit hash>
build_tags: netgo,ledger
go: go version go1.20.4 darwin/amd64
```

## Running a Local Network

After ensuring dependecies are installed correctly, run the following command to start a local development network.

```bash
mage start
```

## Running using Docker Compose

Start the `chain` and `celestia-devnet` using `chain/docker-compose.yml`, make sure to follow these steps:
- Start local-celestia-devnet
  ```
  docker compose up celestia-devnet -d --wait
  ```

- Get DA_AUTH_TOKEN (Celestia RPC Authentication token) from celestia_devnet logs.
  ```
  export DA_AUTH_TOKEN=$(docker logs celestia_devnet 2>&1 | grep CELESTIA_NODE_AUTH_TOKEN -A 5 | tail -n 1)
  echo "Auth Token >> $DA_AUTH_TOKEN"
  ```

- Start the `chain` / `evm_base_shard`
  ```
  docker compose up chain --build --detach
  ```

## Features

### Game Shard Tx Sequencer

The rollup is extended via a special gRPC server that game shards can connect to for the purpose of submitting and storing transactions to the base shard.

This gRPC server runs, by default, at port `9601`, but can be configured by setting the `SHARD_SEQUENCER_PORT` environment variable.

### Router

The rollup provides an extension to it's underlying EVM environment with a specialized precompile that allows messages to be forwarded from smart contracts to game shards that implement the router server.

The router must be informed of the game shard's server address by setting the environment variable `CARDINAL_EVM_LISTENER_ADDR`. 

#### Using the Router in Solidity

In order to use the precompile, you first need to copy over the precompile contract code. The contract lives at:

`chain/contracts/src/cosmos/precompile/router.sol`

The precompile address will always be `0x356833c4666fFB6bFccbF8D600fa7282290dE073`.

Instantiating the precompile is done like so:

```solidity
// the path of import will change depending on where you copied the 
// precompile contract code to.
import {IRouter} from "./precompile/router.sol";

contract SomeGame {
    IRouter public immutable router;

    constructor () {
        router = IRouter(0x356833c4666fFB6bFccbF8D600fa7282290dE073);
    }
}

```

## Environment Variables
The following env variables must be set for the following features.

### Secure gRPC Connections
For production environments, you'll likely want to setup secure connections between gRPC servers handling system transactions.
To make use of these, set the following environment variables to the path of your SSL certification files:
- SERVER_CERT_PATH=<path/to/server/cert>
- SERVER_KEY_PATH=<path/to/server/key>
- CLIENT_CERT_PATH=<path/to/client/cert>

### Celestia DA Layer Connections
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

  Celestia RPC base URL, default value are based on `docker-compose.yml` services URL.

- DA_CONFIG=(default: `{"base_url":"'$DA_BASE_URL'","timeout":60000000000,"fee":6000,"gas_limit":6000000,"fee":600000,"auth_token":"'$DA_AUTH_TOKEN'"}`

  Configure custom json formatted value for `--rollkit.da_config` arguments, see: `chain/scripts/start-rollup.sh`.
