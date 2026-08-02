package physics2d_test

import (
	"os"
	"testing"
)

// TestMain silences Cardinal logging for the whole package once, instead of per-test
// t.Setenv calls: t.Setenv panics in tests that also call t.Parallel, and every test in this
// package runs in parallel now that the pure-Go physics backend is per-instance state.
func TestMain(m *testing.M) {
	os.Setenv("LOG_LEVEL", "disabled")
	os.Exit(m.Run())
}
