package cql

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestParser(t *testing.T) {
	term, err := CQLParser.ParseString("", "!(EXACT(a, b) & EXACT(a)) | CONTAINS(b)")
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
}
