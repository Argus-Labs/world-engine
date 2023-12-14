package cardinal

import (
	"pkg.world.dev/world-engine/assert"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	cfg := GetWorldConfig()
	assert.Equal(t, cfg, defaultConfig)
}

func TestConfigLoadsFromEnv(t *testing.T) {
	wantCfg := WorldConfig{
		RedisAddress:              "foo",
		RedisPassword:             "bar",
		CardinalNamespace:         "baz",
		CardinalMode:              DeployModeProd,
		BaseShardSequencerAddress: "moo",
	}
	t.Setenv("REDIS_ADDRESS", wantCfg.RedisAddress)
	t.Setenv("REDIS_PASSWORD", wantCfg.RedisPassword)
	t.Setenv("CARDINAL_NAMESPACE", wantCfg.CardinalNamespace)
	t.Setenv("CARDINAL_MODE", string(wantCfg.CardinalMode))
	t.Setenv("BASE_SHARD_SEQUENCER_ADDRESS", wantCfg.BaseShardSequencerAddress)

	gotCfg := GetWorldConfig()

	assert.Equal(t, wantCfg, gotCfg)
}
