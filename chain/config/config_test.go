package config

import (
	"testing"

	"github.com/spf13/viper"
	"gotest.tools/v3/assert"
)

func TestConfigFromToml(t *testing.T) {
	expectedCfg := WorldEngineConfig{
		DisplayDenom:    "dark",
		BaseDenom:       "adark",
		Bech32Prefix:    "darkforest",
		RouterAuthority: "",
	}
	v := viper.New()
	v.AddConfigPath(".")
	v.SetConfigName("example")
	err := v.ReadInConfig()
	assert.NilError(t, err)

	cfg := WorldEngineConfig{}
	err = v.Unmarshal(&cfg)
	assert.NilError(t, err)

	assert.Equal(t, expectedCfg, cfg)
}
