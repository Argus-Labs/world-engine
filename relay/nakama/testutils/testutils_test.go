package testutils_test

import (
	"context"
	"testing"

	"github.com/heroiclabs/nakama-common/runtime"

	"pkg.world.dev/world-engine/assert"
	"pkg.world.dev/world-engine/relay/nakama/testutils"
)

func TestObjectStoreComplainsAboutNonJSONEncodedValues(t *testing.T) {
	fakeNK := testutils.NewFakeNakamaModule()
	// Nakama's storage requires the value field is JSON encoded
	_, err := fakeNK.StorageWrite(context.Background(), []*runtime.StorageWrite{
		{
			Collection: "foo",
			Key:        "bar",
			UserID:     "123",
			Value:      "not-json",
		},
	})
	assert.IsError(t, err)
}
