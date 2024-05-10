package cardinal

import (
	"github.com/rotisserie/eris"
)

type QueryManager interface {
	RegisterQuery(name string, query query) error
	GetRegisteredQueries() []query
	GetQueryByName(name string) (query, error)
	BuildQueryIndex(world *World)
	GetQueryIndex() map[string]map[string]query
}

type queryManager struct {
	registeredQueries map[string]query
	queryIndex        map[string]map[string]query
}

func newQueryManager() QueryManager {
	return &queryManager{
		registeredQueries: make(map[string]query),
	}
}

func (m *queryManager) GetQueryIndex() map[string]map[string]query {
	return m.queryIndex
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

func (m *queryManager) BuildQueryIndex(world *World) {
	queries := world.GetRegisteredQueries()
	m.queryIndex = make(map[string]map[string]query)
	// Create query index
	for _, q := range queries {
		// Initialize inner map if it doesn't exist
		if _, ok := m.queryIndex[q.Group()]; !ok {
			m.queryIndex[q.Group()] = make(map[string]query)
		}
		m.queryIndex[q.Group()][q.Name()] = q
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
