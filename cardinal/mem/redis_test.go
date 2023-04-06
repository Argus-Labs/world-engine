package mem

import (
	"context"
	"encoding"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"
)

var _ encoding.BinaryMarshaler = Foo{}

type Foo struct {
	X int `json:"X"`
	Y int `json:"Y"`
}

func (f Foo) MarshalBinary() (data []byte, err error) {
	return json.Marshal(f)
}

func TestRedis(t *testing.T) {
	ctx := context.Background()

	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	foo := &Foo{
		X: 35,
		Y: 40,
	}
	key := "foo"
	err := rdb.Set(ctx, key, foo, time.Duration(0)).Err()
	assert.NilError(t, err)

	cmd := rdb.Get(ctx, key)
	if err := cmd.Err(); err != nil {
		t.Fatal(err)
	}

	bz, err := cmd.Bytes()
	assert.NilError(t, err)

	var f Foo
	err = json.Unmarshal(bz, &f)
	assert.NilError(t, err)
	assert.Equal(t, f.X, foo.X)
	assert.Equal(t, f.Y, foo.Y)
}
