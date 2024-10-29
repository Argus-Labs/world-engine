package testutils

import (
	"testing"
	"time"
)

func SetTestTimeout(t *testing.T, timeout time.Duration) {
	if _, ok := t.Deadline(); ok {
		// A deadline has already been set. Don't add an additional deadline.
		return
	}
	success := make(chan bool)
	t.Cleanup(func() {
		success <- true
	})
	go func() {
		select {
		case <-success:
			// test was successful. Do nothing
		case <-time.After(timeout):
			panic("test timed out")
		}
	}()
}
