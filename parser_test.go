package tp

import (
	"testing"

	"github.com/bobappleyard/assert"
)

type testTok interface {
	testTok()
}

type intTok struct {
	value int
}

type plusTok struct {
}

func (intTok) testTok()  {}
func (plusTok) testTok() {}

type testExpr interface {
	testExpr()
}

type add struct {
	left, right testExpr
}

type intVal struct {
	value int
}

type intList struct {
	vals []int
}

func (add) testExpr()    {}
func (intVal) testExpr() {}

type nullableRuleset struct {
}

func (nullableRuleset) Parse(x intList) (intList, error) {
	return x, nil
}

func (nullableRuleset) ParseInt(left intList, val intTok) intList {
	return intList{append(left.vals, val.value)}
}

func (nullableRuleset) ParseNull() intList {
	return intList{}
}

func TestNullableGrammar(t *testing.T) {
	toks := []testTok{
		intTok{1},
	}

	expr, err := Parse(nullableRuleset{}, toks)
	assert.Nil(t, err)
	assert.Equal(t, intList{[]int{1}}, expr)
}

func TestNullableEmptyGrammar(t *testing.T) {
	toks := []testTok{}

	expr, err := Parse(nullableRuleset{}, toks)
	assert.Nil(t, err)
	assert.Equal(t, intList{}, expr)
}

func TestNullableGrammarFail(t *testing.T) {
	toks := []testTok{
		intTok{1},
		plusTok{},
	}

	_, err := Parse(nullableRuleset{}, toks)
	assert.Equal(t, *(err.(*ErrUnexpectedToken)), ErrUnexpectedToken{Token: plusTok{}})
}

type nullableRightRuleset struct {
}

func (nullableRightRuleset) Parse(x intList) (intList, error) {
	return x, nil
}

func (nullableRightRuleset) ParseInt(val intTok, right intList) intList {
	return intList{append([]int{val.value}, right.vals...)}
}

func (nullableRightRuleset) ParseNull() intList {
	return intList{}
}

func TestNullableRightGrammar(t *testing.T) {
	toks := []testTok{
		intTok{1},
	}

	expr, err := Parse(nullableRightRuleset{}, toks)
	assert.Nil(t, err)
	assert.Equal(t, intList{[]int{1}}, expr)
}

type sliceRuleset struct {
}

func (sliceRuleset) Parse(x intList) (intList, error) {
	return x, nil
}

func (sliceRuleset) ParseInts(ints []intTok) intList {
	vals := make([]int, len(ints))
	for i, t := range ints {
		vals[i] = t.value
	}
	return intList{vals: vals}
}

func TestSliceGrammar(t *testing.T) {
	toks := []testTok{
		intTok{1},
		intTok{2},
		intTok{3},
	}

	expr, err := Parse(sliceRuleset{}, toks)
	assert.Nil(t, err)
	assert.Equal(t, intList{[]int{1, 2, 3}}, expr)
}

type optional[T any] struct {
	value *T
}

type optionalGrammar[T any] struct {
}

func (optional[T]) Grammar() optionalGrammar[T] {
	return optionalGrammar[T]{}
}

func (optionalGrammar[T]) ParseEmpty() optional[T] {
	return optional[T]{
		value: nil,
	}
}

func (optionalGrammar[T]) ParseOne(x T) optional[T] {
	return optional[T]{
		value: &x,
	}
}

type optionalRuleset struct {
}

func (optionalRuleset) Parse(x intList) (intList, error) {
	return x, nil
}

func (optionalRuleset) ParseSentence(x intTok, plus optional[plusTok]) intList {
	if plus.value != nil {
		return intList{vals: []int{x.value, x.value}}
	}
	return intList{vals: []int{x.value}}
}

func TestOptionalSuffix(t *testing.T) {
	toks := []testTok{
		intTok{1},
	}

	expr, err := Parse(optionalRuleset{}, toks)
	assert.Nil(t, err)
	assert.Equal(t, intList{[]int{1}}, expr)
}
