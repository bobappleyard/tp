package tp_test

import (
	"fmt"
	"strconv"

	"github.com/bobappleyard/tp"
)

type delimited[T, D any] struct {
	items []T
}

type delimitedGrammar[T, D any] struct{}

type delimitedItem[T, D any] struct {
	value T
}

func (delimited[T, D]) Grammar() delimitedGrammar[T, D] {
	return delimitedGrammar[T, D]{}
}

func (delimitedGrammar[T, D]) None() delimited[T, D] {
	return delimited[T, D]{}
}

func (delimitedGrammar[T, D]) Some(first T, rest []delimitedItem[T, D]) delimited[T, D] {
	items := []T{first}
	for _, x := range rest {
		items = append(items, x.value)
	}
	return delimited[T, D]{items: items}
}

func (delimitedGrammar[T, D]) Item(_ D, x T) delimitedItem[T, D] {
	return delimitedItem[T, D]{value: x}
}

type jsonToken interface {
	jsonToken()
}

type objectStartToken struct{}
type objectEndToken struct{}
type arrayStartToken struct{}
type arrayEndToken struct{}
type commaToken struct{}
type colonToken struct{}
type whitespaceToken struct{}
type numberToken struct{ value float64 }
type stringToken struct{ value string }

func (objectStartToken) jsonToken() {}
func (objectEndToken) jsonToken()   {}
func (arrayStartToken) jsonToken()  {}
func (arrayEndToken) jsonToken()    {}
func (commaToken) jsonToken()       {}
func (colonToken) jsonToken()       {}
func (whitespaceToken) jsonToken()  {}
func (numberToken) jsonToken()      {}
func (stringToken) jsonToken()      {}

var lexicon = must(tp.NewLexer(
	tp.Regex(`{`, emptyToken[objectStartToken]()),
	tp.Regex(`}`, emptyToken[objectEndToken]()),
	tp.Regex(`\[`, emptyToken[arrayStartToken]()),
	tp.Regex(`\]`, emptyToken[arrayEndToken]()),
	tp.Regex(`,`, emptyToken[commaToken]()),
	tp.Regex(`:`, emptyToken[colonToken]()),
	tp.Regex(`\s+`, emptyToken[whitespaceToken]()),
	tp.Regex(`\d+(\.\d+)?`, func(start int, text string) (jsonToken, error) {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return nil, err
		}
		return numberToken{value: f}, nil
	}),
	tp.Regex(`"([^"]|\\.)*"`, func(start int, text string) (jsonToken, error) {
		s, err := strconv.Unquote(text)
		if err != nil {
			return nil, err
		}
		return stringToken{value: s}, nil
	}),
))

func emptyToken[T jsonToken]() tp.TokenConstructor[jsonToken] {
	return func(start int, text string) (jsonToken, error) {
		var zero T
		return zero, nil
	}
}

func must[T any](x T, err error) T {
	if err != nil {
		panic(err)
	}
	return x
}

func removeWhitespace(toks []jsonToken) []jsonToken {
	res := make([]jsonToken, 0, len(toks))
	for _, x := range toks {
		if _, ok := x.(whitespaceToken); ok {
			continue
		}
		res = append(res, x)
	}
	return res
}

type jsonValue interface {
	jsonValue()
}

type jsonNumber float64
type jsonString string
type jsonObject map[string]jsonValue
type jsonArray []jsonValue

type jsonField struct {
	name  string
	value jsonValue
}

func (jsonNumber) jsonValue() {}
func (jsonString) jsonValue() {}
func (jsonObject) jsonValue() {}
func (jsonArray) jsonValue()  {}

type jsonGrammar struct{}

func (jsonGrammar) Parse(x jsonValue) (jsonValue, error) {
	return x, nil
}

func (jsonGrammar) Number(x numberToken) jsonNumber {
	return jsonNumber(x.value)
}

func (jsonGrammar) String(x stringToken) jsonString {
	return jsonString(x.value)
}

func (jsonGrammar) Array(_ arrayStartToken, xs delimited[jsonValue, commaToken], _ arrayEndToken) jsonValue {
	return jsonArray(xs.items)
}

func (jsonGrammar) Object(_ objectStartToken, fields delimited[jsonField, commaToken], _ objectEndToken) jsonValue {
	object := jsonObject{}
	for _, f := range fields.items {
		object[f.name] = f.value
	}
	return object
}

func (jsonGrammar) Field(name stringToken, _ colonToken, value jsonValue) jsonField {
	return jsonField{
		name:  name.value,
		value: value,
	}
}

func ExampleGrammar_simpleJson() {
	toks := removeWhitespace(must(lexicon.Tokenize([]byte(`

{
	"id": 1234,
	"items": [
		{
			"id": 775,
			"name": "item1",
			"type": "apples",
			"qty": 5
		}
	]	
}
	
	`)).Force()))

	value := must(tp.Parse(jsonGrammar{}, toks))

	fmt.Println(value)

	// Output: map[id:1234 items:[map[id:775 name:item1 qty:5 type:apples]]]
}
