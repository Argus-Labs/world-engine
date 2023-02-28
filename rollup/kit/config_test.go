package kit

import (
	"fmt"
	"os"
	"testing"

	"github.com/spf13/viper"
	"gotest.tools/assert"
)

func TestConfig(t *testing.T) {
	v := viper.New()
	v.SetConfigName("example")
	v.AddConfigPath(".")
	if err := v.ReadInConfig(); err != nil {
		fmt.Printf("couldn't load config: %s", err)
		os.Exit(1)
	}
	var c Config
	if err := v.Unmarshal(&c); err != nil {
		fmt.Printf("couldn't read config: %s", err)
	}
	assert.Equal(t, c.Rollup.FraudProofs, true)
	assert.Equal(t, c.AppCfg.MinGasPrices, "1stake")
}
