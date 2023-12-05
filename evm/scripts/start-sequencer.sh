#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# Required env variables
if [ -z "${DA_AUTH_TOKEN:-}" ]; then
    echo "[x] DA_AUTH_TOKEN (authentication token) required in order to interact with the Celestia Node RPC."
    exit 1
fi

# Cosmos-SDK related vars
NODE_NAME=${NODE_NAME:-"world-engine"}
CHAIN_ID=${CHAIN_ID:-"world-1"}
KEY_NAME=${KEY_NAME:-"world_admin"}
KEY_MNEMONIC=${KEY_MNEMONIC:-"enact adjust liberty squirrel bulk ticket invest tissue antique window thank slam unknown fury script among bread social switch glide wool clog flag enroll"}
KEY_BACKEND=${KEY_BACKEND:-"test"}
TOKEN_AMOUNT=${TOKEN_AMOUNT:-"10000000000000000000000world"}
STAKING_AMOUNT=${STAKING_AMOUNT:-"1000000world"}
MIN_GAS_PRICE=${MIN_GAS_PRICE:-"0world"}
TOKEN_DENOM=${TOKEN_DENOM:-"world"}
FAUCET_ADDR=${FAUCET_ADDR:-"e66d1f367870950190fA3a07D36b26c187a2E578"}

# DA related variables/configuration
DA_BASE_URL="${DA_BASE_URL:-"http://celestia-devnet:26658"}"
DA_BLOCK_HEIGHT=${DA_BLOCK_HEIGHT:-0}
BLOCK_TIME="${BLOCK_TIME:-"10s"}"
# Use 10 bytes hex encoded value (generate random value: `openssl rand -hex 10`)
DA_NAMESPACE_ID="${DA_NAMESPACE_ID:-"67480c4a88c4d12935d4"}"
DA_CONFIG=${DA_CONFIG:-'{"base_url":"'$DA_BASE_URL'","timeout":60000000000,"fee":6000,"gas_limit":6000000,"fee":600000,"auth_token":"'$DA_AUTH_TOKEN'"}'}

echo "DA_NAMESPACE_ID: $DA_NAMESPACE_ID"
echo "DA_CONFIG: $DA_CONFIG"

# World Engine Chain Config & Init
world-evm comet unsafe-reset-all
rm -rf /root/.world-evm/

# Initialize node
world-evm init $NODE_NAME --chain-id $CHAIN_ID --default-denom $TOKEN_DENOM

printf "%s\n\n" "${KEY_MNEMONIC}" | world-evm keys add $KEY_NAME --keyring-backend=$KEY_BACKEND --algo="eth_secp256k1" -i
world-evm genesis add-genesis-account $KEY_NAME $TOKEN_AMOUNT --keyring-backend=$KEY_BACKEND
world-evm genesis gentx $KEY_NAME $STAKING_AMOUNT --chain-id $CHAIN_ID --keyring-backend=$KEY_BACKEND
world-evm genesis collect-gentxs

cp app.toml /root/.world-evm/config/app.toml

sed -i'.bak' 's#"20f33ce90a13a4b5e7697e3544c3083b8f8a51d4"#'"$FAUCET_ADDR"'#g' /root/.world-evm/config/genesis.json
sed -i'.bak' 's#"0x1b1ae4d6e2ef500000"#"0x3fffffffffffffff0000000000000001"#g' /root/.world-evm/config/genesis.json

# start the node.
world-evm start --rollkit.aggregator true --rollkit.da_layer celestia --rollkit.da_config=$DA_CONFIG --rollkit.namespace_id $DA_NAMESPACE_ID --rollkit.da_start_height $DA_BLOCK_HEIGHT --rollkit.block_time $BLOCK_TIME --minimum-gas-prices $MIN_GAS_PRICE --api.enable --api.swagger
