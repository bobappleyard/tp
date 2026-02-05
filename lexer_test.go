package tp

import (
	"strconv"
	"testing"

	"github.com/bobappleyard/assert"
)

func TestLexer(t *testing.T) {
	type Token struct {
		ID    int
		Start int
		Text  string
	}

	yieldToken := func(id int) func(start int, text string) (Token, error) {
		return func(start int, text string) (Token, error) {
			return Token{ID: id, Start: start, Text: text}, nil
		}
	}

	p := &Lexer[Token]{
		closeTransitions: []closeTransition{
			{Given: 1, Then: 2},
			{Given: 3, Then: 2},
			{Given: 3, Then: 4},
			{Given: 0, Then: 5},
			{Given: 6, Then: 5},
			{Given: 6, Then: 7},
			{Given: 0, Then: 8},
			{Given: 9, Then: 8},
			{Given: 9, Then: 10},
			{Given: 11, Then: 12},
			{Given: 13, Then: 12},
			{Given: 13, Then: 14},
		},
		moveTransitions: []moveTransition{
			{Given: 0, Min: 'a', Max: 'z', Then: 1},
			{Given: 2, Min: 'a', Max: 'z', Then: 3},
			{Given: 2, Min: '0', Max: '9', Then: 3},
			{Given: 5, Min: '0', Max: '9', Then: 6},
			{Given: 8, Min: '0', Max: '9', Then: 9},
			{Given: 10, Min: '.', Max: '.', Then: 11},
			{Given: 12, Min: '0', Max: '9', Then: 13},
			{Given: 0, Min: '.', Max: '.', Then: 15},
		},
		finalStates: []finalState[Token]{
			{Given: 4, Then: yieldToken(1)},
			{Given: 7, Then: yieldToken(2)},
			{Given: 14, Then: yieldToken(3)},
			{Given: 15, Then: yieldToken(4)},
		},
		maxState: 16,
	}

	for _, test := range []struct {
		name string
		in   string
		out  []Token
	}{
		{
			name: "Identifier",
			in:   "hello",
			out:  []Token{{ID: 1, Text: "hello"}},
		},
		{
			name: "Integer",
			in:   "123",
			out:  []Token{{ID: 2, Text: "123"}},
		},
		{
			name: "Float",
			in:   "123.4",
			out:  []Token{{ID: 3, Text: "123.4"}},
		},
		{
			name: "IntDot",
			in:   "123.up",
			out: []Token{
				{ID: 2, Start: 0, Text: "123"},
				{ID: 4, Start: 3, Text: "."},
				{ID: 1, Start: 4, Text: "up"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			l := p.Tokenize([]byte(test.in))
			for _, tok := range test.out {
				assert.True(t, l.Next())
				assert.Equal(t, l.This(), tok)
			}
			assert.False(t, l.Next())
		})
	}
}

func TestLexerBuild(t *testing.T) {
	type Token struct {
		ID    int
		Start int
		Text  string
	}

	yieldToken := func(id int) func(start int, text string) (Token, error) {
		return func(start int, text string) (Token, error) {
			return Token{ID: id, Start: start, Text: text}, nil
		}
	}

	var lp Lexer[Token]

	s1 := lp.State()
	s2 := lp.State()
	s3 := lp.State()

	end := lp.State()

	lp.Final(end, yieldToken(1))

	lp.Empty(s1, s2)
	lp.Empty(s2, s3)
	lp.Empty(0, s1)

	lp.Rune(s3, end, '0')

	l := lp.Tokenize([]byte("0"))

	assert.True(t, l.Next())
	assert.Equal(t, l.This().ID, 1)

}

type testToken interface {
	testToken()
}

type identifier struct {
	name string
}

type sep struct {
}

type literal[T any] struct {
	value T
}

func (identifier) testToken() {}
func (sep) testToken()        {}
func (literal[T]) testToken() {}

func TestTypedLexer(t *testing.T) {
	p := &Lexer[testToken]{
		closeTransitions: []closeTransition{
			{Given: 1, Then: 2},
			{Given: 3, Then: 2},
			{Given: 3, Then: 4},
			{Given: 0, Then: 5},
			{Given: 6, Then: 5},
			{Given: 6, Then: 7},
			{Given: 0, Then: 8},
			{Given: 9, Then: 8},
			{Given: 9, Then: 10},
			{Given: 11, Then: 12},
			{Given: 13, Then: 12},
			{Given: 13, Then: 14},
		},
		moveTransitions: []moveTransition{
			{Given: 0, Min: 'a', Max: 'z', Then: 1},
			{Given: 2, Min: 'a', Max: 'z', Then: 3},
			{Given: 2, Min: '0', Max: '9', Then: 3},
			{Given: 5, Min: '0', Max: '9', Then: 6},
			{Given: 8, Min: '0', Max: '9', Then: 9},
			{Given: 10, Min: '.', Max: '.', Then: 11},
			{Given: 12, Min: '0', Max: '9', Then: 13},
			{Given: 0, Min: '.', Max: '.', Then: 15},
		},
		finalStates: []finalState[testToken]{
			{Given: 4, Then: func(start int, text string) (testToken, error) {
				return identifier{name: text}, nil
			}},
			{Given: 7, Then: func(start int, text string) (testToken, error) {
				value, err := strconv.Atoi(text)
				return literal[int]{value: value}, err
			}},
			{Given: 14, Then: func(start int, text string) (testToken, error) {
				value, err := strconv.ParseFloat(text, 64)
				return literal[float64]{value: value}, err
			}},
			{Given: 15, Then: func(start int, text string) (testToken, error) {
				return sep{}, nil
			}},
		},
		maxState: 16,
	}

	for _, test := range []struct {
		name string
		in   string
		out  []testToken
	}{
		{
			name: "Identifier",
			in:   "hello",
			out:  []testToken{identifier{"hello"}},
		},
		{
			name: "Integer",
			in:   "123",
			out:  []testToken{literal[int]{123}},
		},
		{
			name: "Float",
			in:   "123.4",
			out:  []testToken{literal[float64]{123.4}},
		},
		{
			name: "IntDot",
			in:   "123.up",
			out: []testToken{
				literal[int]{123},
				sep{},
				identifier{"up"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			l := p.Tokenize([]byte(test.in))
			for _, tok := range test.out {
				assert.True(t, l.Next())
				assert.Equal(t, tok, l.This())
			}
			assert.False(t, l.Next())
		})
	}
}

func TestFailingLex(t *testing.T) {

}
