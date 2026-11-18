//go:build race

package immutable_test

// raceEnabled reports a -race build. The race detector changes escape analysis, so values that
// stay on the stack in a normal build are heap-allocated, and allocation counts stop meaning
// anything.
const raceEnabled = true
