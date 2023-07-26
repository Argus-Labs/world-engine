# Polaris Integrated Cosmos Chain

## Installation

### From Binary

The easiest way to install a Cosmos-SDK Blockchain running Polaris is to download a pre-built binary. You can find the latest binaries on the [releases](https://github.com/polaris/releases) page.

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
go run magefiles/setup/setup.go
```

**Step 3: Build the Node Software**

Run the following command to install `world` to your `GOPATH` and build the node. `world` is the node daemon and CLI for interacting with a polaris node.

```bash
mage install
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
  docker compose up chain -d
  ```



## Environment Variables
The following env variables must be set for the following features.

### Game Shard Tx Storage
- USE_SHARD_LISTENER=true
- SHARD_HANDLER_LISTEN_ADDR=<the address you want this server to listen on (i.e. 10.209.21:3090)

### Secure gRPC Connections
For production environments, you'll likely want to setup secure connections between gRPC servers handling system transactions.
To make use of these, set the following environment variables to the path of your SSL certification files:
- SERVER_CERT_PATH=<path/to/server/cert>
- SERVER_KEY_PATH=<path/to/server/key>
- CLIENT_CERT_PATH=<path/to/client/cert>

### Celestia DA Layer Connections
The following variable are use to configure the conenctions to the Data Availability layer (Celestia).

Required:

- DA_AUTH_TOKEN=celetia-rpc-node-auth-token

   Get Authentication token from [rollkit/local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet) with `docker logs celestia_devnet | grep CELESTIA_NODE_AUTH_TOKEN -A 5 | tail -n 1`.
   For Celestia Arabica/Mocha testnet, follow the [RPC-API tutorial](https://docs.celestia.org/developers/rpc-tutorial/#auth-token).

Optional:

- DA_NAMESPACE_ID=(default: `67480c4a88c4d12935d4`)

  10 bytes hex encoded value, generate random value using: `openssl rand -hex 10`.

- DA_BASE_URL=(default: `http://celestia-devnet:26658`)

  Celestia RPC base URL, default value are based on `docker-compose.yml` services URL.

- DA_CONFIG=(default: `{"base_url":"'$DA_BASE_URL'","timeout":60000000000,"fee":6000,"gas_limit":6000000,"fee":600000,"auth_token":"'$DA_AUTH_TOKEN'"}`

  Configure custom json formatted value for `--rollkit.da_config` arguments, see: `chain/scripts/start-rollup.sh`.
