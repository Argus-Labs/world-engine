package storage

/*
import (
	"encoding/json"
	"fmt"
	"reflect"
)

type Any struct {
	TypeUrl string

	Value []byte

	cachedValue interface{}
}

type InterfaceRegistry interface {
	RegisterType(string, any)
	UnpackAny(*Any, any) error
}

type registry struct {
	reg map[string]reflect.Type
}

func NewInterfaceRegistry() InterfaceRegistry {
	return registry{reg: make(map[string]reflect.Type)}
}

func (ir registry) RegisterType(s string, a any) {
	ir.reg[s] = reflect.TypeOf(a)
}

func (ir registry) UnpackAny(any *Any, iface interface{}) error {
	if any == nil {
		return nil
	}

	if any.TypeUrl == "" {
		return nil
	}
	rv := reflect.ValueOf(iface)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("UnpackAny expects a pointer")
	}

	rt := rv.Elem().Type()

	cachedValue := any.cachedValue
	if cachedValue != nil {
		if reflect.TypeOf(cachedValue).AssignableTo(rt) {
			rv.Elem().Set(reflect.ValueOf(cachedValue))
			return nil
		}
	}

	typ, found := ir.reg[any.TypeUrl]
	if !found {
		return fmt.Errorf("no concrete type registered for type URL %s against interface %T", any.TypeUrl, iface)
	}

	msg := reflect.New(typ.Elem()).Elem()

	return json.Unmarshal(any.Value, msg.Addr().Interface())
}

*/
