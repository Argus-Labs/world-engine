package testutil

import (
	"context"
	"crypto/ecdsa"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
	"pkg.world.dev/world-engine/sign"
)

const WorldId string = "1"

func GetRedisStorage(t *testing.T) storage.RedisStorage {
	s := miniredis.RunT(t)
	return storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, WorldId)
}

// InitWorldWithRedis sets up an ecs.World using the given redis DB. ecs.NewECSWorldForTest is not used
// because the test will re-use the incoming miniredis instance to initialize multiple worlds.
func InitWorldWithRedis(t *testing.T, s *miniredis.Miniredis) *ecs.World {
	rs := storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, "in-memory-world")
	worldStorage := storage.NewWorldStorage(&rs)
	w, err := ecs.NewWorld(worldStorage)
	assert.NilError(t, err)
	return w
}

// DumpRedis prints the contents of each key/value in the given miniredis instance.
// For list keys, each item is printed to a separate line.
func DumpRedis(t *testing.T, r *miniredis.Miniredis, label any) {
	t.Log("*************************************************")
	t.Logf("* starting redis dump: %v", label)
	t.Log("*************************************************")

	client := redis.NewClient(&redis.Options{
		Addr:     r.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	ctx := context.Background()

	keys, err := client.Keys(ctx, "*").Result()
	assert.NilError(t, err)
	for _, key := range keys {
		t.Log(key)
		str, err := client.Get(ctx, key).Result()
		if err == nil {
			t.Log("  ", str)
		} else if strings.Contains(err.Error(), "WRONGTYPE") {
			// This is a list. Dump each item in the list
			count, err := client.LLen(ctx, key).Result()
			assert.NilError(t, err)
			for i := int64(0); i < count; i++ {
				str, err := client.LIndex(ctx, key, i).Result()
				assert.NilError(t, err)
				t.Logf("  item:%d: %v", i, str)
			}

		} else if err != nil {
			assert.NilError(t, err)
		}
	}
}

var (
	nonce      uint64
	privateKey *ecdsa.PrivateKey
)

func init() {
	var err error
	privateKey, err = crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
}

func UniqueSignature(t *testing.T) *sign.SignedPayload {
	nonce++
	sig, err := sign.NewSignedPayload(privateKey, "some-persona-tag", "namespace", nonce, "data")
	assert.NilError(t, err)
	return sig
}
