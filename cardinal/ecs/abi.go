package ecs

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"reflect"
	"strings"
)

var goTypeToSolType = map[string]string{
	"bool":           "bool",
	"string":         "string",
	"uint":           "uint64",
	"int":            "int64",
	"common.Address": "address",
}

func GenerateABIType(goStruct any) (*abi.Type, error) {
	rt := reflect.TypeOf(goStruct)
	if rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected input to be of type struct, got %T", goStruct)
	}
	args := make([]abi.ArgumentMarshaling, 0, rt.NumField())

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldType := field.Type.Name()
		solType, err := goTypeToSolidityType(fieldType, field.Tag.Get("solidity"))
		if err != nil {
			return nil, err
		}
		fieldName := field.Name
		args = append(args, abi.ArgumentMarshaling{
			Name: fieldName,
			Type: solType,
		})
	}
	at, err := abi.NewType("tuple", "", args)
	if err != nil {
		return nil, err
	}
	return &at, nil
}

func goTypeToSolidityType(t string, tag string) (string, error) {
	// first handle the most special type. []bytes. this is very specific for ethereum, in that it translates to 'bytes'
	if t == "[]bytes" {
		return "bytes", nil
	}
	// next handle slices, all we do here is check that it contains the brackets,
	// then recursively call this function with everything after the brackets.
	// in solidity, the location of brackets for slice/array declarations is at the end.
	if strings.Contains(t, "[]") {
		inner, err := goTypeToSolidityType(t[2:], tag)
		if err != nil {
			return "", err
		}
		return inner + "[]", nil
	}
	// geth will use *big.Int for uint and int sizes >64 in solidity. structs using this function with *big.Int fields
	// are expected to use a special `solidity` struct tag to indicate the type they want to use here.
	if t == "*big.Int" {
		if tag == "" {
			return "", errors.New("cannot convert *big.Int to solidity type. *big.Int is a special type that " +
				"requires the go struct tag informing the parser whether to convert this to a uint256 or int256 type")
		}
		return tag, nil
	}
	// for everything else, we can just look up in the map.
	solType, ok := goTypeToSolType[t]
	if !ok {
		return "", fmt.Errorf("unrecognized type %s", t)
	}
	return solType, nil
}
