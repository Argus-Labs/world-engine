package argus

import (
	evmosCodec "github.com/evmos/ethermint/encoding/codec"

	"github.com/argus-labs/argus/app/simparams"
)

// MakeTestEncodingConfig creates an EncodingConfig for testing. This function
// should be used only in tests or when creating a new app instance (NewApp*()).
// App user shouldn't create new codecs - use the app.AppCodec instead.
// [DEPRECATED]
func MakeTestEncodingConfig() simparams.EncodingConfig {
	encodingConfig := simparams.MakeTestEncodingConfig()
	ModuleBasics.RegisterLegacyAminoCodec(encodingConfig.Amino)
	ModuleBasics.RegisterInterfaces(encodingConfig.InterfaceRegistry)

	// Register ethermint types -(THIS ALSO REGISTERS STANDARD COSMOS INTERFACES AND CONCRETE TYPES.)
	evmosCodec.RegisterLegacyAminoCodec(encodingConfig.Amino)
	evmosCodec.RegisterInterfaces(encodingConfig.InterfaceRegistry)
	return encodingConfig
}
