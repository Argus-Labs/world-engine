package main

import (
	"errors"
	"testing"

	"github.com/rotisserie/eris"
	"github.com/stretchr/testify/assert"
)

func TestFailureMessages(t *testing.T) {
	t.Parallel()

	// start.go joins independent failures, and kong joins that again: one ✖ each.
	cluster := eris.New("cluster start: port 8080 in use")
	build := eris.New("initial shard build: exit code: 1")
	assert.Equal(t, []string{cluster.Error(), build.Error()},
		failureMessages(errors.Join(errors.Join(cluster, build), nil)))

	// A join under a wrap is one step's causes: one message, a line per cause.
	deploy := eris.Wrap(errors.Join(eris.New("shard a: down"), eris.New("shard b: down")), "deploy")
	assert.Equal(t, []string{"deploy: shard a: down\nshard b: down"}, failureMessages(deploy))
}
