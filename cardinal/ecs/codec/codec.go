package codec

import (
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
)

func Decode[T any](bz []byte) (T, error) {
	comp := new(T)
	err := json.Unmarshal(bz, comp)
	if err != nil {
		return *comp, eris.Wrap(err, "")
	}
	return *comp, nil
}

func Encode(comp any) ([]byte, error) {
	bz, err := json.Marshal(comp)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return bz, nil
}
