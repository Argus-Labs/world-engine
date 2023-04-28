package ecs

import (
	"testing"

	"gotest.tools/assert"
)

type foo struct {
	f string
}

func TestCheck(t *testing.T) {
	foos := []foo{{"hi"}, {"hello"}, {"wow"}}

	testCases := []struct {
		name           string
		item           foo
		expectContains bool
	}{
		{
			name:           "should pass",
			item:           foo{"hi"},
			expectContains: true,
		},
		{
			name:           "should not pass",
			item:           foo{"nope"},
			expectContains: false,
		},
	}

	for _, tc := range testCases {
		has := Contains[foo](foos, tc.item, func(x, y foo) bool {
			return x.f == y.f
		})
		assert.Equal(t, has, tc.expectContains, "expected %v, got %v", tc.expectContains, has)
	}
}
