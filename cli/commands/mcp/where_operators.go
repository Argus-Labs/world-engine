package mcp

import (
	"math"
	"math/big"
	"reflect"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm/runtime"
	"github.com/rotisserie/eris"
)

// expr casts integer operands to int before comparing or computing on them, so a uint64 above
// [math.MaxInt64] wraps negative: MaxUint64 > 0 is false and MaxUint64 == -1 is true. The where
// clause overloads every operator that makes that cast. Comparisons, % and in work on exact big
// integers when one operand is such a uint64, or an earlier result too large for one, and the other
// is an integer; + - * do so whenever both operands are integers, since they can overflow int
// themselves. Everything else defers to expr's own implementation. Division and ** already convert
// to float64.

// exactIntOperators returns the expr options that install the overloads.
func exactIntOperators() []expr.Option {
	boolOp, valueOp := new(func(any, any) bool), new(func(any, any) any)
	notEqual := func(a, b any) bool { return !runtime.Equal(a, b) }
	operators := []struct {
		token string
		eval  func(a, b any) (any, error)
		sig   any // declared type, which also tells the checker the result type
	}{
		{"==", comparison(runtime.Equal, func(c int) bool { return c == 0 }), boolOp},
		{"!=", comparison(notEqual, func(c int) bool { return c != 0 }), boolOp},
		{"<", comparison(runtime.Less, func(c int) bool { return c < 0 }), boolOp},
		{">", comparison(runtime.More, func(c int) bool { return c > 0 }), boolOp},
		{"<=", comparison(runtime.LessOrEqual, func(c int) bool { return c <= 0 }), boolOp},
		{">=", comparison(runtime.MoreOrEqual, func(c int) bool { return c >= 0 }), boolOp},
		{"+", arithmetic(runtime.Add, (*big.Int).Add), valueOp},
		{"-", arithmetic(runtime.Subtract, (*big.Int).Sub), valueOp},
		{"*", arithmetic(runtime.Multiply, (*big.Int).Mul), valueOp},
		{"%", modulo, valueOp},
		{"in", in, boolOp},
	}

	options := make([]expr.Option, 0, 2*len(operators))
	for _, op := range operators {
		name := "exact " + op.token // the space keeps a where clause from calling it by name
		call := func(params ...any) (any, error) { return op.eval(params[0], params[1]) }
		options = append(options, expr.Function(name, call, op.sig), expr.Operator(op.token, name))
	}

	return options
}

func comparison(fallback func(a, b any) bool, holds func(cmp int) bool) func(a, b any) (any, error) {
	return func(a, b any) (any, error) {
		if x, y, ok := wideInts(a, b); ok {
			return holds(x.Cmp(y)), nil
		}

		return fallback(a, b), nil
	}
}

func arithmetic(fallback func(a, b any) any, op func(z, x, y *big.Int) *big.Int) func(a, b any) (any, error) {
	return func(a, b any) (any, error) {
		if x, y, ok := bigInts(a, b); ok {
			return narrow(op(new(big.Int), x, y)), nil
		}

		return fallback(a, b), nil
	}
}

// modulo truncates like Go's %, which is what expr does for ints.
func modulo(a, b any) (any, error) {
	x, y, ok := wideInts(a, b)
	if !ok {
		return runtime.Modulo(a, b), nil
	}

	if y.Sign() == 0 {
		return nil, eris.New("integer divide by zero")
	}

	return narrow(new(big.Int).Rem(x, y)), nil
}

// in matches slice and array elements with the exact ==, walking them as expr does; that covers
// typed slices such as the []int a range like -1..1 builds. Other containers defer to expr.
func in(needle, haystack any) (any, error) {
	items := reflect.ValueOf(haystack)
	if kind := items.Kind(); kind != reflect.Slice && kind != reflect.Array {
		return runtime.In(needle, haystack), nil
	}

	for i := range items.Len() {
		item := items.Index(i).Interface()

		x, y, wide := wideInts(item, needle)
		if wide && x.Cmp(y) == 0 || !wide && runtime.Equal(item, needle) {
			return true, nil
		}
	}

	return false, nil
}

// wideInts returns both operands as big integers when both are integers and expr's int cast would
// wrap at least one of them.
func wideInts(a, b any) (*big.Int, *big.Int, bool) {
	if !isWide(a) && !isWide(b) {
		return nil, nil, false
	}

	return bigInts(a, b)
}

// bigInts returns both operands as big integers when both are integers.
func bigInts(a, b any) (*big.Int, *big.Int, bool) {
	x, okX := bigInt(a)
	y, okY := bigInt(b)

	return x, y, okX && okY
}

func isWide(v any) bool {
	switch n := v.(type) {
	case uint64:
		return n > math.MaxInt64
	case *big.Int:
		return true
	}

	return false
}

func bigInt(v any) (*big.Int, bool) {
	switch n := v.(type) {
	case int:
		return big.NewInt(int64(n)), true
	case int8:
		return big.NewInt(int64(n)), true
	case int16:
		return big.NewInt(int64(n)), true
	case int32:
		return big.NewInt(int64(n)), true
	case int64:
		return big.NewInt(n), true
	case uint:
		return new(big.Int).SetUint64(uint64(n)), true
	case uint8:
		return new(big.Int).SetUint64(uint64(n)), true
	case uint16:
		return new(big.Int).SetUint64(uint64(n)), true
	case uint32:
		return new(big.Int).SetUint64(uint64(n)), true
	case uint64:
		return new(big.Int).SetUint64(n), true
	case *big.Int:
		return n, true
	}

	return nil, false
}

// narrow returns z as an int or uint64 when one holds it. Larger results stay big, so a later
// operator still sees them exactly; one that isn't overloaded fails on them instead of wrapping.
func narrow(z *big.Int) any {
	switch {
	case z.IsInt64():
		return int(z.Int64())
	case z.IsUint64():
		return z.Uint64()
	}

	return z
}
