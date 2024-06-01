package cardinal

import (
	"fmt"

	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
)

type QueryManager interface {
	RegisterQuery(queryInput query) error
	GetRegisteredQueries() []query
	GetQuery(group string, name string) (query, error)
	BuildQueryFields() []types.FieldDetail
}

type queryManager struct {
	registeredQueriesByGroup map[string]map[string]query // group:name:query
}

func newQueryManager() QueryManager {
	return &queryManager{
		registeredQueriesByGroup: make(map[string]map[string]query),
	}
}

// RegisterQuery registers a query with the query manager.
// There can only be one query with a given name.
func (m *queryManager) RegisterQuery(queryInput query) error {
	// Register the query
	_, ok := m.registeredQueriesByGroup[queryInput.Group()]
	if !ok {
		m.registeredQueriesByGroup[queryInput.Group()] = make(map[string]query)
	}

	m.registeredQueriesByGroup[queryInput.Group()][queryInput.Name()] = queryInput
	return nil
}

// GetRegisteredQueries returns all the registered queries.
func (m *queryManager) GetRegisteredQueries() []query {
	registeredQueries := make([]query, 0, len(m.registeredQueriesByGroup))
	for _, queryGroup := range m.registeredQueriesByGroup {
		for _, query := range queryGroup {
			registeredQueries = append(registeredQueries, query)
		}
	}
	return registeredQueries
}

func (w *World) HandleQuery(group string, name string, bz []byte) ([]byte, error) {
	q, err := w.GetQuery(group, name)
	if err != nil {
		return nil, eris.Wrap(types.ErrQueryNotFound, fmt.Sprintf("could not find query %q", name))
	}
	if q.Group() != group {
		return nil, eris.Errorf("Query group: %s with name: %s not found", group, name)
	}
	wCtx := NewReadOnlyWorldContext(w)
	return q.handleQueryRaw(wCtx, bz)
}

// GetQueryByName returns a query corresponding to its name.
func (m *queryManager) GetQuery(group string, name string) (query, error) {
	query, ok := m.registeredQueriesByGroup[group][name]
	if !ok {
		return nil, eris.Errorf("query %q is not registered under group %s", name, DefaultQueryGroup)
	}
	return query, nil
}

func (m *queryManager) BuildQueryFields() []types.FieldDetail {
	// Collecting the structure of all queries
	queries := m.GetRegisteredQueries()
	queriesFields := make([]types.FieldDetail, 0, len(queries))
	for _, q := range queries {
		// Extracting the fields of the q
		queriesFields = append(queriesFields, types.FieldDetail{
			Name:   q.Name(),
			Fields: q.GetRequestFieldInformation(),
			URL:    utils.GetQueryURL(q.Group(), q.Name()),
		})
	}
	return queriesFields
}
