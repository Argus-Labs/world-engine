package cql

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/filter"
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

var internalCQLParser = participle.MustBuild[CQLTerm]()

// TODO: Value is sum type is represented as a product type. There is a case where multiple properties are filled out.
// Only one property may not be nil, The parser should prevent this from happening but for safety this should eventually
// be checked.
func valueToLayoutFilter(value *CQLValue, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
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

func factorToLayoutFilter(factor *CQLFactor, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
	return valueToLayoutFilter(factor.Base, stringToComponent)
}

func opFactorToLayoutFilter(opFactor *CQLOpFactor, stringToComponent func(string) component.IComponentType) (*CQLOperator, filter.LayoutFilter, error) {
	resultFilter, err := factorToLayoutFilter(opFactor.Factor, stringToComponent)
	if err != nil {
		return nil, nil, err
	}
	return &opFactor.Operator, resultFilter, nil
}

func termToLayoutFilter(term *CQLTerm, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
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
			if *operator == OpAnd {
				acc = filter.And(acc, resultFilter)
			} else if *operator == OpOr {
				acc = filter.Or(acc, resultFilter)
			} else {
				return nil, errors.New("invalid operator")
			}
		}
	}
	return acc, nil
}

func CQLParse(cqlText string, stringToComponent func(string) component.IComponentType) (filter.LayoutFilter, error) {
	term, err := internalCQLParser.ParseString("", cqlText)
	if err != nil {
		return nil, err
	}
	resultFilter, err := termToLayoutFilter(term, stringToComponent)
	if err != nil {
		return nil, err
	}
	return resultFilter, nil
}
