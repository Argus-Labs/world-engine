#!/bin/sh

set -o errexit -o nounset

rm -rf root/.argus/config/
rm -rf ~/.argus/

CHAINID=foobar



# Build genesis file incl account for passed address
coins="10000000000stake,100000000000samoleans"
gaiad init --chain-id $CHAINID $CHAINID
gaiad keys add validator --keyring-backend="test"
gaiad add-genesis-account "$(gaiad keys show validator -a --keyring-backend="test")" $coins
gaiad gentx validator 5000000000stake --keyring-backend="test" --chain-id $CHAINID
gaiad collect-gentxs

# Set proper defaults and change ports


# Start the gaia
# gaiad start --rollmint.aggregator true --rollmint.da_layer celestia --rollmint.da_config='{"base_url":"http://localhost:26659","timeout":60000000000,"gas_limit":6000000}' --rollmint.namespace_id 000000000000FFFF --rollmint.da_start_height 100783 --minimum-gas-prices 0stake
gaiad start --minimum-gas-prices 0stake
