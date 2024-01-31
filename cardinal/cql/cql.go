package cql

import (
	"encoding/json"
	"fmt"
	filter2 "pkg.world.dev/world-engine/cardinal/filter"
	"strings"

	"github.com/alecthomas/participle/v2"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/cardinal/types/component"
	"pkg.world.dev/world-engine/cardinal/types/entity"
)

type cqlOperator int

const (
	opAnd cqlOperator = iota
	opOr
)

var operatorMap = map[string]cqlOperator{"&": opAnd, "|": opOr}

// Capture basically tells the parser library how to transform a string token that's parsed into the operator type.
func (o *cqlOperator) Capture(s []string) error {
	if len(s) == 0 {
		return eris.New("invalid operator")
	}
	operator, ok := operatorMap[s[0]]
	if !ok {
		return eris.New("invalid operator")
	}
	*o = operator
	return nil
}

type cqlComponent struct {
	Name string `@Ident`
}

type cqlAll struct{}

func (a *cqlAll) Capture(values []string) error {
	if values[0] == "ALL" && values[1] == "(" && values[2] == ")" {
		*a = cqlAll{}
	}
	return nil
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
	All           *cqlAll      `@("ALL" "(" ")")`
	Exact         *cqlExact    `| @@`
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

func (a *cqlAll) String() string {
	return "ALL()"
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
	//nolint: gocritic,nestif // its ok.
	if v.Exact != nil {
		return v.Exact.String()
	} else if v.Contains != nil {
		return v.Contains.String()
	} else if v.All != nil {
		return v.All.String()
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

// TODO: Msg is sum type is represented as a product type. There is a case where multiple properties are filled out.
// Only one property may not be nil, The parser should prevent this from happening but for safety this should eventually
// be checked.
func valueToComponentFilter(value *cqlValue, stringToComponent func(string) (component.ComponentMetadata, error)) (
	filter2.ComponentFilter, error,
) {
	if value.Not != nil { //nolint:gocritic,nestif // its fine.
		resultFilter, err := valueToComponentFilter(value.Not.SubExpression, stringToComponent)
		if err != nil {
			return nil, err
		}
		return filter2.Not(resultFilter), nil
	} else if value.Exact != nil {
		if len(value.Exact.Components) == 0 {
			return nil, eris.New("EXACT cannot have zero parameters")
		}
		components := make([]component.Component, 0, len(value.Exact.Components))
		for _, componentName := range value.Exact.Components {
			comp, err := stringToComponent(componentName.Name)
			if err != nil {
				return nil, eris.Wrap(err, "")
			}
			components = append(components, comp)
		}
		return filter2.Exact(components...), nil
	} else if value.All != nil {
		return filter2.All(), nil
	} else if value.Contains != nil {
		if len(value.Contains.Components) == 0 {
			return nil, eris.New("CONTAINS cannot have zero parameters")
		}
		components := make([]component.Component, 0, len(value.Contains.Components))
		for _, componentName := range value.Contains.Components {
			comp, err := stringToComponent(componentName.Name)
			if err != nil {
				return nil, eris.Wrap(err, "")
			}
			components = append(components, comp)
		}
		return filter2.Contains(components...), nil
	} else if value.Subexpression != nil {
		return termToComponentFilter(value.Subexpression, stringToComponent)
	} else {
		return nil, eris.New("unknown error during conversion from CQL AST to ComponentFilter")
	}
}

func factorToComponentFilter(factor *cqlFactor, stringToComponent func(string) (component.ComponentMetadata, error)) (
	filter2.ComponentFilter, error,
) {
	return valueToComponentFilter(factor.Base, stringToComponent)
}

func opFactorToComponentFilter(
	opFactor *cqlOpFactor,
	stringToComponent func(string) (component.ComponentMetadata, error),
) (*cqlOperator, filter2.ComponentFilter, error) {
	resultFilter, err := factorToComponentFilter(opFactor.Factor, stringToComponent)
	if err != nil {
		return nil, nil, err
	}
	return &opFactor.Operator, resultFilter, nil
}

func termToComponentFilter(
	term *cqlTerm, stringToComponent func(string) (component.ComponentMetadata, error),
) (filter2.ComponentFilter, error) {
	if term.Left == nil {
		return nil, eris.New("not enough values in expression")
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
			acc = filter2.And(acc, resultFilter)
		case opOr:
			acc = filter2.Or(acc, resultFilter)
		default:
			return nil, eris.New("invalid operator")
		}
	}
	return acc, nil
}

func Parse(
	cqlText string, stringToComponent func(string) (component.ComponentMetadata, error),
) (filter2.ComponentFilter, error) {
	term, err := internalCQLParser.ParseString("", cqlText)
	if err != nil {
		return nil, eris.Wrap(err, "")
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
	ID   entity.ID         `json:"id"`
	Data []json.RawMessage `json:"data"`
}
