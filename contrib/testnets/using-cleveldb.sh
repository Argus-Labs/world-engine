#!/bin/bash

make install ARGUS_BUILD_OPTIONS="cleveldb"

argusd init "t6" --home ./t6 --chain-id t6

argusd unsafe-reset-all --home ./t6

mkdir -p ./t6/data/snapshots/metadata.db

argusd keys add validator --keyring-backend test --home ./t6

argusd add-genesis-account $(argusd keys show validator -a --keyring-backend test --home ./t6) 100000000stake --keyring-backend test --home ./t6

argusd gentx validator 100000000stake --keyring-backend test --home ./t6 --chain-id t6

argusd collect-gentxs --home ./t6

argusd start --db_backend cleveldb --home ./t6
