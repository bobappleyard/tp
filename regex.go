package tp

import (
	"sort"
	"unicode"
	"unicode/utf8"
)

func Regex[T any](re string, yield TokenConstructor[T]) TokenSpec[T] {
	return func(l *Lexer[T]) error {
		end := l.State()
		l.Final(end, yield)

		s, err := regexProg.Tokenize([]byte(re)).Force()
		if err != nil {
			return err
		}
		e, err := Parse(regexParser, s)
		if err != nil {
			return err
		}

		e.compile(l, 0, end)
		return nil
	}
}

func (e empty) compile(prog programOps, start, end LexerState) {
	prog.Empty(start, end)
}

func (e match) compile(prog programOps, start, end LexerState) {
	prog.Range(start, end, e.start, e.end)
}

func (e seq) compile(prog programOps, start, end LexerState) {
	mid := prog.State()
	e.left.compile(prog, start, mid)
	e.right.compile(prog, mid, end)
}

func (e choice) compile(prog programOps, start, end LexerState) {
	e.left.compile(prog, start, end)
	e.right.compile(prog, start, end)
}

func (e repeat) compile(prog programOps, start, end LexerState) {
	// kleene closure
	s1, s2 := prog.State(), prog.State()
	prog.Empty(start, s1)
	prog.Empty(s2, s1)
	prog.Empty(s2, end)
	e.repeated.compile(prog, s1, s2)
}

func (e nest) compile(prog programOps, start, end LexerState) {
	e.nested.compile(prog, start, end)
}

var regexParser = NewParser[expr](&regexRules{map[rune]charset{
	'n': {ranges: []match{
		{start: '\n', end: '\n'},
	}},
	'r': {ranges: []match{
		{start: '\r', end: '\r'},
	}},
	't': {ranges: []match{
		{start: '\t', end: '\t'},
	}},
	's': {ranges: []match{
		{start: '\n', end: '\n'},
		{start: '\t', end: '\t'},
		{start: ' ', end: ' '},
	}},
	'c': {ranges: []match{
		{start: 'a', end: 'z'},
		{start: 'A', end: 'Z'},
		{start: '_', end: '_'},
	}},
	'w': {ranges: []match{
		{start: 'a', end: 'z'},
		{start: 'A', end: 'Z'},
		{start: '0', end: '9'},
		{start: '_', end: '_'},
	}},
	'd': {ranges: []match{
		{start: '0', end: '9'},
	}},
}})

type token interface {
	token()
}

type charsetOpen struct{}
type charsetClose struct{}
type charsetRange struct{}
type charsetInvert struct{}
type groupOpen struct{}
type groupClose struct{}
type quantity struct{ of rune }
type bar struct{}
type dot struct{}
type slash struct{ of rune }
type char struct{ of rune }

func (charsetOpen) token()   {}
func (charsetClose) token()  {}
func (charsetRange) token()  {}
func (charsetInvert) token() {}
func (groupOpen) token()     {}
func (groupClose) token()    {}
func (quantity) token()      {}
func (bar) token()           {}
func (dot) token()           {}
func (slash) token()         {}
func (char) token()          {}

var regexProg Lexer[token]

func init() {
	singleCharOp := func(r rune, yield func() token) {
		s := regexProg.State()
		regexProg.Rune(0, s, r)
		regexProg.Final(s, func(start int, text string) (token, error) {
			return yield(), nil
		})
	}

	charRune := func(s string) rune {
		c, _ := utf8.DecodeRuneInString(s)
		return c
	}

	singleCharOp('[', func() token { return charsetOpen{} })
	singleCharOp(']', func() token { return charsetClose{} })
	singleCharOp('-', func() token { return charsetRange{} })
	singleCharOp('^', func() token { return charsetInvert{} })
	singleCharOp('(', func() token { return groupOpen{} })
	singleCharOp(')', func() token { return groupClose{} })
	singleCharOp('|', func() token { return bar{} })
	singleCharOp('.', func() token { return dot{} })

	qEnd := regexProg.State()
	regexProg.Rune(0, qEnd, '*')
	regexProg.Rune(0, qEnd, '?')
	regexProg.Rune(0, qEnd, '+')
	regexProg.Final(qEnd, func(start int, text string) (token, error) {
		return quantity{of: charRune(text)}, nil
	})

	escMid := regexProg.State()
	escEnd := regexProg.State()
	regexProg.Rune(0, escMid, '\\')
	regexProg.Range(escMid, escEnd, ' ', '~')
	regexProg.Final(escEnd, func(start int, text string) (token, error) {
		return slash{of: charRune(text[1:])}, nil
	})

	anyEnd := regexProg.State()
	regexProg.Range(0, anyEnd, ' ', '~')
	regexProg.Final(anyEnd, func(start int, text string) (token, error) {
		return char{of: charRune(text)}, nil
	})
}

type programOps interface {
	State() LexerState
	Range(given, then LexerState, min, max rune)
	Empty(given, then LexerState)
}

type expr interface {
	expr()
	compile(p programOps, start, end LexerState)
}

type run interface {
	run()
	expr
}

type term interface {
	term()
	run
	expr
}

type charset struct {
	ranges []match
}

type empty struct{}

type match struct {
	start, end rune
}

type seq struct {
	left, right run
}

type choice struct {
	left, right expr
}

type repeat struct {
	repeated term
}

type nest struct {
	nested expr
}

func (choice) expr() {}

func (empty) run()  {}
func (empty) expr() {}

func (seq) run()  {}
func (seq) expr() {}

func (repeat) run()  {}
func (repeat) expr() {}

func (nest) term() {}
func (nest) run()  {}
func (nest) expr() {}

func (match) term() {}
func (match) run()  {}
func (match) expr() {}

type regexRules struct {
	escMap map[rune]charset
}

func (r *regexRules) ParseDot(e dot) term {
	return match{start: 0, end: unicode.MaxRune}
}

func (r *regexRules) ParseChar(e char) term {
	return match{start: e.of, end: e.of}
}

func (r *regexRules) ParseRange(e charsetRange) term {
	return match{start: '-', end: '-'}
}

func (r *regexRules) ParseGroup(open groupOpen, e expr, close groupClose) term {
	return nest{e}
}

func (r *regexRules) ParseCharset(op charsetOpen, contents charset, cl charsetClose) term {
	return contents.eval()
}

func (r *regexRules) ParseInverseCharset(op charsetOpen, inv charsetInvert, contents charset, cl charsetClose) term {
	return contents.inverse().eval()
}

func (r *regexRules) ParseEscaped(s slash) term {
	if e, ok := r.escMap[s.of]; ok {
		return e.eval()
	}
	return match{start: s.of, end: s.of}
}

func (r *regexRules) ParseSeq(left run, right run) run {
	return seq{left: left, right: right}
}

func (r *regexRules) ParseQuantifier(e term, q quantity) run {
	switch q.of {
	case '?':
		return nest{choice{left: e.(run), right: empty{}}}
	case '+':
		return repeat{repeated: e}
	case '*':
		return nest{choice{left: repeat{repeated: e}, right: empty{}}}
	}
	panic("unreachable")
}

func (r *regexRules) ParseChoice(left run, b bar, right run) choice {
	return choice{left: left, right: right}
}

func (r *regexRules) ParseMoreChoice(left choice, _ bar, right run) choice {
	return choice{left: left, right: right}
}

func (r *regexRules) ParseCharsetChar(c char) charset {
	return charset{ranges: []match{{start: c.of, end: c.of}}}
}

func (r *regexRules) ParseCharsetEsc(c slash) charset {
	if e, ok := r.escMap[c.of]; ok {
		return e
	}
	return charset{ranges: []match{{start: c.of, end: c.of}}}
}

func (r *regexRules) ParseCharsetQuantity(x quantity) charset {
	return charset{ranges: []match{{start: x.of, end: x.of}}}
}

func (r *regexRules) ParseCharsetBar(x bar) charset {
	return charset{ranges: []match{{start: '|', end: '|'}}}
}

func (r *regexRules) ParseCharsetDot(x dot) charset {
	return charset{ranges: []match{{start: '.', end: '.'}}}
}

func (r *regexRules) ParseCharsetRange(left char, op charsetRange, right char) charset {
	return charset{ranges: []match{{start: left.of, end: right.of}}}
}

func (r *regexRules) ParseCharsetChoice(left, right charset) charset {
	return charset{ranges: append(left.ranges, right.ranges...)}
}

func (contents charset) eval() term {
	var res expr = contents.ranges[0]

	for _, r := range contents.ranges[1:] {
		res = choice{left: res, right: r}
	}

	return nest{res}
}

func (s charset) inverse() charset {
	sort.Slice(s.ranges, func(i, j int) bool {
		return s.ranges[i].start < s.ranges[j].start
	})

	var res []match
	var last rune

	for _, r := range s.ranges {
		if r.end < r.start {
			continue
		}
		if r.start > last {
			res = append(res, match{start: last, end: r.start - 1})
		}
		if r.end >= last {
			last = r.end + 1
		}
	}

	if last < unicode.MaxRune {
		res = append(res, match{start: last, end: unicode.MaxRune})
	}

	return charset{ranges: res}
}
