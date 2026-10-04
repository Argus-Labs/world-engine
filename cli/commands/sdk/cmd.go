// Package sdk implements `world sdk` — generate a typed SDK (Go + C#) from a
// backend's Go command structs, via buf+protoc in Docker. Authoring rules are
// enforced by the generator's lint (it names the fix per violation) and documented
// on GenerateCmd.Run; run `world sdk generate --help` for usage.
package sdk

// Cmd is the `world sdk` command group.
type Cmd struct {
	Generate *GenerateCmd `cmd:"" help:"Generate the typed SDK (Go + C#) from backend wire types"`
}
