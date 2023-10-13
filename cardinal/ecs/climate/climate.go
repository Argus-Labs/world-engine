package climate

type Climate interface {
	Set(key string, value any)
	Get(key string) any
}

func NewClimate() Climate {
	return &climateImpl{
		m: map[string]any{},
	}
}

type climateImpl struct {
	m map[string]any
}

func (c *climateImpl) Set(key string, value any) {
	c.m[key] = value
}

func (c *climateImpl) Get(key string) any {
	return c.m[key]
}
