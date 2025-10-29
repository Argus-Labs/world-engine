package service

import (
	"fmt"
	"sync"
	"testing"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/micro"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"google.golang.org/protobuf/types/known/anypb"
)

// Test components.
type PlayerTag struct{ Nickname string }

func (PlayerTag) Name() string { return "PlayerTag" }

type Health struct{ HP int }

func (Health) Name() string { return "Health" }

type Position struct{ X, Y int }

func (Position) Name() string { return "Position" }

type Score struct{ Value int }

func (Score) Name() string { return "Score" }

type Inventory struct{ Items []string }

func (Inventory) Name() string { return "Inventory" }

type InitSystemState struct {
	ecs.BaseSystemState
	Query ecs.Contains[struct {
		Tag       ecs.Ref[PlayerTag]
		Health    ecs.Ref[Health]
		Position  ecs.Ref[Position]
		Score     ecs.Ref[Score]
		Inventory ecs.Ref[Inventory]
	}]
}

func initSystem(state *InitSystemState) error {
	// Create 100 entities with PlayerTag + Health with varying HP values
	for i := range 100 {
		_, entity := state.Query.Create()
		entity.Tag.Set(PlayerTag{Nickname: fmt.Sprintf("player%d", i)})
		entity.Health.Set(Health{HP: 100 + (i * 5)}) // HP ranges from 100 to 595
	}

	// Create 50 entities with PlayerTag + Position in different quadrants
	for i := range 50 {
		x := (i % 5) * 20 // X: 0, 20, 40, 60, 80 repeating
		y := (i / 5) * 20 // Y: increases by 20 every 5 entities
		_, entity := state.Query.Create()
		entity.Tag.Set(PlayerTag{Nickname: fmt.Sprintf("scout%d", i)})
		entity.Position.Set(Position{X: x, Y: y})
	}

	// Create 50 entities with all components (for complex queries)
	for i := range 50 {
		hp := 200 + (i * 10)     // HP ranges from 200 to 690
		x := -100 + (i * 5)      // X ranges from -100 to 145
		y := i * 5               // Y ranges from 0 to 245
		score := 1000 + (i * 50) // Score ranges from 1000 to 3450

		items := []string{"potion"}
		if i%3 == 0 {
			items = append(items, "sword")
		}
		if i%4 == 0 {
			items = append(items, "bow", "arrows")
		}
		if i%5 == 0 {
			items = append(items, "shield")
		}

		_, entity := state.Query.Create()
		entity.Tag.Set(PlayerTag{Nickname: fmt.Sprintf("hero%d", i)})
		entity.Health.Set(Health{HP: hp})
		entity.Position.Set(Position{X: x, Y: y})
		entity.Score.Set(Score{Value: score})
		entity.Inventory.Set(Inventory{Items: items})
	}

	// Create 50 Position-only entities in a grid pattern
	for i := range 50 {
		x := ((i % 7) * 30) - 90 // X: -90 to 90 in steps of 30
		y := ((i / 7) * 30) - 90 // Y: -90 to 90 in steps of 30
		_, entity := state.Query.Create()
		entity.Position.Set(Position{X: x, Y: y})
	}

	// Create 25 Health-only entities with varying HP
	for i := range 25 {
		_, entity := state.Query.Create()
		entity.Health.Set(Health{HP: 50 + (i * 20)}) // HP ranges from 50 to 530
	}

	// Create 25 Score-only entities
	for i := range 25 {
		_, entity := state.Query.Create()
		entity.Score.Set(Score{Value: 500 + (i * 100)}) // Scores from 500 to 2900
	}

	return nil
}

func BenchmarkQuery(b *testing.B) {
	// Setup test cases
	benchmarks := []struct {
		name  string
		find  []string
		match ecs.SearchMatch
		where string
	}{
		{
			name:  "simple position query",
			find:  []string{"Position"},
			match: ecs.MatchExact,
		},
		{
			name:  "multi-component query",
			find:  []string{"Position", "Health"},
			match: ecs.MatchContains,
		},
		{
			name:  "query with filter",
			find:  []string{"Position", "Health"},
			match: ecs.MatchContains,
			where: "Health.HP > 150",
		},
		{
			name:  "complex filter",
			find:  []string{"Position", "Health", "PlayerTag"},
			match: ecs.MatchContains,
			where: "Health.HP > 200 && Position.X > Position.Y",
		},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			world := ecs.NewWorld()
			ecs.RegisterSystem(world, initSystem, ecs.WithHook(ecs.Init))

			world.InitSchedulers()

			err := world.InitSystems()
			if err != nil {
				b.Fatal("failed to initialize world:", err)
			}

			pool := sync.Pool{
				New: func() any {
					return &Query{
						Find: make([]string, 0, 8),
					}
				},
			}

			req := createTestRequest(b, bm.find, bm.match, bm.where)

			b.ResetTimer()
			for b.Loop() {
				// Parse query.
				query, err := parseQuery(&pool, req)
				if err != nil {
					b.Fatal("parse query failed:", err)
				}

				// Search.
				results, err := world.NewSearch(ecs.SearchParam{
					Find:  query.Find,
					Match: query.Match,
					Where: query.Where,
				})
				if err != nil {
					b.Fatal("search failed:", err)
				}

				// Serialize results.
				_, err = serializeQueryResults(results)
				if err != nil {
					b.Fatal("serialize failed:", err)
				}

				// Return the query object.
				pool.Put(query)
			}
		})
	}
}

func createTestRequest(t testing.TB, find []string, match ecs.SearchMatch, where string) *micro.Request {
	iscMsg := &iscv1.Query{
		Find:  find,
		Match: searchMatchToISCQueryMatch(match),
		Where: where,
	}

	// Pack the ISC Message into Any
	anyMsg, err := anypb.New(iscMsg)
	if err != nil {
		t.Fatal("failed to pack message into Any:", err)
	}

	// Create the final Request using micro.Request
	return &micro.Request{
		ServiceAddress: &microv1.ServiceAddress{},
		Payload:        anyMsg,
	}
}

func searchMatchToISCQueryMatch(m ecs.SearchMatch) iscv1.Query_Match {
	switch m {
	case ecs.MatchExact:
		return iscv1.Query_MATCH_EXACT
	case ecs.MatchContains:
		return iscv1.Query_MATCH_CONTAINS
	default:
		return iscv1.Query_MATCH_UNSPECIFIED
	}
}
