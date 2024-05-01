package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/types/engine"
)

type QueryManager struct {
	registeredQueries map[string]engine.Query
}

func NewQueryManager() *QueryManager {
	return &QueryManager{
		registeredQueries: make(map[string]engine.Query),
	}
}

// RegisterQuery registers a query with the query manager.
// There can only be one query with a given name.
func (m *QueryManager) RegisterQuery(name string, query engine.Query) error {
	// Check that the query is not already registered
	if err := m.isQueryNameUnique(name); err != nil {
		return err
	}

	// Register the query
	m.registeredQueries[name] = query

	return nil
}

// GetRegisteredQueries returns all the registered queries.
func (m *QueryManager) GetRegisteredQueries() []engine.Query {
	registeredQueries := make([]engine.Query, 0, len(m.registeredQueries))
	for _, query := range m.registeredQueries {
		registeredQueries = append(registeredQueries, query)
	}
	return registeredQueries
}

// GetQueryByName returns a query corresponding to its name.
func (m *QueryManager) GetQueryByName(name string) (engine.Query, error) {
	query, ok := m.registeredQueries[name]
	if !ok {
		return nil, eris.Errorf("query %q is not registered", name)
	}
	return query, nil
}

func (m *QueryManager) isQueryNameUnique(name string) error {
	if _, ok := m.registeredQueries[name]; ok {
		return eris.Errorf("query %q is already registered", name)
	}
	return nil
}
