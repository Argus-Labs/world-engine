package otherworld

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Matchmaking returns the example destination. It sends to this shard itself.
func Matchmaking() cardinal.OtherWorld {
	return cardinal.OtherWorld{
		Region:       "us-west1",
		Organization: "organization",
		Project:      "project",
		ShardID:      "game", // The shard ID of the other shard.
	}
}
