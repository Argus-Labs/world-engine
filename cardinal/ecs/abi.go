package ecs

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"reflect"
	"regexp"
	"strings"
)

var (
	hasNumbers = regexp.MustCompile(`\d+`)
)

func GenerateABIType(goStruct any) (*abi.Type, error) {
	rt := reflect.TypeOf(goStruct)
	if rt.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected input to be of type struct, got %T", goStruct)
	}
	args := make([]abi.ArgumentMarshaling, 0, rt.NumField())

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		fieldType := field.Type.String()
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
	at.TupleType = rt
	return &at, nil
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
			return "", errors.New("cannot convert *big.Int to solidity type. *big.Int is a special type that " +
				"requires the go struct tag informing the parser whether to convert this to a uint256 or int256 type")
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
		return "", fmt.Errorf("unsupported type %s", t)
	}

	// finally, check if the uint/int passed contains a size. uint/int without size does not work in ABI->Go.
	if !hasNumbers.MatchString(t) {
		return "", errors.New("cannot use uint/int without specifying size (i.e. uint64, int8, etc)")
	}
	return t, nil

}
