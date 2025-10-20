package otherworld

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Matchmaking is another shard. Just for example send this to itself.
var Matchmaking = cardinal.OtherWorld{ //nolint:gochecknoglobals // it's fine
	Region:       "us-west-2",
	Organization: "organization",
	Project:      "project",
	ShardID:      "cardinal-rampage",
}
