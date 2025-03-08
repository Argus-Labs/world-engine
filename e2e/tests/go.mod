module github.com/argus-labs/world-engine/e2e/tests

go 1.24

replace pkg.world.dev/world-engine/cardinal => ../../cardinal

require (
	github.com/ethereum/go-ethereum v1.14.12
	github.com/rotisserie/eris v0.5.4
	nhooyr.io/websocket v1.8.10
	pkg.world.dev/world-engine/assert v1.0.0
)

require (
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.3.0 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/holiman/uint256 v1.3.2 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/rogpeppe/go-internal v1.13.1 // indirect
	github.com/stretchr/testify v1.10.0 // indirect
	golang.org/x/crypto v0.30.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	gotest.tools/v3 v3.5.1 // indirect
)
