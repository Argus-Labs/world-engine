package cql

import (
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
)

type CQLOperator int

const (
	OpAnd CQLOperator = iota
	OpOr
)

var operatorMap = map[string]CQLOperator{"&": OpAnd, "|": OpOr}

func (o *CQLOperator) Capture(s []string) error {
	*o = operatorMap[s[0]]
	return nil
}

type CQLComponent struct {
	Name string `@Ident`
}

type CQLNot struct {
	SubExpression *CQLValue `"!" @@`
}

type CQLExact struct {
	Components []*CQLComponent `"EXACT""(" (@@",")* @@ ")"`
}

type CQLContains struct {
	Components []*CQLComponent `"CONTAINS" "(" (@@",")* @@ ")"`
}

type CQLValue struct {
	Exact         *CQLExact    `@@`
	Contains      *CQLContains `| @@`
	Not           *CQLNot      `| @@`
	Subexpression *CQLTerm     `| "(" @@ ")"`
}

type CQLFactor struct {
	Base *CQLValue `@@`
}

type CQLOpFactor struct {
	Operator CQLOperator `@("&" | "|")`
	Factor   *CQLFactor  `@@`
}

type CQLTerm struct {
	Left  *CQLFactor     `@@`
	Right []*CQLOpFactor `@@*`
}

// Display

func (o CQLOperator) String() string {
	switch o {
	case OpAnd:
		return "&"
	case OpOr:
		return "|"
	}
	panic("unsupported operator")
}

func (e *CQLExact) String() string {
	parameters := ""
	for i, comp := range e.Components {
		parameters += comp.Name + ", "
		if i < len(e.Components)-1 {
			parameters += ", "
		}
	}
	return "EXACT(" + parameters + ")"

}

func (e *CQLContains) String() string {
	parameters := ""
	for i, comp := range e.Components {
		parameters += comp.Name
		if i < len(e.Components)-1 {
			parameters += ", "
		}
	}
	return "CONTAINS(" + parameters + ")"
}

func (v *CQLValue) String() string {
	if v.Exact != nil {
		parameters := ""
		for _, comp := range v.Exact.Components {
			parameters += comp.Name + ", "
		}
		return "EXACT(" + parameters + ")"
	} else if v.Contains != nil {
		parameters := ""
		for _, comp := range v.Contains.Components {
			parameters += comp.Name + ", "
		}
		return "CONTAINS(" + parameters + ")"
	} else if v.Not != nil {
		return "!(" + v.Not.SubExpression.String() + ")"
	} else if v.Subexpression != nil {
		return "(" + v.Subexpression.String() + ")"
	} else {
		panic("blah")
	}
}

func (f *CQLFactor) String() string {
	out := f.Base.String()
	return out
}

func (o *CQLOpFactor) String() string {
	return fmt.Sprintf("%s %s", o.Operator, o.Factor)
}

func (t *CQLTerm) String() string {
	out := []string{t.Left.String()}
	for _, r := range t.Right {
		out = append(out, r.String())
	}
	return strings.Join(out, " ")
}

var CQLParser = participle.MustBuild[CQLTerm]()
