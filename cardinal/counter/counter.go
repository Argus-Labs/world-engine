package counter

import (
	"sync"

	"github.com/rotisserie/eris"
)

type Counter struct {
	items map[string]uint64
	mutex sync.Mutex
}

func NewCounter() Counter {
	return Counter{
		items: make(map[string]uint64),
		mutex: sync.Mutex{},
	}
}

func (c *Counter) GetCount(key string) (uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	res, ok := c.items[key]
	if !ok {
		return 0, eris.Errorf("key: %s does not exist", key)
	}
	return res, nil
}

func (c *Counter) GetAllCounts() (map[string]uint64, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	res := make(map[string]uint64)
	for key, value := range c.items {
		res[key] = value
	}
	return res, nil
}

func (c *Counter) Add(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	v, ok := c.items[key]
	if !ok {
		c.items[key] = uint64(1)
		return
	}
	c.items[key] = v + 1
}
