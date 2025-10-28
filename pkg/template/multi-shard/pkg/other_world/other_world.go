//nolint:gochecknoglobals // it's fine
package otherworld

import "github.com/argus-labs/world-engine/pkg/cardinal"

// OtherWorld is a way to send commands to other shards.

var Game = cardinal.OtherWorld{
	Region:       "us-west-2",
	Organization: "organization",
	Project:      "project",
	ShardID:      "game",
}

var Chat = cardinal.OtherWorld{
	Region:       "us-west-2",
	Organization: "organization",
	Project:      "project",
	ShardID:      "chat",
}
