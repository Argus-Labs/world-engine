package epoch

import (
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/internal/command"
)

type Epoch struct {
	EpochHeight uint64
	Hash        []byte
	Ticks       []Tick
}

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
	Commands []command.Command // List of commands executed in this tick
}
