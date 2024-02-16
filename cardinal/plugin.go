package cardinal

type Plugin interface {
	Register(world *World) error
}
