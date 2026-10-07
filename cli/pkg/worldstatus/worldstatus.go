// Package worldstatus holds the status and log shapes shared by the local Docker
// runtime (cli/pkg/local) and the remote Kubernetes reads (cli/pkg/cluster), so
// commands and MCP tools see one model.
package worldstatus

import "time"

// PoolStatus is one world.toml shard pool.
type PoolStatus struct {
	ShardID   string
	PoolSize  int32
	Image     string // image ref the instances run
	Phase     string // Running, Partial, Stopped, NotDeployed
	Instances []InstanceStatus
}

// InstanceStatus is one pool instance (a container locally, a pod remotely).
type InstanceStatus struct {
	Name         string // instance id, e.g. "gameplay-2"
	Runtime      string // container or pod name
	Phase        string // container status / pod phase
	Ready        bool
	RestartCount int32
	Age          string
}

// LogLine is one shard log line.
type LogLine struct {
	ShardID      string // pool id
	InstanceName string
	Source       string // container or pod name
	Timestamp    string
	Line         string
}

// LogsOpts selects what StreamShardLogs tails.
type LogsOpts struct {
	Namespace     string // remote only
	ShardIDs      []string
	InstanceNames []string
	TailLines     int32
	Previous      bool          // remote only
	FollowWindow  time.Duration // stop following after this long; 0 follows until ctx ends
}
