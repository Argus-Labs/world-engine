package cardinal

import (
	"github.com/rotisserie/eris"
)

type QueryManager interface {
	RegisterQuery(name string, query Query) error
	GetRegisteredQueries() []Query
	GetQueryByName(name string) (Query, error)
	BuildQueryIndex(world *World)
	GetQueryIndex() map[string]map[string]Query
}

type queryManager struct {
	registeredQueries map[string]Query
	queryIndex        map[string]map[string]Query
}

func newQueryManager() QueryManager {
	return &queryManager{
		registeredQueries: make(map[string]Query),
	}
}

func (m *queryManager) GetQueryIndex() map[string]map[string]Query {
	return m.queryIndex
}

// RegisterQuery registers a query with the query manager.
// There can only be one query with a given name.
func (m *queryManager) RegisterQuery(name string, query Query) error {
	// Check that the query is not already registered
	if err := m.isQueryNameUnique(name); err != nil {
		return err
	}

	// Register the query
	m.registeredQueries[name] = query
	return nil
}

// GetRegisteredQueries returns all the registered queries.
func (m *queryManager) GetRegisteredQueries() []Query {
	registeredQueries := make([]Query, 0, len(m.registeredQueries))
	for _, query := range m.registeredQueries {
		registeredQueries = append(registeredQueries, query)
	}
	return registeredQueries
}

func (m *queryManager) BuildQueryIndex(world *World) {
	queries := world.GetRegisteredQueries()
	m.queryIndex = make(map[string]map[string]Query)
	// Create query index
	for _, query := range queries {
		// Initialize inner map if it doesn't exist
		if _, ok := m.queryIndex[query.Group()]; !ok {
			m.queryIndex[query.Group()] = make(map[string]Query)
		}
		m.queryIndex[query.Group()][query.Name()] = query
	}
}
func (w *World) QueryHandler(name string, group string, bz []byte) ([]byte, error) {
	index := w.QueryManager.GetQueryIndex()
	groupIndex, ok := index[group]

	if !ok {
		return nil, eris.Errorf("query with group %s not found", group)
	}
	query, ok := groupIndex[name]
	if !ok {
		return nil, eris.Errorf("query with name %s not found", name)
	}
	wCtx := NewReadOnlyWorldContext(w)
	return query.handleQueryRaw(wCtx, bz)
}

// GetQueryByName returns a query corresponding to its name.
func (m *queryManager) GetQueryByName(name string) (Query, error) {
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
