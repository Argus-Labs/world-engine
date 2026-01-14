package micro

import (
	"time"
)

// Tick represents a single execution step in the shard's lifecycle.
type Tick struct {
	Header TickHeader // Metadata about when and which tick this represents
	Data   TickData   // The actual commands processed during this tick
}

// TickHeader contains metadata about the tick execution.
type TickHeader struct {
	TickHeight uint64    // Tick height
	Timestamp  time.Time // When this tick was created
}

// TickData contains the commands that were processed during this tick execution.
type TickData struct {
	Commands []Command // List of commands executed in this tick
}

// Command represents a command from a player or external system.
type Command struct {
	Name    string          // The command name
	Address *ServiceAddress // Service address this command is sent to
	Persona string          // Sender's persona
	Payload any             // The command payload itself
}

type ShardCommand interface {
	Name() string
}
