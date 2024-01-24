package world

type WorldVarsKey string

const (
	PlayerCount = WorldVarsKey("playerCount")
)

// WorldVars Register your own world vars here!
var WorldVars = map[WorldVarsKey]interface{}{
	PlayerCount: 0,
}
