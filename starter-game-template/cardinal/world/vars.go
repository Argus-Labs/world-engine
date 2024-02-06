package world

type VarsKey string

const (
	PlayerCount = VarsKey("playerCount")
)

// WorldVars Register your own world vars here!
var WorldVars = map[VarsKey]interface{}{
	PlayerCount: 0,
}
