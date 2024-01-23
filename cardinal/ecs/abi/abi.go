package abi

import (
	"encoding/json"
	"reflect"
	"regexp"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/rotisserie/eris"
)

const (
	bigIntStructTag = "evm"
)

var (
	hasNumbers = regexp.MustCompile(`\d+`)
)

func GenerateABIType(goStruct any) (*abi.Type, error) {
	rt := reflect.TypeOf(goStruct)
	if rt.Kind() != reflect.Struct {
		return nil, eris.Errorf("expected input to be of type struct, got %T", goStruct)
	}
	args, err := getArgumentsForType(rt)
	if err != nil {
		return nil, err
	}
	at, err := abi.NewType("tuple", "", args)
	if err != nil {
		return nil, eris.Wrap(err, "")
	}
	return &at, nil
}

func getArgumentsForType(rt reflect.Type) ([]abi.ArgumentMarshaling, error) {
	args := make([]abi.ArgumentMarshaling, 0, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		kind := field.Type.Kind()
		fieldType := field.Type.String()
		fieldName := field.Name

		// handle the special case for slice of struct fields.
		if kind == reflect.Slice {
			if field.Type.Elem().Kind() == reflect.Struct {
				arg, err := goStructToEVMStruct(field.Type.Elem(), fieldName)
				if err != nil {
					return nil, err
				}
				arg.Type = "tuple[]"
				args = append(args, arg)
				continue
			}
		}

		// handle special case for struct fields.
		if kind == reflect.Struct {
			arg, err := goStructToEVMStruct(field.Type, fieldName)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			continue
		}

		// all other fields can be handled normally.
		solType, err := goTypeToSolidityType(fieldType, field.Tag.Get(bigIntStructTag))
		if err != nil {
			return nil, err
		}
		args = append(args, abi.ArgumentMarshaling{
			Name: fieldName,
			Type: solType,
		})
	}
	return args, nil
}

func goStructToEVMStruct(p reflect.Type, fieldName string) (abi.ArgumentMarshaling, error) {
	components, err := getArgumentsForType(p)
	if err != nil {
		return abi.ArgumentMarshaling{}, err
	}
	arg := abi.ArgumentMarshaling{
		Name:       fieldName,
		Type:       "tuple",
		Components: components,
	}
	return arg, nil
}

func goTypeToSolidityType(t string, tag string) (string, error) {
	// first handle the most special type. []byte. this is very specific for ethereum, in that it translates to 'bytes'
	if t == "[]byte" {
		return "bytes", nil
	}
	// next handle slices, all we do here is check that it contains the brackets,
	// then recursively call this function with everything after the brackets.
	if strings.Contains(t, "[]") {
		inner, err := goTypeToSolidityType(t[2:], tag)
		if err != nil {
			return "", err
		}
		// in solidity, the location of brackets for slice/array declarations is at the end.
		return inner + "[]", nil
	}
	// geth will use *big.Int for uint and int sizes >64 in solidity. structs using this function with *big.Int fields
	// are expected to use a special `solidity` struct tag to indicate the type they want to use here.
	if t == "*big.Int" {
		if tag == "" {
			return "", eris.Errorf("when using *big.Int, you MUST use the `%s` struct tag to indicate which "+
				"underlying evm integer type you wish to resolve to (i.e. uint256, int128, etc)", bigIntStructTag)
		}
		return tag, nil
	}

	if t == "common.Address" {
		return "address", nil
	}

	if t == "string" || t == "bool" {
		return t, nil
	}

	// the final type we can support is int/uint, so if we don't have that by here, we error.
	if !strings.Contains(t, "int") {
		return "", eris.Errorf("unsupported type %s", t)
	}

	// finally, check if the uint/int passed contains a size. uint/int without size does not work in ABI->Go.
	if !hasNumbers.MatchString(t) {
		return "", eris.New("cannot use uint/int without specifying size (i.e. uint64, int8, etc)")
	}
	return t, nil
}

// SerdeInto takes an interface with JSON tags, serializes it to JSON, then deserializes it to the specified type param.
func SerdeInto[T any](iface interface{}) (T, error) {
	v := new(T)
	bz, err := json.Marshal(iface)
	if err != nil {
		return *v, eris.Wrap(err, "")
	}

	err = json.Unmarshal(bz, v)
	return *v, eris.Wrap(err, "")
}
