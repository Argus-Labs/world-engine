package cardinal

import (
	"reflect"
	"strconv"
	"testing"

	"pkg.world.dev/world-engine/assert"
)

func TestWorldConfig_loadWorldConfig(t *testing.T) {
	// Test that loading config prorammatically works
	cfg, err := loadWorldConfig()
	assert.NilError(t, err)
	assert.Equal(t, defaultConfig, *cfg)
}

func TestWorldConfig_LoadFromEnv(t *testing.T) {
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
		TelemetryEnabled:          true,
		TelemetryStatsdAddress:    "localhost:8125",
		TelemetryTraceAddress:     "localhost:8126",
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
	t.Setenv("TELEMETRY_ENABLED", strconv.FormatBool(wantCfg.TelemetryEnabled))
	t.Setenv("TELEMETRY_STATSD_ADDRESS", wantCfg.TelemetryStatsdAddress)
	t.Setenv("TELEMETRY_TRACE_ADDRESS", wantCfg.TelemetryTraceAddress)

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
				RedisAddress: "&1235%^^",
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

func TestWorldConfig_Validate_Redis(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     WorldConfig
		wantErr bool
	}{
		{
			name:    "If redis address is valid, no errors",
			cfg:     defaultConfigWithOverrides(WorldConfig{RedisAddress: "localhost:6379"}),
			wantErr: false,
		},
		{
			name: "If redis address has invalid format, error",
			cfg: defaultConfigWithOverrides(WorldConfig{
				RedisAddress: "localhost",
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

func TestWorldConfig_Validate_Telemetry(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     WorldConfig
		wantErr bool
	}{
		{
			name: "If telemetry not enabled, don't validate the addresses",
			cfg: defaultConfigWithOverrides(WorldConfig{
				TelemetryEnabled:       false,
				TelemetryStatsdAddress: "foo",
				TelemetryTraceAddress:  "bar",
			}),
			wantErr: false,
		},
		{
			name: "If telemetry is enabled and all addresses are set and valid, no errors",
			cfg: defaultConfigWithOverrides(WorldConfig{
				TelemetryEnabled:       true,
				TelemetryStatsdAddress: "localhost:8125",
				TelemetryTraceAddress:  "localhost:8126",
			}),
			wantErr: false,
		},
		{
			name: "If telemetry is enabled but address is empty, no errors (just log a warning)",
			cfg: defaultConfigWithOverrides(WorldConfig{
				TelemetryEnabled:       true,
				TelemetryStatsdAddress: "",
				TelemetryTraceAddress:  "",
			}),
			wantErr: false,
		},
		{
			name: "If telemetry is enabled, bad statsd address causes validation error",
			cfg: defaultConfigWithOverrides(WorldConfig{
				TelemetryEnabled:       true,
				TelemetryStatsdAddress: "foo",
				TelemetryTraceAddress:  "localhost:8126",
			}),
			wantErr: true,
		},
		{
			name: "If telemetry is enabled, bad trace address causes validation error",
			cfg: defaultConfigWithOverrides(WorldConfig{
				TelemetryEnabled:       true,
				TelemetryStatsdAddress: "localhost:8125",
				TelemetryTraceAddress:  "bar",
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
