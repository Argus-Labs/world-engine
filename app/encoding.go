package argus

import (
	evmosCodec "github.com/evmos/ethermint/encoding/codec"

	"github.com/argus-labs/argus/app/simulation_params"
)

// MakeTestEncodingConfig creates an EncodingConfig for testing. This function
// should be used only in tests or when creating a new app instance (NewApp*()).
// App user shouldn't create new codecs - use the app.AppCodec instead.
// [DEPRECATED]
func MakeTestEncodingConfig() simulation_params.EncodingConfig {
	encodingConfig := simulation_params.MakeTestEncodingConfig()
	ModuleBasics.RegisterLegacyAminoCodec(encodingConfig.Amino)
	ModuleBasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	// Register ethermint types -(THIS ALSO REGISTERS STANDARD COSMOS INTERFACES AND CONCRETE TYPES.)
	evmosCodec.RegisterLegacyAminoCodec(encodingConfig.Amino)
	evmosCodec.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	return encodingConfig
}
