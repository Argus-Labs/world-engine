#!/bin/sh

VALIDATOR_NAME=validator1
CHAIN_ID=argus_90000-1
KEY_NAME=argus-key
CHAINFLAG="--chain-id ${CHAIN_ID}"
TOKEN_AMOUNT="10000000000000000000000000stake"
STAKING_AMOUNT="1000000000stake"

NAMESPACE_ID=$(echo $RANDOM | md5sum | head -c 16; echo;)
echo $NAMESPACE_ID
# DA_BLOCK_HEIGHT=$(curl https://rpc-mocha.pops.one/block | jq -r '.result.block.header.height')
DA_BLOCK_HEIGHT=10

world tendermint unsafe-reset-all
world init $VALIDATOR_NAME --chain-id $CHAIN_ID

printf "enact adjust liberty squirrel bulk ticket invest tissue antique window thank slam unknown fury script among bread social switch glide wool clog flag enroll\n\n" | world keys add $KEY_NAME --keyring-backend="test" -i
world add-genesis-account $KEY_NAME $TOKEN_AMOUNT --keyring-backend test
world gentx $KEY_NAME $STAKING_AMOUNT --chain-id $CHAIN_ID --keyring-backend test
world collect-gentxs
world start --rollkit.aggregator true --rollkit.da_layer celestia --rollkit.da_config='{"base_url":"http://celestia:26659","timeout":60000000000,"fee":6000,"gas_limit":6000000}' --rollkit.namespace_id $NAMESPACE_ID --rollkit.da_start_height $DA_BLOCK_HEIGHT --minimum-gas-prices 0stake
