package assert

import (
	"time"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/rotisserie/eris"
	testify "github.com/stretchr/testify/assert"
	gotest "gotest.tools/v3/assert"
)

type helperT interface {
	Helper()
}

func Assert(t gotest.TestingT, comparison gotest.BoolOrComparison, msgAndArgs ...interface{}) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	gotest.Assert(t, comparison, msgAndArgs...)
}

func Check(t gotest.TestingT, comparison gotest.BoolOrComparison, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return gotest.Check(t, comparison, msgAndArgs...)
}

func NilError(t gotest.TestingT, err error, msgAndArgs ...interface{}) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.NilError(t, err, msgAndArgs...)
}

func Equal(t gotest.TestingT, x, y interface{}, msgAndArgs ...interface{}) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	gotest.Equal(t, x, y, msgAndArgs...)
}

func DeepEqual(t gotest.TestingT, x, y interface{}, opts ...gocmp.Option) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	gotest.DeepEqual(t, x, y, opts...)
}

func Error(t gotest.TestingT, err error, expected string, msgAndArgs ...interface{}) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.Error(t, eris.Cause(err), expected, msgAndArgs...)
}

func ErrorContains(t gotest.TestingT, err error, substring string, msgAndArgs ...interface{}) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.ErrorContains(t, eris.Cause(err), substring, msgAndArgs...)
}

func ErrorIs(t gotest.TestingT, err error, expected error, msgAndArgs ...interface{}) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.ErrorIs(t, eris.Cause(err), eris.Cause(expected), msgAndArgs...)
}

// testify assert wrappers

func FailNow(t testify.TestingT, failureMessage string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.FailNow(t, failureMessage, msgAndArgs...)
}

func Fail(t testify.TestingT, failureMessage string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Fail(t, failureMessage, msgAndArgs...)
}

func IsType(t testify.TestingT, expectedType interface{}, object interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.IsType(t, expectedType, object, msgAndArgs...)
}

func Same(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Same(t, expected, actual, msgAndArgs...)
}

func NotSame(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotSame(t, expected, actual, msgAndArgs...)
}

func EqualValues(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.EqualValues(t, expected, actual, msgAndArgs...)
}

func EqualExportedValues(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.EqualExportedValues(t, expected, actual, msgAndArgs...)
}

func Exactly(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Exactly(t, expected, actual, msgAndArgs...)
}

func NotNil(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotNil(t, object, msgAndArgs...)
}

func Nil(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Nil(t, object, msgAndArgs...)
}

func Empty(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Empty(t, object, msgAndArgs...)
}

func NotEmpty(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotEmpty(t, object, msgAndArgs...)
}

func Len(t testify.TestingT, object interface{}, length int, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Len(t, object, length, msgAndArgs...)
}

func True(t testify.TestingT, value bool, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.True(t, value, msgAndArgs...)
}

func False(t testify.TestingT, value bool, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.False(t, value, msgAndArgs...)
}

func NotEqual(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotEqual(t, expected, actual, msgAndArgs...)
}

func NotEqualValues(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotEqualValues(t, expected, actual, msgAndArgs...)
}

func Contains(t testify.TestingT, s, contains interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Contains(t, s, contains, msgAndArgs...)
}

func NotContains(t testify.TestingT, s, contains interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotContains(t, s, contains, msgAndArgs...)
}

func Subset(t testify.TestingT, list, subset interface{}, msgAndArgs ...interface{}) (ok bool) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Subset(t, list, subset, msgAndArgs...)
}

func NotSubset(t testify.TestingT, list, subset interface{}, msgAndArgs ...interface{}) (ok bool) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotSubset(t, list, subset, msgAndArgs...)
}

func ElementsMatch(t testify.TestingT, listA, listB interface{}, msgAndArgs ...interface{}) (ok bool) {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.ElementsMatch(t, listA, listB, msgAndArgs...)
}

func Condition(t testify.TestingT, comp testify.Comparison, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Condition(t, comp, msgAndArgs...)
}

func Panics(t testify.TestingT, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Panics(t, f, msgAndArgs...)
}

func PanicsWithValue(
	t testify.TestingT, expected interface{}, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.PanicsWithValue(t, expected, f, msgAndArgs...)
}

func PanicsWithError(t testify.TestingT, errString string, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.PanicsWithError(t, errString, f, msgAndArgs...)
}

func NotPanics(t testify.TestingT, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotPanics(t, f, msgAndArgs...)
}

func WithinDuration(
	t testify.TestingT, expected, actual time.Time, delta time.Duration, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.WithinDuration(t, expected, actual, delta, msgAndArgs...)
}

func WithinRange(t testify.TestingT, actual, start, end time.Time, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.WithinRange(t, actual, start, end, msgAndArgs...)
}

func InDelta(t testify.TestingT, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.InDelta(t, expected, actual, delta, msgAndArgs...)
}

func InDeltaSlice(t testify.TestingT, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.InDeltaSlice(t, expected, actual, delta, msgAndArgs...)
}

func InDeltaMapValues(t testify.TestingT, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.InDeltaMapValues(t, expected, actual, delta, msgAndArgs...)
}

func InEpsilon(t testify.TestingT, expected, actual interface{}, epsilon float64, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.InEpsilon(t, expected, actual, epsilon, msgAndArgs...)
}

func InEpsilonSlice(t testify.TestingT, expected, actual interface{}, epsilon float64, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.InEpsilonSlice(t, expected, actual, epsilon, msgAndArgs...)
}

func NoError(t testify.TestingT, err error, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	if err != nil {
		msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	}
	return testify.NoError(t, err, msgAndArgs...)
}

func EqualError(t testify.TestingT, theError error, errString string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	msgAndArgs = append([]interface{}{eris.ToString(theError, true)}, msgAndArgs...)
	return testify.EqualError(t, eris.Cause(theError), errString, msgAndArgs...)
}

func Regexp(t testify.TestingT, rx interface{}, str interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Regexp(t, rx, str, msgAndArgs...)
}

func NotRegexp(t testify.TestingT, rx interface{}, str interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotRegexp(t, rx, str, msgAndArgs...)
}

func Zero(t testify.TestingT, i interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Zero(t, i, msgAndArgs...)
}

func NotZero(t testify.TestingT, i interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotZero(t, i, msgAndArgs...)
}

func FileExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.FileExists(t, path, msgAndArgs...)
}

func NoFileExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NoFileExists(t, path, msgAndArgs...)
}

func DirExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.DirExists(t, path, msgAndArgs...)
}

func NoDirExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NoDirExists(t, path, msgAndArgs...)
}

func JSONEq(t testify.TestingT, expected string, actual string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.JSONEq(t, expected, actual, msgAndArgs...)
}

func YAMLEq(t testify.TestingT, expected string, actual string, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.YAMLEq(t, expected, actual, msgAndArgs...)
}

func Eventually(
	t testify.TestingT,
	condition func() bool,
	waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Eventually(t, condition, waitFor, tick, msgAndArgs...)
}

func EventuallyWithT(
	t testify.TestingT,
	condition func(collect *testify.CollectT),
	waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.EventuallyWithT(t, condition, waitFor, tick, msgAndArgs...)
}

func Never(
	t testify.TestingT,
	condition func() bool, waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Never(t, condition, waitFor, tick, msgAndArgs...)
}

func NotErrorIs(t testify.TestingT, err, target error, msgAndArgs ...interface{}) bool {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.NotErrorIs(t, eris.Cause(err), eris.Cause(target), msgAndArgs...)
}

func IsError(t testify.TestingT, err error, msgAndArgs ...interface{}) bool {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Error(t, err, msgAndArgs...)
}

func IsEqual(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	if ht, ok := t.(helperT); ok {
		ht.Helper()
	}
	return testify.Equal(t, expected, actual, msgAndArgs...)
}

// the following below is covered by gotest and is duplicated by testify
// some signatures are slightly different but overall everything should be covered.

// func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool //renamed IsEqual above.
// func Error(t TestingT, err error, msgAndArgs ...interface{}) bool //renamed to IsError above.
// func ErrorContains(t TestingT, theError error, contains string, msgAndArgs ...interface{}) bool
// func ErrorIs(t TestingT, err, target error, msgAndArgs ...interface{}) bool {
// func ErrorAs(t TestingT, err error, target interface{}, msgAndArgs ...interface{}) bool {
