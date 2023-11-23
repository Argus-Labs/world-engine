package assert

import (
	"time"

	gotest "gotest.tools/v3/assert"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/rotisserie/eris"
	testify "github.com/stretchr/testify/assert"
)

func Assert(t gotest.TestingT, comparison gotest.BoolOrComparison, msgAndArgs ...interface{}) {
	gotest.Assert(t, comparison, msgAndArgs...)
}

func Check(t gotest.TestingT, comparison gotest.BoolOrComparison, msgAndArgs ...interface{}) bool {
	return gotest.Check(t, comparison, msgAndArgs...)
}

func NilError(t gotest.TestingT, err error, msgAndArgs ...interface{}) {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.NilError(t, err, msgAndArgs...)
}

func Equal(t gotest.TestingT, x, y interface{}, msgAndArgs ...interface{}) {
	gotest.Equal(t, x, y, msgAndArgs...)
}

func DeepEqual(t gotest.TestingT, x, y interface{}, opts ...gocmp.Option) {
	gotest.DeepEqual(t, x, y, opts...)
}

func Error(t gotest.TestingT, err error, expected string, msgAndArgs ...interface{}) {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.Error(t, eris.Cause(err), expected, msgAndArgs...)
}

func ErrorContains(t gotest.TestingT, err error, substring string, msgAndArgs ...interface{}) {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.ErrorContains(t, eris.Cause(err), substring, msgAndArgs...)
}

func ErrorIs(t gotest.TestingT, err error, expected error, msgAndArgs ...interface{}) {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	gotest.ErrorIs(t, eris.Cause(err), eris.Cause(expected), msgAndArgs...)
}

// testify assert wrappers

func FailNow(t testify.TestingT, failureMessage string, msgAndArgs ...interface{}) bool {
	return testify.FailNow(t, failureMessage, msgAndArgs...)
}

func Fail(t testify.TestingT, failureMessage string, msgAndArgs ...interface{}) bool {
	return testify.Fail(t, failureMessage, msgAndArgs...)
}

func IsType(t testify.TestingT, expectedType interface{}, object interface{}, msgAndArgs ...interface{}) bool {
	return testify.IsType(t, expectedType, object, msgAndArgs...)
}

func Same(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.Same(t, expected, actual, msgAndArgs...)
}

func NotSame(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotSame(t, expected, actual, msgAndArgs...)
}

func EqualValues(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.EqualValues(t, expected, actual, msgAndArgs...)
}

func EqualExportedValues(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.EqualExportedValues(t, expected, actual, msgAndArgs...)
}

func Exactly(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.Exactly(t, expected, actual, msgAndArgs...)
}

func NotNil(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotNil(t, object, msgAndArgs...)
}

func Nil(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	return testify.Nil(t, object, msgAndArgs...)
}

func Empty(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	return testify.Empty(t, object, msgAndArgs...)
}

func NotEmpty(t testify.TestingT, object interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotEmpty(t, object, msgAndArgs...)
}

func Len(t testify.TestingT, object interface{}, length int, msgAndArgs ...interface{}) bool {
	return testify.Len(t, object, length, msgAndArgs...)
}

func True(t testify.TestingT, value bool, msgAndArgs ...interface{}) bool {
	return testify.True(t, value, msgAndArgs...)
}

func False(t testify.TestingT, value bool, msgAndArgs ...interface{}) bool {
	return testify.False(t, value, msgAndArgs...)
}

func NotEqual(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotEqual(t, expected, actual, msgAndArgs...)
}

func NotEqualValues(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotEqualValues(t, expected, actual, msgAndArgs...)
}

func Contains(t testify.TestingT, s, contains interface{}, msgAndArgs ...interface{}) bool {
	return testify.Contains(t, s, contains, msgAndArgs...)
}

func NotContains(t testify.TestingT, s, contains interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotContains(t, s, contains, msgAndArgs...)
}

func Subset(t testify.TestingT, list, subset interface{}, msgAndArgs ...interface{}) (ok bool) {
	return testify.Subset(t, list, subset, msgAndArgs...)
}

func NotSubset(t testify.TestingT, list, subset interface{}, msgAndArgs ...interface{}) (ok bool) {
	return testify.NotSubset(t, list, subset, msgAndArgs...)
}

func ElementsMatch(t testify.TestingT, listA, listB interface{}, msgAndArgs ...interface{}) (ok bool) {
	return testify.ElementsMatch(t, listA, listB, msgAndArgs...)
}

func Condition(t testify.TestingT, comp testify.Comparison, msgAndArgs ...interface{}) bool {
	return testify.Condition(t, comp, msgAndArgs...)
}

func Panics(t testify.TestingT, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	return testify.Panics(t, f, msgAndArgs...)
}

func PanicsWithValue(
	t testify.TestingT, expected interface{}, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	return testify.PanicsWithValue(t, expected, f, msgAndArgs...)
}

func PanicsWithError(t testify.TestingT, errString string, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	return testify.PanicsWithError(t, errString, f, msgAndArgs...)
}

func NotPanics(t testify.TestingT, f testify.PanicTestFunc, msgAndArgs ...interface{}) bool {
	return testify.NotPanics(t, f, msgAndArgs...)
}

func WithinDuration(
	t testify.TestingT, expected, actual time.Time, delta time.Duration, msgAndArgs ...interface{}) bool {
	return testify.WithinDuration(t, expected, actual, delta, msgAndArgs...)
}

func WithinRange(t testify.TestingT, actual, start, end time.Time, msgAndArgs ...interface{}) bool {
	return testify.WithinRange(t, actual, start, end, msgAndArgs...)
}

func InDelta(t testify.TestingT, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	return testify.InDelta(t, expected, actual, delta, msgAndArgs...)
}

func InDeltaSlice(t testify.TestingT, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	return testify.InDeltaSlice(t, expected, actual, delta, msgAndArgs...)
}

func InDeltaMapValues(t testify.TestingT, expected, actual interface{}, delta float64, msgAndArgs ...interface{}) bool {
	return testify.InDeltaMapValues(t, expected, actual, delta, msgAndArgs...)
}

func InEpsilon(t testify.TestingT, expected, actual interface{}, epsilon float64, msgAndArgs ...interface{}) bool {
	return testify.InEpsilon(t, expected, actual, epsilon, msgAndArgs...)
}

func InEpsilonSlice(t testify.TestingT, expected, actual interface{}, epsilon float64, msgAndArgs ...interface{}) bool {
	return testify.InEpsilonSlice(t, expected, actual, epsilon, msgAndArgs...)
}

func NoError(t testify.TestingT, err error, msgAndArgs ...interface{}) bool {
	if err != nil {
		msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	}
	return testify.NoError(t, err, msgAndArgs...)
}

func EqualError(t testify.TestingT, theError error, errString string, msgAndArgs ...interface{}) bool {
	msgAndArgs = append([]interface{}{eris.ToString(theError, true)}, msgAndArgs...)
	return testify.EqualError(t, eris.Cause(theError), errString, msgAndArgs...)
}

func Regexp(t testify.TestingT, rx interface{}, str interface{}, msgAndArgs ...interface{}) bool {
	return testify.Regexp(t, rx, str, msgAndArgs...)
}

func NotRegexp(t testify.TestingT, rx interface{}, str interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotRegexp(t, rx, str, msgAndArgs...)
}

func Zero(t testify.TestingT, i interface{}, msgAndArgs ...interface{}) bool {
	return testify.Zero(t, i, msgAndArgs...)
}

func NotZero(t testify.TestingT, i interface{}, msgAndArgs ...interface{}) bool {
	return testify.NotZero(t, i, msgAndArgs...)
}

func FileExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	return testify.FileExists(t, path, msgAndArgs...)
}

func NoFileExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	return testify.NoFileExists(t, path, msgAndArgs...)
}

func DirExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	return testify.DirExists(t, path, msgAndArgs...)
}

func NoDirExists(t testify.TestingT, path string, msgAndArgs ...interface{}) bool {
	return testify.NoDirExists(t, path, msgAndArgs...)
}

func JSONEq(t testify.TestingT, expected string, actual string, msgAndArgs ...interface{}) bool {
	return testify.JSONEq(t, expected, actual, msgAndArgs...)
}

func YAMLEq(t testify.TestingT, expected string, actual string, msgAndArgs ...interface{}) bool {
	return testify.YAMLEq(t, expected, actual, msgAndArgs...)
}

func Eventually(
	t testify.TestingT,
	condition func() bool,
	waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) bool {
	return testify.Eventually(t, condition, waitFor, tick, msgAndArgs...)
}

func EventuallyWithT(
	t testify.TestingT,
	condition func(collect *testify.CollectT),
	waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) bool {
	return testify.EventuallyWithT(t, condition, waitFor, tick, msgAndArgs...)
}

func Never(
	t testify.TestingT,
	condition func() bool, waitFor time.Duration, tick time.Duration, msgAndArgs ...interface{}) bool {
	return testify.Never(t, condition, waitFor, tick, msgAndArgs...)
}

func NotErrorIs(t testify.TestingT, err, target error, msgAndArgs ...interface{}) bool {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	return testify.NotErrorIs(t, eris.Cause(err), eris.Cause(target), msgAndArgs...)
}

func IsError(t testify.TestingT, err error, msgAndArgs ...interface{}) bool {
	msgAndArgs = append([]interface{}{eris.ToString(err, true)}, msgAndArgs...)
	return testify.Error(t, err, msgAndArgs...)
}

func IsEqual(t testify.TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool {
	return testify.Equal(t, expected, actual, msgAndArgs...)
}

// the following below is covered by gotest and is duplicated by testify
// some signatures are slightly different but overall everything should be covered.

// func Equal(t TestingT, expected, actual interface{}, msgAndArgs ...interface{}) bool //renamed IsEqual above.
// func Error(t TestingT, err error, msgAndArgs ...interface{}) bool //renamed to IsError above.
// func ErrorContains(t TestingT, theError error, contains string, msgAndArgs ...interface{}) bool
// func ErrorIs(t TestingT, err, target error, msgAndArgs ...interface{}) bool {
// func ErrorAs(t TestingT, err error, target interface{}, msgAndArgs ...interface{}) bool {
