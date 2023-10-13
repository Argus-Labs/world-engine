package cql

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/alecthomas/participle/v2"
	"pkg.world.dev/world-engine/cardinal/ecs/component"
	"pkg.world.dev/world-engine/cardinal/ecs/entity"
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

var internalCQLParser = participle.MustBuild[cqlTerm]()

// TODO: Value is sum type is represented as a product type. There is a case where multiple properties are filled out.
// Only one property may not be nil, The parser should prevent this from happening but for safety this should eventually
// be checked.
func valueToComponentFilter(value *cqlValue, stringToComponent func(string) (component.IComponentMetaData, error)) (filter.ComponentFilter, error) {
	if value.Not != nil {
		resultFilter, err := valueToComponentFilter(value.Not.SubExpression, stringToComponent)
		if err != nil {
			return nil, err
		}
		return filter.Not(resultFilter), nil
	} else if value.Exact != nil {
		if len(value.Exact.Components) <= 0 {
			return nil, errors.New("EXACT cannot have zero parameters")
		}
		components := make([]component.IComponentMetaData, 0, len(value.Exact.Components))
		for _, componentName := range value.Exact.Components {
			comp, err := stringToComponent(componentName.Name)
			if err != nil {
				return nil, err
			}
			components = append(components, comp)
		}
		return filter.Exact(components...), nil
	} else if value.Contains != nil {
		if len(value.Contains.Components) <= 0 {
			return nil, errors.New("CONTAINS cannot have zero parameters")
		}
		components := make([]component.IComponentMetaData, 0, len(value.Contains.Components))
		for _, componentName := range value.Contains.Components {
			comp, err := stringToComponent(componentName.Name)
			if err != nil {
				return nil, err
			}
			components = append(components, comp)
		}
		return filter.Contains(components...), nil
	} else if value.Subexpression != nil {
		return termToComponentFilter(value.Subexpression, stringToComponent)
	} else {
		return nil, errors.New("unknown error during conversion from CQL AST to ComponentFilter")
	}
}

func factorToComponentFilter(factor *cqlFactor, stringToComponent func(string) (component.IComponentMetaData, error)) (filter.ComponentFilter, error) {
	return valueToComponentFilter(factor.Base, stringToComponent)
}

func opFactorToComponentFilter(opFactor *cqlOpFactor, stringToComponent func(string) (component.IComponentMetaData, error)) (*cqlOperator, filter.ComponentFilter, error) {
	resultFilter, err := factorToComponentFilter(opFactor.Factor, stringToComponent)
	if err != nil {
		return nil, nil, err
	}
	return &opFactor.Operator, resultFilter, nil
}

func termToComponentFilter(term *cqlTerm, stringToComponent func(string) (component.IComponentMetaData, error)) (filter.ComponentFilter, error) {
	if term.Left == nil {
		return nil, errors.New("Not enough values in expression")
	}
	acc, err := factorToComponentFilter(term.Left, stringToComponent)
	if err != nil {
		return nil, err
	}
	for _, opFactor := range term.Right {
		operator, resultFilter, err := opFactorToComponentFilter(opFactor, stringToComponent)
		if err != nil {
			return nil, err
		}
		switch *operator {
		case opAnd:
			acc = filter.And(acc, resultFilter)
		case opOr:
			acc = filter.Or(acc, resultFilter)
		default:
			return nil, errors.New("invalid operator")
		}
	}
	return acc, nil
}

func CQLParse(cqlText string, stringToComponent func(string) (component.IComponentMetaData, error)) (filter.ComponentFilter, error) {
	term, err := internalCQLParser.ParseString("", cqlText)
	if err != nil {
		return nil, err
	}
	resultFilter, err := termToComponentFilter(term, stringToComponent)
	if err != nil {
		return nil, err
	}
	return resultFilter, nil
}

type QueryRequest struct {
	CQL string
}

type QueryResponse struct {
	Id   entity.ID         `json:"id"`
	Data []json.RawMessage `json:"data"`
}
