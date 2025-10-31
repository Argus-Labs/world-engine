//nolint:gochecknoglobals // it's fine
package otherworld

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Matchmaking is another shard. Just for example send this to itself.
var Matchmaking = cardinal.OtherWorld{
	Region:       "us-west-2",
	Organization: "organization",
	Project:      "project",
	ShardID:      "game", // The shard ID of the other shard.
}
