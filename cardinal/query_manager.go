package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type QueryManager interface {
	RegisterQuery(name string, query query) error
	GetRegisteredQueries() []query
	GetQueryByName(name string) (query, error)
	BuildQueryFields() []engine.FieldDetail
}

type queryManager struct {
	registeredQueries map[string]query
}

func newQueryManager() QueryManager {
	return &queryManager{
		registeredQueries: make(map[string]query),
	}
}

// RegisterQuery registers a query with the query manager.
// There can only be one query with a given name.
func (m *queryManager) RegisterQuery(name string, query query) error {
	// Check that the query is not already registered
	if err := m.isQueryNameUnique(name); err != nil {
		return err
	}

	// Register the query
	m.registeredQueries[name] = query
	return nil
}

// GetRegisteredQueries returns all the registered queries.
func (m *queryManager) GetRegisteredQueries() []query {
	registeredQueries := make([]query, 0, len(m.registeredQueries))
	for _, query := range m.registeredQueries {
		registeredQueries = append(registeredQueries, query)
	}
	return registeredQueries
}

func (w *World) QueryHandler(name string, bz []byte) ([]byte, error) {
	q, err := w.GetQueryByName(name)
	if err != nil {
		return nil, eris.Wrap(engine.NewQueryNotFoundError(name), "")
	}
	wCtx := NewReadOnlyWorldContext(w)
	return q.handleQueryRaw(wCtx, bz)
}

// GetQueryByName returns a query corresponding to its name.
func (m *queryManager) GetQueryByName(name string) (query, error) {
	query, ok := m.registeredQueries[name]
	if !ok {
		return nil, eris.Errorf("query %q is not registered", name)
	}
	return query, nil
}

func (m *queryManager) isQueryNameUnique(name string) error {
	if _, ok := m.registeredQueries[name]; ok {
		return eris.Errorf("query %q is already registered", name)
	}
	return nil
}

func (m *queryManager) BuildQueryFields() []engine.FieldDetail {
	// Collecting the structure of all queries
	queries := m.GetRegisteredQueries()
	queriesFields := make([]engine.FieldDetail, 0, len(queries))
	for _, q := range queries {
		// Extracting the fields of the q
		queriesFields = append(queriesFields, engine.FieldDetail{
			Name:   q.Name(),
			Fields: q.GetRequestFieldInformation(),
			URL:    utils.GetQueryURL(q.Group(), q.Name()),
		})
	}
	return queriesFields
}
