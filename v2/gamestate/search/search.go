package search

import (
	"math"
	"slices"

	"github.com/rotisserie/eris"
	"github.com/rs/zerolog/log"

	"pkg.world.dev/world-engine/cardinal/v2/gamestate"
	"pkg.world.dev/world-engine/cardinal/v2/gamestate/search/filter"
	"pkg.world.dev/world-engine/cardinal/v2/types"
)

var nonFatalError = []error{
	gamestate.ErrEntityDoesNotExist,
	gamestate.ErrComponentNotOnEntity,
	gamestate.ErrComponentAlreadyOnEntity,
	gamestate.ErrEntityMustHaveAtLeastOneComponent,
}

const badEntityID types.EntityID = math.MaxUint64

type CallbackFn func(types.EntityID) bool

// Search allows you to find a set of entities that match a given criteria. Entities are first filtered by their
// components defined in componentFilter, and then filtered by an arbitrary user-defined filter defined in whereFilter.
type Search struct {
	stateReader gamestate.Reader

	// componentFilter defines our entitity component criteria.
	componentFilter filter.ComponentFilter

	// whereFilter is an arbitrary user-defined filter that can be evaluated to filter entities.
	whereFilter FilterFn
}

// New allows users to create a Search object with a filter already provided
// as a property.
func New(stateReader gamestate.Reader, compFilter filter.ComponentFilter) *Search {
	return &Search{
		stateReader:     stateReader,
		componentFilter: compFilter,
		whereFilter:     nil,
	}
}

// Where Once the where clause method is activated the search will ONLY return results
// if a where clause returns true and no error.
func (s *Search) Where(whereFn func(id types.EntityID) (bool, error)) *Search {
	var whereFilter FilterFn

	// A where clause can be chained with another where clause. If the where clause is not nil, we need to chain it.
	if s.whereFilter != nil {
		whereFilter = andFilter(s.whereFilter, whereFn)
	} else {
		whereFilter = whereFn
	}

	return &Search{
		stateReader:     s.stateReader,
		componentFilter: s.componentFilter,
		whereFilter:     whereFilter,
	}
}

// Each iterates over all entities that match the search.
// If you would like to stop the iteration, return false to the callback. To continue iterating, return true.
func (s *Search) Each(callback CallbackFn) (err error) {
	defer panicOnFatalError(err)

	archetypes, err := s.findArchetypes()
	if err != nil {
		return err
	}

	entities := gamestate.NewEntityIterator(s.stateReader, archetypes)

	for entities.HasNext() {
		entities, err := entities.Next()
		if err != nil {
			return err
		}

		for _, id := range entities {
			// Entity is eligible until proven otherwise
			entityEligible := true

			if s.whereFilter != nil {
				entityEligible, err = s.whereFilter(id)
				if err != nil {
					continue
				}
			}

			if entityEligible {
				if cont := callback(id); !cont {
					return nil
				}
			}
		}
	}

	return nil
}

// First returns the first entity that matches the search.
func (s *Search) First() (id types.EntityID, err error) {
	defer panicOnFatalError(err)

	archetypes, err := s.findArchetypes()
	if err != nil {
		return badEntityID, err
	}

	entities := gamestate.NewEntityIterator(s.stateReader, archetypes)
	if !entities.HasNext() {
		return badEntityID, eris.New("no entities for the given criteria found")
	}

	for entities.HasNext() {
		entities, err := entities.Next()
		if err != nil {
			return 0, err
		}

		for _, id := range entities {
			// Entity is eligible until proven otherwise
			entityEligible := true

			if s.whereFilter != nil {
				entityEligible, err = s.whereFilter(id)
				if err != nil {
					continue
				}
			}

			if entityEligible {
				return id, nil
			}
		}
	}

	return badEntityID, nil
}

func (s *Search) MustFirst() types.EntityID {
	id, err := s.First()
	if err != nil {
		panic("no entity matches the search")
	}
	return id
}

// Count returns the number of entities that match the search.
func (s *Search) Count() (ret int, err error) {
	defer panicOnFatalError(err)

	archetypes, err := s.findArchetypes()
	if err != nil {
		return 0, err
	}

	entities := gamestate.NewEntityIterator(s.stateReader, archetypes)
	for entities.HasNext() {
		entities, err := entities.Next()
		if err != nil {
			return 0, err
		}

		for _, id := range entities {
			// Entity is eligible until proven otherwise
			entityEligible := true

			if s.whereFilter != nil {
				entityEligible, err = s.whereFilter(id)
				if err != nil {
					continue
				}
			}

			if entityEligible {
				ret++
			}
		}
	}
	return ret, nil
}

func (s *Search) Collect() ([]types.EntityID, error) {
	acc := make([]types.EntityID, 0)

	err := s.Each(func(id types.EntityID) bool {
		acc = append(acc, id)
		return true
	})
	if err != nil {
		return nil, err
	}

	fastSortIDs(acc)
	return acc, nil
}

func (s *Search) findArchetypes() ([]types.ArchetypeID, error) {
	return s.stateReader.FindArchetypes(s.componentFilter)
}

func fastSortIDs(ids []types.EntityID) {
	slices.Sort(ids)
}

// panicOnFatalError is a helper function to panic on non-deterministic errors (i.e. Redis error).
func panicOnFatalError(err error) {
	if err != nil && isFatalError(err) {
		log.Logger.Panic().Err(err).Msgf("fatal error: %v", eris.ToString(err, true))
		panic(err)
	}
}

func isFatalError(err error) bool {
	for _, e := range nonFatalError {
		if eris.Is(err, e) {
			return false
		}
	}
	return true
}
