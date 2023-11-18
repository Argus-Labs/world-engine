package testutils

import (
	"gotest.tools/v3/assert"

	"github.com/rotisserie/eris"
)

func AssertNilErrorWithTrace(t assert.TestingT, err error, args ...interface{}) {
	args = append([]interface{}{eris.ToString(err, true)}, args...)
	assert.NilError(t, err, args...)
}
