package cql

import (
	"pkg.world.dev/world-engine/cardinal/filter"
	"pkg.world.dev/world-engine/cardinal/types"
	"reflect"
	"testing"

	"pkg.world.dev/world-engine/assert"
)

type EmptyComponent struct{}

func (EmptyComponent) Name() string { return "emptyComponent" }

func TestParser(t *testing.T) {
	term, err := internalCQLParser.ParseString("", "!(EXACT(a, b) & EXACT(a)) | CONTAINS(b)")
	testTerm := cqlTerm{
		Left: &cqlFactor{
			Base: &cqlValue{
				All:      nil,
				Exact:    nil,
				Contains: nil,
				Not: &cqlNot{
					SubExpression: &cqlValue{
						Exact:    nil,
						Contains: nil,
						Not:      nil,
						Subexpression: &cqlTerm{
							Left: &cqlFactor{
								Base: &cqlValue{
									Exact: &cqlExact{
										Components: []*cqlComponent{
											{Name: "a"},
											{Name: "b"},
										},
									},
									Contains:      nil,
									Not:           nil,
									Subexpression: nil,
								},
							},
							Right: []*cqlOpFactor{
								{
									Operator: opAnd,
									Factor: &cqlFactor{
										Base: &cqlValue{
											Exact:         &cqlExact{Components: []*cqlComponent{{Name: "a"}}},
											Contains:      nil,
											Not:           nil,
											Subexpression: nil,
										},
									},
								},
							},
						},
					},
				},
				Subexpression: nil,
			},
		},
		Right: []*cqlOpFactor{
			{
				Operator: opOr,
				Factor: &cqlFactor{
					Base: &cqlValue{
						Exact:         nil,
						Contains:      &cqlContains{Components: []*cqlComponent{{Name: "b"}}},
						Not:           nil,
						Subexpression: nil,
					},
				},
			},
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, *term, testTerm)

	emptyComponent := EmptyComponent{}
	assert.NilError(t, err)
	stringToComponent := func(_ string) (types.Component, error) {
		return emptyComponent, nil
	}
	filterResult, err := termToComponentFilter(term, stringToComponent)
	assert.NilError(t, err)
	testResult := filter.Or(
		filter.Not(
			filter.And(
				filter.Exact(emptyComponent, emptyComponent),
				filter.Exact(emptyComponent),
			),
		),
		filter.Contains(emptyComponent),
	)
	// have to do the below because of unexported fields in ComponentFilter datastructures.
	assert.Assert(t, reflect.DeepEqual(filterResult, testResult))
	query := "CONTAINS(A) & CONTAINS(A, B) & CONTAINS(A, B, C) | EXACT(D)"
	term, err = internalCQLParser.ParseString("", query)
	assert.NilError(t, err)
	result, err := termToComponentFilter(term, stringToComponent)
	assert.NilError(t, err)
	testResult2 :=
		filter.Or(
			filter.And(
				filter.And(
					filter.Contains(emptyComponent),
					filter.Contains(emptyComponent, emptyComponent),
				),
				filter.Contains(emptyComponent, emptyComponent, emptyComponent),
			),
			filter.Exact(emptyComponent),
		)
	assert.Assert(t, reflect.DeepEqual(testResult2, result))

	query = "ALL   (  )    "
	term, err = internalCQLParser.ParseString("", query)
	assert.NilError(t, err)
	result, err = termToComponentFilter(term, stringToComponent)
	assert.NilError(t, err)
	testResult2 = filter.All()
	assert.Assert(t, reflect.DeepEqual(result, testResult2))
}
