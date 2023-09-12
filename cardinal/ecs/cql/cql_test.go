package cql

import (
	"reflect"
	"testing"

	"gotest.tools/v3/assert"

	"pkg.world.dev/world-engine/cardinal/ecs"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
)

func TestParser(t *testing.T) {
	term, err := internalCQLParser.ParseString("", "!(EXACT(a, b) & EXACT(a)) | CONTAINS(b)")
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

	emptyComponent := ecs.NewComponentType[struct{}]()
	stringToComponent := func(_ string) component.IComponentType {
		return emptyComponent
	}
	filterResult, err := termToLayoutFilter(term, stringToComponent)
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
	//have to do the below because of unexported fields in LayoutFilter datastructures. .
	assert.Assert(t, reflect.DeepEqual(filterResult, testResult))

}
