#!/bin/sh

set -o errexit -o nounset

rm -rf root/.argus/config/
rm -rf ~/.argus/

CHAINID=argus_9000-1

MNEMONIC="document reveal rug gorilla office card impulse virus intact legend suspect warfare cheap ribbon express barrel throw keep rapid direct order annual town gold"


# Build genesis file incl account for passed address
coins="10000000000stake,100000000000samoleans,1000000000000000000aphoton"
argusd init --chain-id $CHAINID $CHAINID
echo $MNEMONIC | argusd keys add validator --recover --keyring-backend="test"
echo  $(argusd keys show validator -a --keyring-backend="test") $coins
argusd add-genesis-account $(argusd keys show validator -a --keyring-backend="test") $coins
argusd gentx validator 5000000000stake --keyring-backend="test" --chain-id $CHAINID
argusd collect-gentxs

# Set proper defaults and change ports


# Start the argus node
# argusd start --rollmint.aggregator true --rollmint.da_layer celestia --rollmint.da_config='{"base_url":"http://localhost:26659","timeout":60000000000,"gas_limit":6000000}' --rollmint.namespace_id 000000000000FFFF --rollmint.da_start_height 100783 --minimum-gas-prices 0stake
argusd start --minimum-gas-prices 0stake
