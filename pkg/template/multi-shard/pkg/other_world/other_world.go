package otherworld

import "github.com/argus-labs/world-engine/pkg/cardinal"

// Game returns the example game shard address.
func Game() cardinal.OtherWorld {
	return cardinal.OtherWorld{
		Region:       "us-west1",
		Organization: "organization",
		Project:      "project",
		ShardID:      "game",
	}
}

// Chat returns the example chat shard address.
func Chat() cardinal.OtherWorld {
	return cardinal.OtherWorld{
		Region:       "us-west1",
		Organization: "organization",
		Project:      "project",
		ShardID:      "chat",
	}
}
