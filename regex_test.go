package tp

import (
	"testing"
	"unicode"

	"github.com/bobappleyard/assert"
)

func TestTokenization(t *testing.T) {
	e := `[a]\\b+|c`
	l := regexProg.Tokenize([]byte(e))

	ts := []token{
		charsetOpen{},
		char{of: 'a'},
		charsetClose{},
		slash{of: '\\'},
		char{of: 'b'},
		quantity{of: '+'},
		bar{},
		char{of: 'c'},
	}

	for _, tok := range ts {
		assert.True(t, l.Next())
		assert.Equal(t, tok, l.This())
	}
	assert.False(t, l.Next())
}

func TestRegexCompilation(t *testing.T) {
	type testTok struct {
		text string
	}

	p, err := NewLexer(Regex(`d(abc*)+`, func(start int, text string) (testTok, error) {
		return testTok{text: text}, nil
	}))

	assert.Nil(t, err)

	l := p.Tokenize([]byte("dababccdab"))

	assert.True(t, l.Next())
	assert.Equal(t, testTok{"dababcc"}, l.This())
	assert.True(t, l.Next())
	assert.Equal(t, testTok{"dab"}, l.This())
	assert.False(t, l.Next())
}

func TestParse(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		out  expr
	}{
		{
			name: "Any",
			in:   `.`,
			out:  match{start: 0, end: unicode.MaxRune},
		},
		{
			name: "Char",
			in:   `a`,
			out:  match{start: 'a', end: 'a'},
		},
		{
			name: "Esc",
			in:   `\(`,
			out:  match{start: '(', end: '('},
		},
		{
			name: "Named",
			in:   `\n`,
			out:  nest{match{start: '\n', end: '\n'}},
		},
		{
			name: "Option",
			in:   `a?`,
			out: nest{nested: choice{
				left:  match{start: 'a', end: 'a'},
				right: empty{},
			}},
		},
		{
			name: "Option",
			in:   `a+`,
			out:  repeat{repeated: match{start: 'a', end: 'a'}},
		},
		{
			name: "Option",
			in:   `a*`,
			out: nest{choice{
				left:  repeat{repeated: match{start: 'a', end: 'a'}},
				right: empty{},
			}},
		},
		{
			name: "Seq",
			in:   `ab`,
			out: seq{
				left:  match{start: 'a', end: 'a'},
				right: match{start: 'b', end: 'b'},
			},
		},
		{
			name: "LongSeq",
			in:   `abc`,
			out: seq{
				left: match{start: 'a', end: 'a'},
				right: seq{
					left:  match{start: 'b', end: 'b'},
					right: match{start: 'c', end: 'c'},
				},
			},
		},
		{
			name: "DashSeq",
			in:   `a-c`,
			out: seq{
				left: match{start: 'a', end: 'a'},
				right: seq{
					left:  match{start: '-', end: '-'},
					right: match{start: 'c', end: 'c'},
				},
			},
		},
		{
			name: "Choice",
			in:   `a|b`,
			out: choice{
				left:  match{start: 'a', end: 'a'},
				right: match{start: 'b', end: 'b'},
			},
		},
		{
			name: "TriChoice",
			in:   `a|b|c`,
			out: choice{
				left: choice{
					left:  match{start: 'a', end: 'a'},
					right: match{start: 'b', end: 'b'},
				},
				right: match{start: 'c', end: 'c'},
			},
		},
		{
			name: "QSeq",
			in:   `ab+`,
			out: seq{
				left:  match{start: 'a', end: 'a'},
				right: repeat{repeated: match{start: 'b', end: 'b'}},
			},
		},
		{
			name: "Group",
			in:   `(ab)+`,
			out: repeat{nest{nested: seq{
				left:  match{start: 'a', end: 'a'},
				right: match{start: 'b', end: 'b'},
			}}},
		},
		{
			name: "CharsetChar",
			in:   `[a]`,
			out:  nest{match{start: 'a', end: 'a'}},
		},
		{
			name: "CharsetChoice",
			in:   `[ab]`,
			out: nest{choice{
				left:  match{start: 'a', end: 'a'},
				right: match{start: 'b', end: 'b'},
			}},
		},
		{
			name: "CharsetMetas",
			in:   `[|+.]`,
			out: nest{choice{
				left: choice{
					left:  match{start: '|', end: '|'},
					right: match{start: '+', end: '+'},
				},
				right: match{start: '.', end: '.'}},
			},
		},
		{
			name: "CharsetNamed",
			in:   `[\n]`,
			out:  nest{match{start: '\n', end: '\n'}},
		},
		{
			name: "CharsetRange",
			in:   `[a-z]`,
			out:  nest{match{start: 'a', end: 'z'}},
		},
		{
			name: "InverseCharset",
			in:   `[^b-y]`,
			out: nest{choice{
				left:  match{start: 0, end: 'a'},
				right: match{start: 'z', end: unicode.MaxRune},
			}},
		},
		{
			name: "InverseCharset",
			in:   `[^bcd]`,
			out: nest{choice{
				left:  match{start: 0, end: 'a'},
				right: match{start: 'e', end: unicode.MaxRune},
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			toks, err := regexProg.Tokenize([]byte(test.in)).Force()
			if !assert.Nil(t, err) {
				return
			}
			expr, err := Parse(regexParser, toks)
			if !assert.Nil(t, err) {
				return
			}
			assert.Equal(t, expr, test.out)
		})
	}
}
