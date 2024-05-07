#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# Required env variables
if [[ -z "${DA_AUTH_TOKEN:-}" ]]; then
    echo "[x] DA_AUTH_TOKEN (authentication token) required in order to interact with the Celestia Node RPC."
    exit 1
fi

# General configs
LOG_LEVEL=${LOG_LEVEL:-"info"}

# Cosmos SDK configs
CHAIN_TOKEN_DENOM="world"
CHAIN_ID=${CHAIN_ID:-"world-420"}
CHAIN_KEY_MNEMONIC=${CHAIN_KEY_MNEMONIC:-"enact adjust liberty squirrel bulk ticket invest tissue antique window thank slam unknown fury script among bread social switch glide wool clog flag enroll"}
CHAIN_KEY_BACKEND=${CHAIN_KEY_BACKEND:-"test"}
CHAIN_MIN_GAS_PRICE="0.0001world"

# Faucet configs
FAUCET_ENABLED=${FAUCET_ENABLED:-"true"}
FAUCET_ADDRESS=${FAUCET_ADDRESS:-"aa9288F88233Eb887d194fF2215Cf1776a6FEE41"} # ETH address without leading 0x, default: account 0 of CHAIN_KEY_MNEMONIC
FAUCET_AMOUNT=${FAUCET_AMOUNT:-"0x3fffffffffffffff0000000000000001"}

# DA related configs
DA_BASE_URL="${DA_BASE_URL:-"http://celestia-devnet"}"
DA_BLOCK_TIME="${DA_BLOCK_TIME:-"12s"}"
DA_NAMESPACE_ID="${DA_NAMESPACE_ID:-"00000000000000000000000000000000000000000008e5f679bf7116cb"}" # Use 10 bytes hex encoded value (generate random value: `openssl rand -hex 10`)
echo "--> Using DA_NAMESPACE_ID: $DA_NAMESPACE_ID"

# Path configs
GENESIS=$HOME/.world-evm/config/genesis.json
TMP_GENESIS=$HOME/.world-evm/config/genesis.json.bak

# Setup local node if an existing one doesn't exist at $HOME/.world-evm
if [[ ! -d "$HOME/.world-evm" ]]; then
  # Initialize node
  MONIKER="world-sequencer"
  world-evm init $MONIKER --chain-id $CHAIN_ID --default-denom $CHAIN_TOKEN_DENOM
    
  # Set client config
  world-evm config set client keyring-backend $CHAIN_KEY_BACKEND
  world-evm config set client chain-id "$CHAIN_ID"


  # -------------------------
  # Setup sequencer account
  # -------------------------
  SEQUENCER_KEY_NAME="sequencer"
  SEQUENCER_STAKE_AMOUNT="1000000000000000000000world"
  ## Create sequencer account from mnemonic (notice the account number 0 is used in the HD derivation path)
  printf "%s\n\n" "${CHAIN_KEY_MNEMONIC}" | world-evm keys add $SEQUENCER_KEY_NAME --keyring-backend=$CHAIN_KEY_BACKEND --algo="eth_secp256k1" --recover --hd-path "m/44/60/0/0"
  world-evm genesis add-genesis-account $SEQUENCER_KEY_NAME $SEQUENCER_STAKE_AMOUNT --keyring-backend=$CHAIN_KEY_BACKEND

  # -------------------------------
  # Setup faucet account if enabled
  # -------------------------------
  if [[ $FAUCET_ENABLED == "true" ]]; then
      FAUCET_KEY_NAME="faucet"
      ## Create faucet account from mnemonic (notice the account number 1 is used in the HD derivation path)
      printf "%s\n\n" "${CHAIN_KEY_MNEMONIC}" | world-evm keys add $FAUCET_KEY_NAME --keyring-backend=$CHAIN_KEY_BACKEND --algo="eth_secp256k1" --recover --hd-path "m/44/60/1/0"
      ## Seed the faucet account with tokens
      world-evm genesis add-genesis-account $FAUCET_KEY_NAME "10000000000000000000000world" --keyring-backend=$CHAIN_KEY_BACKEND
  fi 

  # Create genesis stake tx using sequencer account
  world-evm genesis gentx $SEQUENCER_KEY_NAME $SEQUENCER_STAKE_AMOUNT --chain-id $CHAIN_ID --keyring-backend=$CHAIN_KEY_BACKEND

  # Collect genesis tx
  world-evm genesis collect-gentxs
  
  # Create sequencer in genesis data
  ADDRESS=$(jq -r '.address' $HOME/.world-evm/config/priv_validator_key.json)
  PUB_KEY=$(jq -r '.pub_key' $HOME/.world-evm/config/priv_validator_key.json)
  jq --argjson pubKey "$PUB_KEY" '.consensus["validators"]=[{"address": "'$ADDRESS'", "pub_key": $pubKey, "power": "1000000000000000", "name": "Rollkit Sequencer"}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
  
  # Update the faucet balance in the genesis file
  if [[ $FAUCET_ENABLED == "true" ]]; then
      sed -i'.bak' "s#20f33ce90a13a4b5e7697e3544c3083b8f8a51d4#$FAUCET_ADDRESS#g" $HOME/.world-evm/config/genesis.json
      sed -i'.bak' "s#0x1b1ae4d6e2ef500000#$FAUCET_AMOUNT#g" $HOME/.world-evm/config/genesis.json
  fi
  
  # Run this to ensure everything worked and that the genesis file is setup correctly
  world-evm genesis validate-genesis
  
  # Copy app.toml to the home directory
  cp app.toml $HOME/.world-evm/config/app.toml
  
  sed -i'.bak' 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:26667"#g' $HOME/.world-evm/config/config.toml
fi

# Set DA layer block height
DA_BLOCK_HEIGHT="null"
while [ "$DA_BLOCK_HEIGHT" == "null" ]; do
    DA_BLOCK_HEIGHT=$(curl $DA_BASE_URL:26657/block --silent | jq -r '.result.block.header.height')
    # Usually you have to wait a little bit until Celestia node runs and we are able to connect to it
    if [ "$DA_BLOCK_HEIGHT" == "null" ]; then
        echo "DA_BLOCK_HEIGHT is null, retrying until Celestia node connects..."
        sleep 1
    fi
done

echo "--> Starting sequencer with DA_BLOCK_HEIGHT: $DA_BLOCK_HEIGHT"

# Start the node (remove the --pruning=nothing flag if historical queries are not needed)
echo "world-evm start --pruning=nothing --log_level $LOG_LEVEL --api.enabled-unsafe-cors --api.enable --api.swagger --minimum-gas-prices=$CHAIN_MIN_GAS_PRICE --rollkit.aggregator true --rollkit.da_auth_token=$DA_AUTH_TOKEN --rollkit.da_namespace $DA_NAMESPACE_ID --rollkit.da_start_height $DA_BLOCK_HEIGHT --rollkit.da_block_time $DA_BLOCK_TIME" 
world-evm start --pruning=nothing --log_level $LOG_LEVEL --api.enabled-unsafe-cors --api.enable --api.swagger --minimum-gas-prices=$CHAIN_MIN_GAS_PRICE --rollkit.aggregator true --rollkit.da_auth_token=$DA_AUTH_TOKEN --rollkit.da_namespace $DA_NAMESPACE_ID --rollkit.da_start_height $DA_BLOCK_HEIGHT --rollkit.da_block_time $DA_BLOCK_TIME