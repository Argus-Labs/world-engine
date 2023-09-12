package cql

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
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

// TODO: Value is sum type is represented as a product type. There is a case where multiple properties are filled out.
// Only one property may not be nil, The parser should prevent this from happening but for safety this should eventually
// be checked.
func valueToLayoutFilter(value *cqlValue, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
	if value.Not != nil {
		result_filter, err := valueToLayoutFilter(value.Not.SubExpression, stringToComponent)
		if err != nil {
			return nil, err
		}
		return filter.Not(result_filter), nil
	} else if value.Exact != nil {
		if len(value.Exact.Components) <= 0 {
			return nil, errors.New("EXACT cannot have zero parameters")
		}
		components := make([]component.IComponentType, 0, len(value.Exact.Components))
		for _, componentName := range value.Exact.Components {
			components = append(components, stringToComponent(componentName.Name))
		}
		return filter.Exact(components...), nil
	} else if value.Contains != nil {
		if len(value.Contains.Components) <= 0 {
			return nil, errors.New("CONTAINS cannot have zero parameters")
		}
		components := make([]component.IComponentType, 0, len(value.Contains.Components))
		for _, componentName := range value.Contains.Components {
			components = append(components, stringToComponent(componentName.Name))
		}
		return filter.Contains(components...), nil
	} else if value.Subexpression != nil {
		return termToLayoutFilter(value.Subexpression, stringToComponent)
	} else {
		return nil, errors.New("unknown error during conversion from CQL AST to LayoutFilter")
	}
}

func factorToLayoutFilter(factor *cqlFactor, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
	return valueToLayoutFilter(factor.Base, stringToComponent)
}

func opFactorToLayoutFilter(opFactor *cqlOpFactor, stringToComponent func(string) component.IComponentType) (*cqlOperator, filter.LayoutFilter, error) {
	resultFilter, err := factorToLayoutFilter(opFactor.Factor, stringToComponent)
	if err != nil {
		return nil, nil, err
	}
	return &opFactor.Operator, resultFilter, nil
}

func termToLayoutFilter(term *cqlTerm, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
	if term.Left == nil {
		return nil, errors.New("Not enough values in expression")
	}
	acc, err := factorToLayoutFilter(term.Left, stringToComponent)
	if err != nil {
		return nil, err
	}
	if len(term.Right) > 0 {
		for _, opFactor := range term.Right {
			operator, resultFilter, err := opFactorToLayoutFilter(opFactor, stringToComponent)
			if err != nil {
				return nil, err
			}
			if *operator == opAnd {
				acc = filter.And(acc, resultFilter)
			} else if *operator == opOr {
				acc = filter.Or(acc, resultFilter)
			} else {
				return nil, errors.New("invalid operator")
			}
		}
	}
	return acc, nil
}
