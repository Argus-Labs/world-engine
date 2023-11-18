package testutils

import (
	"gotest.tools/v3/assert"

	testify "github.com/stretchr/testify/assert"

	"github.com/rotisserie/eris"
)

func AssertNilErrorWithTrace(t assert.TestingT, err error, args ...interface{}) {
	args = append([]interface{}{eris.ToString(err, true)}, args...)
	assert.NilError(t, err, args...)
}

func AssertErrorWithTrace(t testify.TestingT, err error, args ...interface{}) {
	args = append([]interface{}{eris.ToString(err, true)}, args...)
	testify.Error(t, eris.Cause(err), args...)
}

func AssertErrorIsWithTrace(t assert.TestingT, err error, expected error, args ...interface{}) {
	args = append([]interface{}{eris.ToString(err, true)}, args...)
	assert.ErrorIs(t, eris.Cause(err), eris.Cause(expected), args...)
}
