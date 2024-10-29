package world

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types"
)

// RegisterQuery registers a query with the query World.
// There can only be one query with a given name.
func (w *World) RegisterQuery(queryInput Query) error {
	_, ok := w.registeredQueriesByGroup[queryInput.Group()]
	if !ok {
		w.registeredQueriesByGroup[queryInput.Group()] = make(map[string]Query)
	}

	w.registeredQueriesByGroup[queryInput.Group()][queryInput.Name()] = queryInput
	return nil
}

// RegisteredQuries returns all the registered queries.
func (w *World) RegisteredQuries() []Query {
	registeredQueries := make([]Query, 0, len(w.registeredQueriesByGroup))
	for _, queryGroup := range w.registeredQueriesByGroup {
		for _, q := range queryGroup {
			registeredQueries = append(registeredQueries, q)
		}
	}
	return registeredQueries
}

func (w *World) HandleQuery(group string, name string, bz []byte) ([]byte, error) {
	q, err := w.getQuery(group, name)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to find query %s/%s", group, name)
	}
	return q.HandleQueryJSON(NewWorldContextReadOnly(w.State(), w.pm), bz)
}

func (w *World) HandleQueryEVM(group string, name string, abiRequest []byte) ([]byte, error) {
	q, err := w.getQuery(group, name)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to find EVM-compatible query %s/%s", group, name)
	}
	return q.HandleQueryEVM(NewWorldContextReadOnly(w.State(), w.pm), abiRequest)
}

// getQuery returns a query corresponding to the identifier with the format <group>/<name>.
func (w *World) getQuery(group string, name string) (Query, error) {
	q, ok := w.registeredQueriesByGroup[group][name]
	if !ok {
		return nil, types.ErrQueryNotFound
	}
	return q, nil
}
