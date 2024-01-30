package ecs

type InternalPlugin interface {
	Register(engine *Engine) error
}
