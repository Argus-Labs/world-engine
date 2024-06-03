package cardinal

import (
	"os"
	"reflect"
	"strconv"
	"testing"

	"github.com/naoina/toml"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"pkg.world.dev/world-engine/assert"
)

func TestWorldConfig_loadWorldConfig(t *testing.T) {
	defer CleanupViper(t)

	// Test that loading config prorammatically works
	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, defaultConfig, *cfg)
}

func TestWorldConfig_LoadFromEnv(t *testing.T) {
	defer CleanupViper(t)

	// This target config intentionally does not use the default config values
	// to make sure that all custom config is properly loaded from env vars.
	wantCfg := WorldConfig{
		CardinalNamespace:         "baz",
		CardinalRollupEnabled:     false,
		CardinalLogLevel:          "error",
		CardinalLogPretty:         true,
		RedisAddress:              "localhost:7070",
		RedisPassword:             "bar",
		BaseShardSequencerAddress: "localhost:8080",
		BaseShardRouterKey:        "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ01",
	}

	// Set env vars to target config values
	t.Setenv("CARDINAL_NAMESPACE", wantCfg.CardinalNamespace)
	t.Setenv("CARDINAL_ROLLUP_ENABLED", strconv.FormatBool(wantCfg.CardinalRollupEnabled))
	t.Setenv("CARDINAL_LOG_LEVEL", wantCfg.CardinalLogLevel)
	t.Setenv("CARDINAL_LOG_PRETTY", strconv.FormatBool(wantCfg.CardinalLogPretty))
	t.Setenv("REDIS_ADDRESS", wantCfg.RedisAddress)
	t.Setenv("REDIS_PASSWORD", wantCfg.RedisPassword)
	t.Setenv("BASE_SHARD_SEQUENCER_ADDRESS", wantCfg.BaseShardSequencerAddress)
	t.Setenv("BASE_SHARD_ROUTER_KEY", wantCfg.BaseShardRouterKey)

	gotCfg, err := loadWorldConfig()
	assert.NilError(t, err)

	assert.Equal(t, wantCfg, *gotCfg)
}

func TestWorldConfig_Validate_DefaultConfigIsValid(t *testing.T) {
	// Validates the default config
	assert.NilError(t, defaultConfig.Validate())
}

func TestWorldConfig_Validate_Namespace(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     WorldConfig
		wantErr bool
	}{
		{
			name:    "If Namespace is valid, no errors",
			cfg:     defaultConfigWithOverrides(WorldConfig{CardinalNamespace: "world-1"}),
			wantErr: false,
		},
		{
			name: "If namespace contains anything other than alphanumeric and -, error",
			cfg: defaultConfigWithOverrides(WorldConfig{
				CardinalNamespace: "&1235%^^",
			}),
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				assert.IsError(t, err)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func TestWorldConfig_Validate_LogLevel(t *testing.T) {
	for _, logLevel := range validLogLevels {
		t.Run("If log level is set to "+logLevel+", no errors", func(t *testing.T) {
			cfg := defaultConfigWithOverrides(WorldConfig{CardinalLogLevel: logLevel})
			assert.NilError(t, cfg.Validate())
		})
	}

	t.Run("If log level is invalid, error", func(t *testing.T) {
		cfg := defaultConfigWithOverrides(WorldConfig{CardinalLogLevel: "foo"})
		assert.IsError(t, cfg.Validate())
	})
}

func TestWorldConfig_Validate_RollupMode(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     WorldConfig
		wantErr bool
	}{
		{
			name:    "Without setting base shard configs fails",
			cfg:     defaultConfigWithOverrides(WorldConfig{CardinalRollupEnabled: true}),
			wantErr: true,
		},
		{
			name: "With base shard config, but bad token",
			cfg: defaultConfigWithOverrides(WorldConfig{
				CardinalRollupEnabled:     true,
				BaseShardSequencerAddress: DefaultBaseShardSequencerAddress,
				BaseShardRouterKey:        "not a good token!",
			}),
			wantErr: true,
		},
		{
			name: "With valid base shard config",
			cfg: defaultConfigWithOverrides(WorldConfig{
				CardinalRollupEnabled:     true,
				BaseShardSequencerAddress: "localhost:8080",
				BaseShardRouterKey:        "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ01",
			}),
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.wantErr {
				assert.IsError(t, err)
			} else {
				assert.NilError(t, err)
			}
		})
	}
}

func defaultConfigWithOverrides(overrideCfg WorldConfig) WorldConfig {
	// Iterate over all the fields in the default config and override the ones that are set in the overrideCfg
	// with the values from the overrideCfg.
	cfg := defaultConfig

	for i := range reflect.TypeOf(overrideCfg).NumField() {
		// Get the field name and value from the overrideCfg
		overrideFieldValue := reflect.ValueOf(overrideCfg).Field(i)

		if overrideFieldValue.Kind() == reflect.Ptr {
			// Dereference before checking zero value if it is a pointer
			if !overrideFieldValue.Elem().IsZero() {
				reflect.ValueOf(&cfg).Elem().Field(i).Set(overrideFieldValue)
			}
		} else {
			// If the field is set in the overrideCfg, set it in the default config
			if !overrideFieldValue.IsZero() {
				reflect.ValueOf(&cfg).Elem().Field(i).Set(overrideFieldValue)
			}
		}
	}

	return cfg
}

func TestWorldConfig_loadWorldConfigUsingTomlEnv(t *testing.T) {
	defer CleanupViper(t)

	tomlFilePath := "../e2e/testgames/world.toml"
	t.Setenv(configFilePathEnvVariable, tomlFilePath)

	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, "my-world-e2e", cfg.CardinalNamespace)
}

func TestWorldConfig_loadWorldConfigUsingTomlFlag(t *testing.T) {
	// Save the original values of os.Args and pflag.CommandLine
	originalArgs := os.Args
	originalFlagSet := pflag.CommandLine

	// Ensure they are restored after the test completes
	defer func() {
		os.Args = originalArgs
		pflag.CommandLine = originalFlagSet
		CleanupViper(t)
	}()

	// Set up command-line arguments
	os.Args = []string{"cmd", "--CARDINAL_CONFIG=../e2e/testgames/world.toml"}
	pflag.CommandLine = pflag.NewFlagSet(os.Args[0], pflag.ExitOnError)

	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, "my-world-e2e", cfg.CardinalNamespace)
}

func makeConfigAtPath(t *testing.T, path, namespace string) {
	file, err := os.Create(path)
	assert.NilError(t, err)
	defer file.Close()
	makeConfigAtFile(t, file, namespace)
}

func makeConfigAtFile(t *testing.T, file *os.File, namespace string) {
	data := map[string]any{
		"cardinal": map[string]any{
			"CARDINAL_NAMESPACE": namespace,
		},
	}
	assert.NilError(t, toml.NewEncoder(file).Encode(data))
}

func TestWorldConfig_loadWorldConfigUsingFromCurDir(t *testing.T) {
	defer CleanupViper(t)

	makeConfigAtPath(t, "world.toml", "my-world-current-dir")
	t.Cleanup(func() {
		os.Remove("world.toml")
	})

	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, "my-world-current-dir", cfg.CardinalNamespace)
}

func TestWorldConfig_loadWorldConfigUsingFromParDir(t *testing.T) {
	defer CleanupViper(t)

	makeConfigAtPath(t, "../world.toml", "my-world-parrent-dir")
	t.Cleanup(func() {
		os.Remove("../world.toml")
	})

	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, "my-world-parrent-dir", cfg.CardinalNamespace)
}

func TestWorldConfig_loadWorldConfigUsingOverrideByenv(t *testing.T) {
	defer CleanupViper(t)

	makeConfigAtPath(t, "../world.toml", "my-world-parrent-dir")
	t.Cleanup(func() {
		os.Remove("../world.toml")
	})
	t.Setenv("CARDINAL_NAMESPACE", "my-world-env")

	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, "my-world-env", cfg.CardinalNamespace)
}

// CleanupViper resets Viper configuration
func CleanupViper(t *testing.T) {
	viper.Reset()

	// Optionally, you can also clear environment variables if needed
	for _, key := range viper.AllKeys() {
		err := os.Unsetenv(key)
		if err != nil {
			t.Errorf("failed to unset env var %s: %v", key, err)
		}
	}
}
