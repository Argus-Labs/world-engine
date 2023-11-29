package counter

import (
	"sync"

	"github.com/rotisserie/eris"
)

type Counter struct {
	items sync.Map
}

func (c *Counter) GetCount(key string) (uint64, error) {
	amount, ok := c.items.Load(key)
	if !ok {
		return 0, eris.Errorf("key: %s does not exist", key)
	}
	amountInt, ok := amount.(uint64)
	if !ok {
		return 0, eris.Errorf("stored type for %s is not uint64", key)
	}
	return amountInt, nil
}

func (c *Counter) GetAllCounts() (map[string]uint64, error) {
	result := map[string]uint64{}
	var err error
	c.items.Range(func(key any, value any) bool {
		valueInt, ok := value.(uint64)
		if !ok {
			err = eris.Errorf("stored type for key %s is not uint64", key)
			return false
		}
		keyString, ok := key.(string)
		if !ok {
			err = eris.New("stored key is not a string")
			return false
		}
		result[keyString] = valueInt
		return true
	})

	return result, err
}

func (c *Counter) Add(key string) error {
	var ok bool
	var v any
	var vint uint64
	f := func() error {
		v, ok = c.items.Load(key)
		if !ok {
			return eris.Errorf("key: %s does not exist", key)
		}
		vint, ok = v.(uint64)
		if !ok {
			return eris.Errorf("stored type for key %s is not uint64", key)
		}
		return nil
	}
	if err := f(); err != nil {
		return err
	}

	// might starve
	for c.items.CompareAndSwap(key, vint, vint+1) {
		if err := f(); err != nil {
			return err
		}
	}
	return nil
}
