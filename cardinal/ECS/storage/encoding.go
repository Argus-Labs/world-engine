package storage

import (
	"bytes"
	"encoding/gob"
)

func Decode[T any](bz []byte) (T, error) {
	var buf bytes.Buffer
	buf.Write(bz)
	dec := gob.NewDecoder(&buf)
	comp := new(T)
	err := dec.Decode(comp)
	var t T
	if err != nil {
		return t, err
	}
	return *comp, nil
}

func Encode(comp any) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	err := enc.Encode(comp)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
