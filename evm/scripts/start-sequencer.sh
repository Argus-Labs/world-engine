#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# Required env variables
if [[ -z "${DA_AUTH_TOKEN:-}" ]]; then
    echo "[x] DA_AUTH_TOKEN (authentication token) required in order to interact with the Celestia Node RPC."
    exit 1
fi

# Cosmos SDK configs
CHAIN_ID=${CHAIN_ID:-"world-420"}
KEY_MNEMONIC=${KEY_MNEMONIC:-"enact adjust liberty squirrel bulk ticket invest tissue antique window thank slam unknown fury script among bread social switch glide wool clog flag enroll"}
KEY_BACKEND=${KEY_BACKEND:-"test"}
FAUCET_AMOUNT=${FAUCET_AMOUNT:-"10000000000000000000000world"}
TOKEN_DENOM="world"
MIN_GAS_PRICE="0world"
LOG_LEVEL=${LOG_LEVEL:-"info"}

# Faucet configs
FAUCET_ENABLED=${FAUCET_ENABLED:-"true"}

# DA related configs
DA_BASE_URL="${DA_BASE_URL:-"http://celestia-devnet"}"
DA_BLOCK_TIME="${DA_BLOCK_TIME:-"10s"}"
DA_NAMESPACE_ID="${DA_NAMESPACE_ID:-"67480c4a88c4d12935d4"}" # Use 10 bytes hex encoded value (generate random value: `openssl rand -hex 10`)
echo "DA_NAMESPACE_ID: $DA_NAMESPACE_ID"

# Path configs
GENESIS=$HOME/.world-evm/config/genesis.json
TMP_GENESIS=$HOME/.world-evm/config/genesis.json.bak

# Setup local node if an existing one doesn't exist at $HOME/.world-evm
if [[ ! -d "$HOME/.world-evm" ]]; then
  # Initialize node
  MONIKER="world-sequencer"
  world-evm init $MONIKER --chain-id $CHAIN_ID --default-denom $TOKEN_DENOM
    
  # Set client config
  world-evm config set client keyring-backend $KEY_BACKEND
  world-evm config set client chain-id "$CHAIN_ID"


  # -------------------------
  # Setup sequencer account
  # -------------------------
  KEY_SEQUENCER_NAME="sequencer"
  ## Create sequencer account from mnemonic (notice the account number 0 is used in the HD derivation path)
  printf "%s\n\n" "${KEY_MNEMONIC}" | world-evm keys add $KEY_SEQUENCER_NAME --keyring-backend=$KEY_BACKEND --algo="eth_secp256k1" --recover --account 0
  world-evm genesis add-genesis-account $KEY_SEQUENCER_NAME $FAUCET_AMOUNT --keyring-backend=$KEY_BACKEND

  if [[ $FAUCET_ENABLED == "true" ]]; then
      # -------------------------
      # Setup faucet account
      # -------------------------
      KEY_FAUCET_NAME="faucet"
      ## Create faucet account from mnemonic (notice the account number 1 is used in the HD derivation path)
      printf "%s\n\n" "${KEY_MNEMONIC}" | world-evm keys add $KEY_FAUCET_NAME --keyring-backend=$KEY_BACKEND --algo="eth_secp256k1" --recover --account 1
      ## Seed the faucet account with tokens
      world-evm genesis add-genesis-account $KEY_FAUCET_NAME $FAUCET_AMOUNT --keyring-backend=$KEY_BACKEND
  fi 

  # Create genesis stake tx using sequencer account
  world-evm genesis gentx $KEY_SEQUENCER_NAME 1000000000000000000000world --chain-id $CHAIN_ID --keyring-backend=$KEY_BACKEND

  # Collect genesis tx
  world-evm genesis collect-gentxs
  
  # Create sequencer entry (marked as validator, because cosmos sdk) in genesis data
  ADDRESS=$(jq -r '.address' $HOME/.world-evm/config/priv_validator_key.json)
  PUB_KEY=$(jq -r '.pub_key' $HOME/.world-evm/config/priv_validator_key.json)
  jq --argjson pubKey "$PUB_KEY" '.consensus["validators"]=[{"address": "'$ADDRESS'", "pub_key": $pubKey, "power": "1000000000000000", "name": "Rollkit Sequencer"}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  
  # Run this to ensure everything worked and that the genesis file is setup correctly
  world-evm genesis validate-genesis
  
  # Copy app.toml to the home directory
  cp app.toml $HOME/.world-evm/config/app.toml
  
  # CometBFT API
  sed -i'.bak' 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:26657"#g' $HOME/.world-evm/config/config.toml
fi

# set the data availability layer's block height from local-celestia-devnet
DA_BLOCK_HEIGHT=$(curl $DA_BASE_URL:26660/block | jq -r '.result.block.header.height')
echo $DA_BLOCK_HEIGHT

## start the node.
#world-evm start --pruning=nothing --log_level $LOG_LEVEL --api.enabled-unsafe-cors && 
#  --api.enable --api.swagger --minimum-gas-prices=$MIN_GAS_PRICE --rollkit.aggregator true &&
#  --rollkit.da_auth_token=$AUTH_TOKEN --rollkit.da_namespace $DA_NAMESPACE_ID &&
#  --rollkit.da_start_height $DA_BLOCK_HEIGHT --rollkit.da_block_time $DA_BLOCK_TIME

AUTH_TOKEN=$(docker exec $(docker ps -q) celestia bridge auth admin --node.store /home/celestia/bridge)

# Start the node (remove the --pruning=nothing flag if historical queries are not needed)
world-evm start --pruning=nothing --log_level $LOG_LEVEL --api.enabled-unsafe-cors --api.enable --api.swagger --minimum-gas-prices=0.0001world --rollkit.aggregator true --rollkit.da_auth_token=$AUTH_TOKEN --rollkit.da_namespace 00000000000000000000000000000000000000000008e5f679bf7116cb --rollkit.da_start_height $DA_BLOCK_HEIGHT --rollkit.da_block_time 2s