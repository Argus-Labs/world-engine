package cql

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
)

type cqlOperator int

const (
	opAnd cqlOperator = iota
	opOr
)

var operatorMap = map[string]cqlOperator{"&": opAnd, "|": opOr}

// Capture basically tells the parser library how to transform a string token that's parsed into the operator type.
func (o *cqlOperator) Capture(s []string) error {
	if len(s) <= 0 {
		return errors.New("invalid operator")
	}
	operator, ok := operatorMap[s[0]]
	if !ok {
		return errors.New("invalid operator")
	}
	*o = operator
	return nil
}

type cqlComponent struct {
	Name string `@Ident`
}

type cqlNot struct {
	SubExpression *cqlValue `"!" @@`
}

type cqlExact struct {
	Components []*cqlComponent `"EXACT""(" (@@",")* @@ ")"`
}

type cqlContains struct {
	Components []*cqlComponent `"CONTAINS" "(" (@@",")* @@ ")"`
}

type cqlValue struct {
	Exact         *cqlExact    `@@`
	Contains      *cqlContains `| @@`
	Not           *cqlNot      `| @@`
	Subexpression *cqlTerm     `| "(" @@ ")"`
}

type cqlFactor struct {
	Base *cqlValue `@@`
}

type cqlOpFactor struct {
	Operator cqlOperator `@("&" | "|")`
	Factor   *cqlFactor  `@@`
}

type cqlTerm struct {
	Left  *cqlFactor     `@@`
	Right []*cqlOpFactor `@@*`
}

// Display

func (o cqlOperator) String() string {
	switch o {
	case opAnd:
		return "&"
	case opOr:
		return "|"
	}
	panic("unsupported operator")
}

func (e *cqlExact) String() string {
	parameters := ""
	for i, comp := range e.Components {
		parameters += comp.Name
		if i < len(e.Components)-1 {
			parameters += ", "
		}
	}
	return "EXACT(" + parameters + ")"

}

func (e *cqlContains) String() string {
	parameters := ""
	for i, comp := range e.Components {
		parameters += comp.Name
		if i < len(e.Components)-1 {
			parameters += ", "
		}
	}
	return "CONTAINS(" + parameters + ")"
}

func (v *cqlValue) String() string {
	if v.Exact != nil {
		return v.Exact.String()
	} else if v.Contains != nil {
		return v.Contains.String()
	} else if v.Not != nil {
		return "!(" + v.Not.SubExpression.String() + ")"
	} else if v.Subexpression != nil {
		return "(" + v.Subexpression.String() + ")"
	} else {
		panic("logic error displaying CQL ast. Check the code in cql.go")
	}
}

func (f *cqlFactor) String() string {
	out := f.Base.String()
	return out
}

func (o *cqlOpFactor) String() string {
	return fmt.Sprintf("%s %s", o.Operator, o.Factor)
}

func (t *cqlTerm) String() string {
	out := []string{t.Left.String()}
	for _, r := range t.Right {
		out = append(out, r.String())
	}
	return strings.Join(out, " ")
}

var CQLParser = participle.MustBuild[cqlTerm]()
