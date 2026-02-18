package tp_test

import (
	"fmt"

	"github.com/bobappleyard/tp"
)

type ifTok struct{}
type elseTok struct{}
type boolTok struct{ value bool }
type openTok struct{}
type closeTok struct{}

type boolExpr struct {
	value bool
}

type ifStmt struct {
	test            boolExpr
	ifTrue, ifFalse block
}

type block struct{}

type ifStmtGrammar struct {
}

func (ifStmtGrammar) Parse(x ifStmt) (ifStmt, error) {
	return x, nil
}

func (ifStmtGrammar) Expr(x boolTok) boolExpr {
	return boolExpr{value: x.value}
}

func (ifStmtGrammar) If(_ ifTok, test boolExpr, ifTrue block, _ elseTok, ifFalse block) ifStmt {
	return ifStmt{
		test:    test,
		ifTrue:  ifTrue,
		ifFalse: ifFalse,
	}
}

func (ifStmtGrammar) Block(_ openTok, _ closeTok) block {
	return block{}
}

func ExampleGrammar_basic() {
	toks := []any{
		ifTok{},
		boolTok{value: true},
		openTok{},
		closeTok{},
		elseTok{},
		openTok{},
		closeTok{},
	}

	expr, err := tp.Parse(ifStmtGrammar{}, toks)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%#v\n", expr)

	// Output: tp_test.ifStmt{test:tp_test.boolExpr{value:true}, ifTrue:tp_test.block{}, ifFalse:tp_test.block{}}
}
