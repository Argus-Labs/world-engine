package cardinal

import (
	"context"
	"fmt"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"gotest.tools/v3/assert"

	"github.com/argus-labs/cardinal/filter"
	"github.com/argus-labs/cardinal/storage"
)

type EnergyComponent struct {
	Amt int64
	Cap int64
}

type OwnableComponent struct {
	Owner string
}

func UpdateEnergySystem(w World) {
	Energy.Each(w, func(entry *storage.Entry) {
		energyPlanet, err := Energy.Get(entry)
		if err != nil {
			panic(err)
		}
		energyPlanet.Amt += 10 // bs whatever
		err = Energy.Set(entry, &energyPlanet)
		if err != nil {
			panic(err)
		}
	})
}

var (
	Energy  = NewComponentType[EnergyComponent]()
	Ownable = NewComponentType[OwnableComponent]()
)

func getRedisClient(t *testing.T) *redis.Client {
	s := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{
		Addr:     s.Addr(),
		Password: "", // no password set
		DB:       0,  // use default DB
	})
	return rdb
}

func Test_ECS(t *testing.T) {

	redisClient := getRedisClient(t)
	world := NewWorld(storage.NewRedisStorage(redisClient, "0"))
	//legacyStorage := storage.NewLegacyStorage()
	//world := NewWorld(legacyStorage)

	world.RegisterComponents(Energy, Ownable)

	// create a bunch of planets!
	numPlanets := 5
	_, err := world.CreateMany(numPlanets, Energy, Ownable)
	assert.NilError(t, err)

	//key := "COMPD:WORLD-0:CID-1:A-0"
	//res := redisClient.LRange(ctx, key, 0, -1)
	//assert.NilError(t, res.Err())
	//results, err := res.Result()
	//assert.NilError(t, err)
	//for _, r := range results {
	//	cd, err := decodeComponent[EnergyComponent]([]byte(r))
	//	assert.NilError(t, err)
	//	fmt.Printf("%+v", cd)
	//}

	numEnergyOnly := 10
	_, err = world.CreateMany(numEnergyOnly, Energy)
	assert.NilError(t, err)

	ctx := context.Background()
	scn := redisClient.Scan(ctx, 0, "", 100)
	assert.NilError(t, scn.Err())
	result, _, err := scn.Result()
	assert.NilError(t, err)
	for _, k := range result {
		fmt.Println(k)
	}

	world.AddSystem(UpdateEnergySystem)
	world.Update()

	Energy.Each(world, func(entry *storage.Entry) {
		energyPlanet, err := Energy.Get(entry)
		fmt.Printf("%+v\n", energyPlanet)
		assert.NilError(t, err)
		assert.Equal(t, int64(10), energyPlanet.Amt)
	})

	q := NewQuery(filter.Or(filter.Contains(Energy), filter.Contains(Ownable)))
	amt := q.Count(world)
	assert.Equal(t, numPlanets+numEnergyOnly, amt)
}
