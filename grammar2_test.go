package tp_test

import (
	"fmt"

	"github.com/bobappleyard/tp"
)

type intTok struct {
	value int
}

type plusTok struct {
}

// Expression type
//
// We encode this as an interface with a private marker method. This is so that we can declare that
// add and intVal are both expression types.

type expr interface {
	testExpr()
}

type add struct {
	left, right expr
}

type intVal struct {
	value int
}

func (add) testExpr()    {}
func (intVal) testExpr() {}

type interfaceGrammar struct {
}

func (interfaceGrammar) Parse(x expr) (expr, error) {
	return x, nil
}

func (interfaceGrammar) Int(val intTok) intVal {
	return intVal(val)
}

func (interfaceGrammar) Add(left expr, op plusTok, right expr) add {
	return add{left: left, right: right}
}

func ExampleGrammar_interface() {
	toks := []any{
		intTok{1},
		plusTok{},
		intTok{2},
	}

	expr, err := tp.Parse(interfaceGrammar{}, toks)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%#v\n", expr)

	// Output: tp_test.add{left:tp_test.intVal{value:1}, right:tp_test.intVal{value:2}}
}
