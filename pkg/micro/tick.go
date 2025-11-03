package micro

import (
	"time"

	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
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
	Commands []Command // List of verified commands executed in this tick
}

// Command represents a signed command from a player or external system.
type Command struct {
	Signature    []byte // Ed25519 signature (64 bytes)
	AuthInfo     AuthInfo
	CommandBytes []byte     // Serialized command bytes
	Command      CommandRaw // Parsed command bytes
}

// CommandRaw contains the actual command type.
type CommandRaw struct {
	Timestamp time.Time   // When the command was created
	Salt      []byte      // Salt for additional uniqueness
	Body      CommandBody // The actual command payload and metadata
}

// CommandBody contains the core command payload.
type CommandBody struct {
	Name    string          // The command name
	Address *ServiceAddress // Service address this command is sent to
	Persona string          // Sender's persona
	Payload any             // The comand payload itself
}

type AuthInfo struct {
	Mode          iscv1.AuthInfo_AuthMode
	SignerAddress []byte // Signer's public key address (32 bytes)
}

type ShardCommand interface {
	Name() string
}
