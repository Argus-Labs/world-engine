package cardinal

import (
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/cardinal/server/utils"
	"pkg.world.dev/world-engine/cardinal/types"
)

var _ QueryManager = &queryManager{}

type QueryManager interface {
	RegisterQuery(queryInput query) error
	GetRegisteredQueries() []query
	HandleQuery(group string, name string, bz []byte) ([]byte, error)
	HandleQueryEVM(group string, name string, abiRequest []byte) ([]byte, error)
	getQuery(group string, name string) (query, error)
	BuildQueryFields() []types.FieldDetail
}

type queryManager struct {
	world                    *World
	registeredQueriesByGroup map[string]map[string]query // group:name:query
}

func newQueryManager(world *World) QueryManager {
	return &queryManager{
		world:                    world,
		registeredQueriesByGroup: make(map[string]map[string]query),
	}
}

// RegisterQuery registers a query with the query manager.
// There can only be one query with a given name.
func (m *queryManager) RegisterQuery(queryInput query) error {
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

func (m *queryManager) HandleQuery(group string, name string, bz []byte) ([]byte, error) {
	q, err := m.getQuery(group, name)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to find query %s/%s", group, name)
	}
	return q.handleQueryJSON(NewReadOnlyWorldContext(m.world), bz)
}

func (m *queryManager) HandleQueryEVM(group string, name string, abiRequest []byte) ([]byte, error) {
	q, err := m.getQuery(group, name)
	if err != nil {
		return nil, eris.Wrapf(err, "unable to find EVM-compatible query %s/%s", group, name)
	}
	return q.handleQueryEVM(NewReadOnlyWorldContext(m.world), abiRequest)
}

// getQuery returns a query corresponding to the identifier with the format <group>/<name>.
func (m *queryManager) getQuery(group string, name string) (query, error) {
	query, ok := m.registeredQueriesByGroup[group][name]
	if !ok {
		return nil, types.ErrQueryNotFound
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
