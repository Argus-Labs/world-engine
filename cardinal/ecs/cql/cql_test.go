package cql

import (
	"fmt"
	"reflect"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

type EmptyComponent struct{}

func (EmptyComponent) Name() string { return "emptyComponent" }

func TestParser(t *testing.T) {
	term, err := internalCQLParser.ParseString("", "!(EXACT(a, b) & EXACT(a)) | CONTAINS(b)")
	fmt.Println(term.String())
	testTerm := cqlTerm{
		Left: &cqlFactor{Base: &cqlValue{
			Exact:    nil,
			Contains: nil,
			Not: &cqlNot{SubExpression: &cqlValue{
				Exact:    nil,
				Contains: nil,
				Not:      nil,
				Subexpression: &cqlTerm{
					Left: &cqlFactor{Base: &cqlValue{
						Exact: &cqlExact{Components: []*cqlComponent{
							&cqlComponent{Name: "a"},
							&cqlComponent{Name: "b"}}},
						Contains:      nil,
						Not:           nil,
						Subexpression: nil,
					}},
					Right: []*cqlOpFactor{&cqlOpFactor{
						Operator: opAnd,
						Factor: &cqlFactor{Base: &cqlValue{
							Exact:         &cqlExact{Components: []*cqlComponent{&cqlComponent{Name: "a"}}},
							Contains:      nil,
							Not:           nil,
							Subexpression: nil,
						}},
					}},
				},
			}},
			Subexpression: nil,
		}},
		Right: []*cqlOpFactor{
			&cqlOpFactor{
				Operator: opOr,
				Factor: &cqlFactor{Base: &cqlValue{
					Exact:         nil,
					Contains:      &cqlContains{Components: []*cqlComponent{&cqlComponent{Name: "b"}}},
					Not:           nil,
					Subexpression: nil,
				}},
			},
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, *term, testTerm)

	emptyComponent := ecs.NewComponentType[EmptyComponent]()
	stringToComponent := func(_ string) (component.IComponentType, error) {
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
	//have to do the below because of unexported fields in ComponentFilter datastructures. .
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
					filter.Contains(emptyComponent, emptyComponent)),
				filter.Contains(emptyComponent, emptyComponent, emptyComponent)),
			filter.Exact(emptyComponent),
		)
	assert.Assert(t, reflect.DeepEqual(testResult2, result))

}
