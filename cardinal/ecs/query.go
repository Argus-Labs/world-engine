package ecs

/*
import (
	"github.com/argus-labs/world-engine/cardinal/ecs/entity"
	"github.com/argus-labs/world-engine/cardinal/ecs/filter"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
	types "github.com/argus-labs/world-engine/cardinal/ecs/storage/types/v1"
)

type cache struct {
	archetypes []storage.ArchetypeIndex
	seen       int
}

// Query represents a query for entities and components.
// It is used to filter entities based on their components.
// It receives arbitrary filters that are used to filter entities.
// It contains a cache that is used to avoid re-evaluating the query.
// So it is not recommended to create a new query every time you want
// to filter entities with the same query.
type Query struct {
	layoutMatches map[WorldId]*cache
	filter        filter.LayoutFilter
}

// NewQuery creates a new query.
// It receives arbitrary filters that are used to filter entities.
func NewQuery(filter filter.LayoutFilter) *Query {
	return &Query{
		layoutMatches: make(map[WorldId]*cache),
		filter:        filter,
	}
}

// Each iterates over all entities that match the query.
func (q *Query) Each(w World, callback func(*types.Entry)) {
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
	f := func(entity entity.Entity) {
		entry, _ := w.Entry(entity)
		callback(entry)
	}
	for iter.HasNext() {
		entities := iter.Next()
		for _, e := range entities {
			f(e)
		}
	}
}

// Count returns the number of entities that match the query.
func (q *Query) Count(w World) int {
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
	ret := 0
	for iter.HasNext() {
		entities := iter.Next()
		ret += len(entities)
	}
	return ret
}

// First returns the first entity that matches the query.
func (q *Query) First(w World) (entry *types.Entry, ok bool, err error) {
	accessor := w.StorageAccessor()
	result := q.evaluateQuery(w, &accessor)
	iter := storage.NewEntityIterator(0, accessor.Archetypes, result)
	if !iter.HasNext() {
		return nil, false, err
	}
	for iter.HasNext() {
		entities := iter.Next()
		if len(entities) > 0 {
			ent, err := w.Entry(storage.Entity(entities[0]))
			if err != nil {
				return nil, false, err
			}
			return ent, true, err
		}
	}
	return nil, false, err
}

func (q *Query) evaluateQuery(world World, accessor *StorageAccessor) []storage.ArchetypeIndex {
	w := world.ID()
	if _, ok := q.layoutMatches[w]; !ok {
		q.layoutMatches[w] = &cache{
			archetypes: make([]storage.ArchetypeIndex, 0),
			seen:       0,
		}
	}
	cache := q.layoutMatches[w]
	for it := accessor.Index.SearchFrom(q.filter, cache.seen); it.HasNext(); {
		cache.archetypes = append(cache.archetypes, it.Next())
	}
	// TODO(technicallyty): deal with this
	// cache.seen = accessor.Archetypes.Count()
	return cache.archetypes
}
*/
