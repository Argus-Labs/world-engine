module github.com/argus-labs/world-engine/nakamabenchmark_test

go 1.21.0

require (
	github.com/ethereum/go-ethereum v1.13.4
	pkg.world.dev/world-engine/assert v0.0.0-00010101000000-000000000000
)

replace pkg.world.dev/world-engine/assert => ../../assert

require (
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/btcsuite/btcd/chaincfg/chainhash v1.0.2 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/holiman/uint256 v1.2.3 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rotisserie/eris v0.5.4 // indirect
	github.com/stretchr/testify v1.8.4 // indirect
	golang.org/x/crypto v0.14.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)
