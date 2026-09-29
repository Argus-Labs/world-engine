package spinner

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStart(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())
	defer cancel()

	session, err := Start(cancel, "Test spinner")
	require.NoError(t, err)
	require.NotNil(t, session)

	// Verify session implements the interface
	assert.Implements(t, (*Session)(nil), session)
}

func TestSession_Update(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())
	defer cancel()

	session, err := Start(cancel, "Initial text")
	require.NoError(t, err)

	// Test Update method doesn't panic
	assert.NotPanics(t, func() {
		session.Update("Updated text")
	})
}

func TestSession_Complete(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())
	defer cancel()

	session, err := Start(cancel, "Test spinner")
	require.NoError(t, err)

	// Test Complete method doesn't panic
	assert.NotPanics(t, func() {
		session.Complete()
	})
}

func TestSession_Quit(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())
	defer cancel()

	session, err := Start(cancel, "Test spinner")
	require.NoError(t, err)

	// Test Quit method doesn't panic
	assert.NotPanics(t, func() {
		session.Quit()
	})
}

func TestSession_NilSafety(t *testing.T) {
	var s *session

	// Test all methods handle nil gracefully
	assert.NotPanics(t, func() {
		s.Update("test")
	})
	assert.NotPanics(t, func() {
		s.Complete()
	})
	assert.NotPanics(t, func() {
		s.Quit()
	})
}

func TestSession_ContextCancellation(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())

	sess, err := Start(cancel, "Test spinner")
	require.NoError(t, err)

	// Cancel the context
	cancel()

	// Give some time for the cancellation to propagate
	time.Sleep(100 * time.Millisecond)

	// Session should still be usable
	assert.NotPanics(t, func() {
		sess.Update("Still working")
		sess.Complete()
	})
}

func TestStartWithEmptyText(t *testing.T) {
	_, cancel := context.WithCancel(t.Context())
	defer cancel()

	sess, err := Start(cancel, "")
	require.NoError(t, err)
	require.NotNil(t, sess)

	// Should handle empty text gracefully
	assert.NotPanics(t, func() {
		sess.Update("")
		sess.Complete()
	})
}

func TestStartMultipleSessions(t *testing.T) {
	_, cancel1 := context.WithCancel(t.Context())
	_, cancel2 := context.WithCancel(t.Context())
	defer cancel1()
	defer cancel2()

	// Start multiple sessions
	session1, err1 := Start(cancel1, "Session 1")
	require.NoError(t, err1)

	session2, err2 := Start(cancel2, "Session 2")
	require.NoError(t, err2)

	// Both should work independently
	assert.NotPanics(t, func() {
		session1.Update("Update 1")
		session2.Update("Update 2")
	})

	// Complete both
	assert.NotPanics(t, func() {
		session1.Complete()
		session2.Complete()
	})
}
