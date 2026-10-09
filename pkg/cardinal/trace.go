package cardinal

import "go.opentelemetry.io/otel/attribute"

// Span names and attribute keys emitted by the tick loop. Keep these stable: dashboards and
// alerts key on them.
const (
	spanInit          = "cardinal.init"
	spanRestore       = "cardinal.restore"
	spanTick          = "cardinal.tick"
	spanSystem        = "cardinal.system"
	spanEventDispatch = "cardinal.events.dispatch"
	spanPersistState  = "cardinal.persist_state"

	attrTickHeight   = attribute.Key("cardinal.tick.height")
	attrTickCommands = attribute.Key("cardinal.tick.commands")
	attrSystemName   = attribute.Key("cardinal.system.name")
	attrSystemHook   = attribute.Key("cardinal.system.hook")
	attrSnapshotDue  = attribute.Key("cardinal.snapshot.due")

	attrCommandName   = attribute.Key("cardinal.command.name")
	attrCommandSender = attribute.Key("cardinal.command.sender")
)
