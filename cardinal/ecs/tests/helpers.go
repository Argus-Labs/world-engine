package tests

import (
	"context"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"
	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/storage"
)

const WorldId string = "1"

func getRedisStorage(t *testing.T) storage.RedisStorage {
	s := miniredis.RunT(t)
	return storage.NewRedisStorage(storage.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	}, WorldId)
}

// initWorldWithRedis sets up an ecs.World using the given redis DB. ecs.NewECSWorldForTest is not used
// because the test will re-use the incoming miniredis instance to initialize multiple worlds.
func initWorldWithRedis(t *testing.T, s *miniredis.Miniredis) *ecs.World {
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

// dumpRedis prints the contents of each key/value in the given miniredis instance.
// For list keys, each item is printed to a separate line.
func dumpRedis(t *testing.T, r *miniredis.Miniredis, label any) {
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
