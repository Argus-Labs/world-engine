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

func TestRandomVersionShouldFail(t *testing.T) {
	ctx := context.Background()
	fakeNK := testutils.NewFakeNakamaModule()
	write := &runtime.StorageWrite{
		Collection: "foo",
		Key:        "bar",
		UserID:     "123",
		Value:      "{}",
		Version:    "RANDOM_VERSION",
	}
	// "RANDOM_VERSION" is not the version in storage (because there is no version), so this write should fail
	_, err := fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{write})
	assert.IsError(t, err)
}

// TestConditionalWrites ensures writes are only successful if the given version matches.
// See https://heroiclabs.com/docs/nakama/concepts/storage/collections/#conditional-writes
func TestConditionalWrite(t *testing.T) {
	ctx := context.Background()
	fakeNK := testutils.NewFakeNakamaModule()
	write := &runtime.StorageWrite{
		Collection: "foo",
		Key:        "bar",
		UserID:     "123",
		Value:      "{}",
		Version:    "",
	}
	// The first write (with no version) should be successful
	_, err := fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{write})
	assert.NilError(t, err)

	// Setting a random version should fail
	write.Version = "some-missing-version"
	_, err = fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{write})
	assert.IsError(t, err)

	// But performing a read first, then using the returned version should succeed
	objs, err := fakeNK.StorageRead(ctx, []*runtime.StorageRead{
		{Collection: "foo", Key: "bar", UserID: "123"},
	})
	assert.NilError(t, err)
	assert.Equal(t, len(objs), 1)
	write.Version = objs[0].GetVersion()

	_, err = fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{write})
	assert.NilError(t, err)

	// Finally, writing to some other collection/key/userid without a version should be possible
	_, err = fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{{
		Collection: "some-other-collection",
		Key:        "some-other-key",
		UserID:     "some-other-userid",
		Value:      "{}",
	}})
	assert.NilError(t, err)
}

func TestConditionalWriteIfNotExists(t *testing.T) {
	ctx := context.Background()
	fakeNK := testutils.NewFakeNakamaModule()
	write := &runtime.StorageWrite{
		Collection: "foo",
		Key:        "bar",
		UserID:     "123",
		Value:      "{}",
		Version:    "*",
	}

	// Version is "*", and it's not in the DB, so this should succeed
	_, err := fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{write})
	assert.NilError(t, err)

	// Version is "*", and it's already in the DB. This should fail.
	_, err = fakeNK.StorageWrite(ctx, []*runtime.StorageWrite{write})
	assert.IsError(t, err)
}
