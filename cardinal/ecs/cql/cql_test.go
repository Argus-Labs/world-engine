package cql

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestParser(t *testing.T) {
	term, err := CQLParser.ParseString("", "!(EXACT(a, b) & EXACT(a)) | CONTAINS(b)")
	testTerm := CQLTerm{
		Left: &CQLFactor{Base: &CQLValue{
			Exact:    nil,
			Contains: nil,
			Not: &CQLNot{SubExpression: &CQLValue{
				Exact:    nil,
				Contains: nil,
				Not:      nil,
				Subexpression: &CQLTerm{
					Left: &CQLFactor{Base: &CQLValue{
						Exact: &CQLExact{Components: []*CQLComponent{
							&CQLComponent{Name: "a"},
							&CQLComponent{Name: "b"}}},
						Contains:      nil,
						Not:           nil,
						Subexpression: nil,
					}},
					Right: []*CQLOpFactor{&CQLOpFactor{
						Operator: OpAnd,
						Factor: &CQLFactor{Base: &CQLValue{
							Exact:         &CQLExact{Components: []*CQLComponent{&CQLComponent{Name: "a"}}},
							Contains:      nil,
							Not:           nil,
							Subexpression: nil,
						}},
					}},
				},
			}},
			Subexpression: nil,
		}},
		Right: []*CQLOpFactor{
			&CQLOpFactor{
				Operator: OpOr,
				Factor: &CQLFactor{Base: &CQLValue{
					Exact:         nil,
					Contains:      &CQLContains{Components: []*CQLComponent{&CQLComponent{Name: "b"}}},
					Not:           nil,
					Subexpression: nil,
				}},
			},
		},
	}
	assert.NilError(t, err)
	assert.DeepEqual(t, *term, testTerm)
}
