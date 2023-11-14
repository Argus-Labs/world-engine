package codec

import (
	"github.com/goccy/go-json"
)

func Decode[T any](bz []byte) (T, error) {
	comp := new(T)
	err := json.Unmarshal(bz, comp)
	if err != nil {
		return *comp, err
	}
	return *comp, nil
}

func Encode(comp any) ([]byte, error) {
	bz, err := json.Marshal(comp)
	if err != nil {
		return nil, err
	}
	return bz, nil
}
