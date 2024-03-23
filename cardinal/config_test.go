package cardinal

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
)

func TestWorldConfig_Defaults(t *testing.T) {
	cfg := LoadWorldConfig()
	assert.Equal(t, cfg, defaultConfig)
}

func TestWorldConfig_LoadFromEnv(t *testing.T) {
	wantCfg := WorldConfig{
		RedisAddress:              "localhost:6379",
		RedisPassword:             "bar",
		CardinalNamespace:         "baz",
		CardinalMode:              RunModeProd,
		BaseShardSequencerAddress: "localhost:8080",
		BaseShardQueryAddress:     "localhost:8081",
		CardinalLogLevel:          DefaultLogLevel,
		StatsdAddress:             DefaultStatsdAddress,
	}
	t.Setenv("REDIS_ADDRESS", wantCfg.RedisAddress)
	t.Setenv("REDIS_PASSWORD", wantCfg.RedisPassword)
	t.Setenv("CARDINAL_NAMESPACE", wantCfg.CardinalNamespace)
	t.Setenv("CARDINAL_MODE", string(wantCfg.CardinalMode))
	t.Setenv("BASE_SHARD_SEQUENCER_ADDRESS", wantCfg.BaseShardSequencerAddress)
	t.Setenv("BASE_SHARD_Query_ADDRESS", wantCfg.BaseShardQueryAddress)

	gotCfg := LoadWorldConfig()

	assert.Equal(t, wantCfg, gotCfg)
}

func TestWorldConfig_Validate(t *testing.T) {
	testCases := []struct {
		name    string
		cfg     WorldConfig
		wantErr bool
	}{
		{
			name:    "default should work, its devmode",
			cfg:     defaultConfig,
			wantErr: false,
		},
		{
			name:    "prod without setting other values fails",
			cfg:     WorldConfig{CardinalMode: RunModeProd},
			wantErr: true,
		},
		{
			name:    "prod with only redis pass",
			cfg:     WorldConfig{CardinalMode: RunModeProd, RedisPassword: "foo"},
			wantErr: true,
		},
		{
			name:    "prod with redis pass + namespace",
			cfg:     WorldConfig{CardinalMode: RunModeProd, RedisPassword: "foo", CardinalNamespace: "foo"},
			wantErr: true,
		},
		{
			name: "prod with all required values",
			cfg: WorldConfig{
				CardinalMode:              RunModeProd,
				RedisAddress:              "localhost:6379",
				RedisPassword:             "foo",
				CardinalNamespace:         "foo",
				BaseShardQueryAddress:     "localhost:8081",
				BaseShardSequencerAddress: "localhost:8080",
			},
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
