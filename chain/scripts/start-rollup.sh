#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# Required env variables
if [ -z "${DA_AUTH_TOKEN:-}" ]; then
    echo "[x] DA_AUTH_TOKEN (authentication token) required in order to interact with the Celestia Node RPC."
    exit 1
fi

# Default variables
VALIDATOR_NAME=validator1
CHAIN_ID=argus_90000-1
KEY_NAME=argus-key
TOKEN_AMOUNT="10000000000000000000000000ether"
STAKING_AMOUNT="1000000000ether"

# DA related variables/configuration
DA_BASE_URL="${DA_BASE_URL:-"http://celestia-devnet:26658"}"
DA_BLOCK_HEIGHT=${DA_BLOCK_HEIGHT:-0}
BLOCK_TIME="${BLOCK_TIME:-"10s"}"

# Use 10 bytes hex encoded value (generate random value: `openssl rand -hex 10`)
DA_NAMESPACE_ID="${DA_NAMESPACE_ID:-"67480c4a88c4d12935d4"}"

DA_CONFIG='{"base_url":"'$DA_BASE_URL'","timeout":60000000000,"fee":6000,"gas_limit":6000000,"fee":600000,"auth_token":"'$DA_AUTH_TOKEN'"}'

echo "DA_NAMESPACE_ID: $DA_NAMESPACE_ID"
echo "DA_CONFIG: $DA_CONFIG"

# World Engine Chain Config & Init
world comet unsafe-reset-all
rm -rf /root/.world/

world init $VALIDATOR_NAME --chain-id $CHAIN_ID

printf "enact adjust liberty squirrel bulk ticket invest tissue antique window thank slam unknown fury script among bread social switch glide wool clog flag enroll\n\n" | world keys add $KEY_NAME --keyring-backend="test" --algo="eth_secp256k1" -i
world genesis add-genesis-account $KEY_NAME $TOKEN_AMOUNT --keyring-backend test
world genesis gentx $KEY_NAME $STAKING_AMOUNT --chain-id $CHAIN_ID --keyring-backend test
world genesis collect-gentxs

sed -i'.bak' 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:26657"#g' /root/.world/config/config.toml

sed -i '/api\]/,/\[/ s/enable = false/enable = true/' /root/.world/config/app.toml

# Cosmos SDK gRPC listener
sed -i'.bak' 's#"localhost:9090"#"0.0.0.0:9090"#g' /root/.world/config/app.toml
# Cosmos SDK API server listener
sed -i'.bak' 's#localhost:1317#0.0.0.0:1317#g' /root/.world/config/app.toml

sed -i 's/"stake"/"ether"/g' /root/.world/config/genesis.json

world start --rollkit.aggregator true --rollkit.da_layer celestia --rollkit.da_config=$DA_CONFIG --rollkit.namespace_id $DA_NAMESPACE_ID --rollkit.da_start_height $DA_BLOCK_HEIGHT --rollkit.block_time $BLOCK_TIME --minimum-gas-prices 0eth
