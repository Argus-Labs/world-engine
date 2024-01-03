package statsd

import (
	"testing"

	"pkg.world.dev/world-engine/assert"
)

func TestMetricTagToTraceTag(t *testing.T) {
	testCases := []struct {
		tag       string
		wantKey   string
		wantValue any
	}{
		{
			tag:       "foo:bar",
			wantKey:   "foo",
			wantValue: "bar",
		},
		{
			tag:       "no_value",
			wantKey:   "no_value",
			wantValue: nil,
		},
		{
			tag:       "many:colons:in:this:tag",
			wantKey:   "many",
			wantValue: "colons:in:this:tag",
		},
		{
			tag:       "no_tag_value:",
			wantKey:   "no_tag_value",
			wantValue: nil,
		},
		{
			tag:       ":no_tag_key",
			wantKey:   "no_tag_key",
			wantValue: nil,
		},
	}

	for _, tc := range testCases {
		gotKey, gotValue := tagToTraceTag(tc.tag)
		assert.Equal(t, tc.wantKey, gotKey)
		assert.Equal(t, tc.wantValue, gotValue)
	}
}
