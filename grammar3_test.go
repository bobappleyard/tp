package tp_test

import (
	"fmt"

	"github.com/bobappleyard/tp"
)

// Reusable syntax

type delim[T, D any] struct {
	items []T
}

type delimGrammar[T, D any] struct{}

func (delim[T, D]) Grammar() delimGrammar[T, D] {
	return delimGrammar[T, D]{}
}

func (delimGrammar[T, D]) One(x T) delim[T, D] {
	return delim[T, D]{items: []T{x}}
}

func (delimGrammar[T, D]) Many(xs delim[T, D], _ D, x T) delim[T, D] {
	return delim[T, D]{items: append(xs.items, x)}
}

// Tokens

type identTok struct {
	name string
}

type dotTok struct {
}

// Syntax

type path struct {
	items []string
}

type reuseGrammar struct {
}

func (reuseGrammar) Parse(x path) (path, error) {
	return x, nil
}

func (reuseGrammar) Path(p delim[identTok, dotTok]) path {
	items := make([]string, len(p.items))
	for i, x := range p.items {
		items[i] = x.name
	}
	return path{items: items}
}

func ExampleGrammar_reuse() {
	toks := []any{
		identTok{name: "a"},
		dotTok{},
		identTok{name: "b"},
	}

	expr, err := tp.Parse(reuseGrammar{}, toks)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Printf("%#v\n", expr.items)

	// Output: []string{"a", "b"}
}
