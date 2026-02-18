package tp

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sync"
)

var (
	ErrFailedMatch    = errors.New("failed to match")
	ErrAmbiguousParse = errors.New("ambiguous parse")
)

type ErrUnexpectedToken struct {
	Token any
}

func (e *ErrUnexpectedToken) Error() string {
	return fmt.Sprintf("unexpected token: %#v", e.Token)
}

// A specification of a context-free grammar. These are grammars that are sufficiently expressive to
// describe most data formats and programming languages. While this specifies a method, Parse, all
// of the public methods on the type are used by this library in order to describe the structure of
// the grammar.
//
// Each public method describes a rule. During parsing, these rules are consulted in order to
// determine what to do. The structure of a rule is:
//
//	S -> α
//
// Where S is a nonterminal symbol and α is a string of terminal and nonterminal symbols. A terminal
// symbol is defined outside the grammar, and is assumed to appear in the input. A nonterminal
// symbol is defined within the grammar by the rules within which it appears on the left hand side.
// The idea is that whenever you see the nonterminal, you can replace it with whatever appears on
// the right hand side, and if that is what you see in the input, then the rule matches.
//
// For more information, see https://en.wikipedia.org/wiki/Context-free_grammar
//
// Here we encode the rules into methods, where the nonterminal symbol is given as the return type
// and and the replacement string is given by the arguments. The method body affords the opportunity
// to apply some processing to the syntax during matching.
//
// Any types that appear in a rule's arguments that do not appear in any rule's return type cannot
// be created as a result of parsing. They are assumed to already exist in the input to be parsed,
// i.e. terminal symbols.
//
// An interface can be used in a grammar. This indicates that any of the types that implement the
// interface can appear in that location in the parse.
//
// If an argument is declared as a slice of a type, then it will be matched as zero or more of that
// type.
//
// If an argument is of a type with a method named Grammar, this is used to furnish more rules. The
// method is called once per type, and whatever it returns is treated as if it is part of the
// grammar, which is to say that its public methods are also treated as rules.
type Grammar[T, U any] interface {
	// Called on the parse tree, yielding the result of the parse. The argument type, T, indicates
	// where matching should begin.
	Parse(T) (U, error)
}

// Parse an input, given as a slice of tokens, using the set of rules described by the provided
// grammar. If it fails to parse, it will return an error indicating the problem.
func Parse[T, U, V any](g Grammar[U, V], toks []T) (V, error) {
	var zero V

	tokVals := make([]reflect.Value, len(toks))
	for i, t := range toks {
		tokVals[i] = reflect.ValueOf(t)
	}

	m := &matcher{
		root:  scanGrammar(reflect.ValueOf(g), reflect.TypeFor[U]()),
		state: make([][]item, min(1, len(tokVals)), len(tokVals)),
		toks:  tokVals,
	}

	if err := m.run(); err != nil {
		return zero, err
	}

	rv, err := m.builder().build()
	if err != nil {
		return zero, err
	}

	return g.Parse(rv.Interface().(U))
}

type symbol struct {
	// this symbol can be empty
	Nullable bool

	// if this is a token rule
	TokenType reflect.Type

	// if this is a nonterminal rule
	Predictions []*rule
}

type rule struct {
	// symbol this Implements
	Implements *symbol

	// array of symbols to match
	Deps []*symbol

	// the parser Host
	Host reflect.Value

	// debug: the rule's method Name
	Name string

	// Index of method into host
	Index int

	// function to call when building the parse tree
	Method func(host reflect.Value, args []reflect.Value) []reflect.Value
}

type scanner struct {
	host     reflect.Value
	rootType reflect.Type
	types    map[reflect.Type]*symbol
}

var cache = map[reflect.Type]*symbol{}
var lock sync.Mutex

func scanGrammar(ruleSet reflect.Value, rootType reflect.Type) *symbol {
	lock.Lock()
	defer lock.Unlock()

	if p, ok := cache[ruleSet.Type()]; ok {
		return p
	}

	s := &scanner{
		host:     ruleSet,
		rootType: rootType,
		types:    map[reflect.Type]*symbol{},
	}

	root := s.scan()
	cache[ruleSet.Type()] = root
	return root
}

func (s *scanner) scan() *symbol {
	s.ensure(s.rootType)
	s.scanMethods(s.host)
	s.markNullableTypes()
	s.fillOutInterfaces()
	s.markTokenTypes()

	return s.types[s.rootType]
}

func (s *scanner) scanMethods(host reflect.Value) {
	hostType := host.Type()
	for i := hostType.NumMethod() - 1; i >= 0; i-- {
		m := hostType.Method(i)
		if m.Name == "Parse" {
			continue
		}
		if !m.IsExported() {
			continue
		}
		deps := make([]*symbol, m.Type.NumIn()-1)
		for i := m.Type.NumIn() - 1; i >= 1; i-- {
			deps[i-1] = s.ensure(m.Type.In(i))
		}
		if m.Type.Out(0).Kind() == reflect.Slice {
			panic("explicit slice rules are not supported")
		}
		produces := s.ensure(m.Type.Out(0))
		produces.Predictions = append(produces.Predictions, &rule{
			Implements: produces,
			Deps:       deps,
			Host:       host,
			Name:       m.Name,
			Index:      m.Index,
			Method: func(host reflect.Value, args []reflect.Value) []reflect.Value {
				return m.Func.Call(args)
			},
		})
	}
}

func (s *scanner) markTokenTypes() {
	for k, v := range s.types {
		if len(v.Predictions) == 0 {
			v.TokenType = k
			continue
		}
	}
}

func (s *scanner) markNullableTypes() {
	var needsWork queue[*symbol]
	symUsers := map[*symbol][]*rule{}

	for _, sym := range s.types {
		for _, r := range sym.Predictions {
			for _, s := range r.Deps {
				symUsers[s] = append(symUsers[s], r)
			}
			if len(r.Deps) == 0 {
				sym.Nullable = true
				needsWork.Enqueue(sym)
			}
		}
	}

	for next := range needsWork.All() {
	nextRule:
		for _, r := range symUsers[next] {
			if r.Implements.Nullable {
				continue
			}
			for _, s := range r.Deps {
				if !s.Nullable {
					continue nextRule
				}
			}
			r.Implements.Nullable = true
			needsWork.Enqueue(r.Implements)
		}
	}
}

func (s *scanner) fillOutInterfaces() {
	var itfs []reflect.Type
	for k := range s.types {
		if k.Kind() != reflect.Interface {
			continue
		}
		itfs = append(itfs, k)
	}
	for len(itfs) != 0 {
		s.fillOutInterface(&itfs, itfs[0])
	}
}

func (s *scanner) fillOutInterface(itfs *[]reflect.Type, todo reflect.Type) {
	if !s.needsFilling(itfs, todo) {
		return
	}
	for k, v := range s.types {
		if k == todo {
			continue
		}
		if !k.AssignableTo(todo) {
			continue
		}
		if k.Kind() == reflect.Interface {
			s.fillOutInterface(itfs, k)
		}
		sym := s.types[todo]
		for _, r := range v.Predictions {
			sym.Predictions = append(sym.Predictions, &rule{
				Implements: sym,
				Deps:       r.Deps,
				Host:       r.Host,
				Name:       r.Name,
				Index:      r.Index,
				Method:     r.Method,
			})
		}
	}
}

func (s *scanner) needsFilling(itfs *[]reflect.Type, todo reflect.Type) bool {
	set := *itfs
	for i, t := range set {
		if t != todo {
			continue
		}
		copy(set[i:], set[i+1:])
		set = set[:len(set)-1]
		*itfs = set
		return true
	}
	return false
}

func (s *scanner) ensure(key reflect.Type) *symbol {
	if v, ok := s.types[key]; ok {
		return v
	}
	v := new(symbol)
	s.types[key] = v
	if key.Kind() == reflect.Slice {
		s.sliceTypeSymbol(v, key)
	} else if m, ok := key.MethodByName("Grammar"); ok {
		host := m.Func.Call([]reflect.Value{
			reflect.New(key).Elem(),
		})[0]
		s.scanMethods(host)
	}
	return v
}

func (s *scanner) sliceTypeSymbol(sliceSym *symbol, slice reflect.Type) {
	elem := slice.Elem()
	elemSym := s.ensure(elem)
	sliceSym.Predictions = append(sliceSym.Predictions, &rule{
		Implements: sliceSym,
		Deps:       []*symbol{},
		Host:       s.host,
		Name:       fmt.Sprintf("[]%s(nil)", elem),
		Index:      -1,
		Method: func(host reflect.Value, args []reflect.Value) []reflect.Value {
			res := reflect.MakeSlice(slice, 0, 0)
			return []reflect.Value{res}
		},
	})
	sliceSym.Predictions = append(sliceSym.Predictions, &rule{
		Implements: sliceSym,
		Deps:       []*symbol{sliceSym, elemSym},
		Host:       s.host,
		Name:       fmt.Sprintf("[]%s(append)", elem),
		Index:      -1,
		Method: func(host reflect.Value, args []reflect.Value) []reflect.Value {
			res := reflect.Append(args[1], args[2])
			return []reflect.Value{res}
		},
	})
}

type matcher struct {
	root  *symbol
	state [][]item
	toks  []reflect.Value
	cur   int
}

type item struct {
	// the rule that this item is matching
	rule *rule

	// where in the input this item begins
	position int

	// how far through the rule this item has progressed
	progress int
}

func (p *matcher) run() error {
	p.state = [][]item{nil}
	p.predict(p.root)
	for _, t := range p.toks {
		p.state = append(p.state, nil)

		p.step(t)
		p.cur++
	}
	p.finalStep()
	return p.matches(p.root)
}

func (p *matcher) step(tok reflect.Value) {
	for i := 0; i < len(p.state[p.cur]); i++ {
		item := p.state[p.cur][i]
		next, ok := item.nextSymbol()
		if !ok {
			p.complete(item)
			continue
		}
		if next.TokenType != nil {
			if tok.Type().AssignableTo(next.TokenType) {
				p.scan(item)
			}
			continue
		}
		if next.Nullable {
			p.advance(item)
		}
		p.predict(next)
	}
}

func (p *matcher) finalStep() {
	for i := 0; i < len(p.state[p.cur]); i++ {
		item := p.state[p.cur][i]
		next, ok := item.nextSymbol()
		if !ok {
			p.complete(item)
			continue
		}
		if next.Nullable {
			p.advance(item)
			p.predict(next)
		}
	}
}

func (p *matcher) matches(root *symbol) error {
	if len(p.state[len(p.state)-1]) == 0 {
		for i := range p.state[1:] {
			if len(p.state[i+1]) != 0 {
				continue
			}
			return &ErrUnexpectedToken{
				p.toks[i].Interface(),
			}
		}
	}
	for _, item := range p.state[len(p.state)-1] {
		if item.rule.Implements != root {
			continue
		}
		if item.position != 0 {
			continue
		}
		if _, ok := item.nextSymbol(); ok {
			continue
		}
		return nil
	}
	return io.ErrUnexpectedEOF
}

func (p *matcher) predict(s *symbol) {
	for _, prediction := range s.Predictions {
		p.addToCur(item{
			rule:     prediction,
			position: p.cur,
		})
	}
}

func (p *matcher) advance(x item) {
	p.addToCur(x.makeProgress())
}

func (p *matcher) scan(x item) {
	p.addToNext(x.makeProgress())
}

func (p *matcher) complete(x item) {
	for _, y := range p.state[x.position] {
		next, ok := y.nextSymbol()
		if !ok {
			continue
		}
		if next == x.rule.Implements {
			p.addToCur(y.makeProgress())
		}
	}
}

func (p *matcher) addToCur(x item) {
	p.addTo(p.cur, x)
}

func (p *matcher) addToNext(x item) {
	p.addTo(p.cur+1, x)
}

func (p *matcher) addTo(pos int, x item) {
	if !slices.Contains(p.state[pos], x) {
		p.state[pos] = append(p.state[pos], x)
	}
}

func (x item) complete() bool {
	_, ok := x.nextSymbol()
	return !ok
}

func (x item) nextSymbol() (*symbol, bool) {
	if x.progress == len(x.rule.Deps) {
		return nil, false
	}
	return x.rule.Deps[x.progress], true
}

func (x item) makeProgress() item {
	return item{
		rule:     x.rule,
		position: x.position,
		progress: x.progress + 1,
	}
}

type builder struct {
	root  *symbol
	state [][]item
	seen  []reflect.Value
}

type span struct {
	item     item
	at       int
	value    reflect.Value
	children []span
}

func (p *matcher) builder() *builder {
	flipped := p.flipState()
	for _, s := range flipped {
		slices.SortFunc(s, func(a, b item) int {
			if a.rule.Index == b.rule.Index {
				return a.position - b.position
			}
			return a.rule.Index - b.rule.Index
		})
	}
	return &builder{
		root:  p.root,
		state: flipped,
		seen:  p.toks,
	}
}

func (p *matcher) flipState() [][]item {
	flipped := make([][]item, len(p.state))
	for i, set := range p.state {
		for _, x := range set {
			if !x.complete() {
				continue
			}
			flipped[x.position] = append(flipped[x.position], item{
				rule:     x.rule,
				position: i,
				progress: x.progress,
			})
		}
	}
	return flipped
}

func (b *builder) build() (reflect.Value, error) {
	for _, top := range b.state[0] {
		if top.rule.Implements != b.root {
			continue
		}
		if top.position != len(b.seen) {
			continue
		}
		span, ok := b.findSpan(top, 0)
		if !ok {
			return reflect.Value{}, ErrFailedMatch
		}
		return b.buildFromSpan(span)
	}
	return reflect.Value{}, ErrFailedMatch
}

func (b *builder) findSpan(x item, at int) (span, bool) {
	children, ok := b.findSpanChildren(x.rule.Deps, at, x.position)
	if !ok {
		return span{}, false
	}
	return span{
		item:     x,
		at:       at,
		children: children,
	}, true
}

func (b *builder) buildFromSpan(s span) (reflect.Value, error) {
	if s.value.IsValid() {
		return s.value, nil
	}
	r := s.item.rule
	args := make([]reflect.Value, len(s.children)+1)
	args[0] = r.Host
	for i, c := range s.children {
		child, err := b.buildFromSpan(c)
		if err != nil {
			return reflect.Value{}, err
		}
		args[i+1] = child
	}

	rets := r.Method(r.Host, args)
	if len(rets) == 2 && !rets[1].IsNil() {
		return reflect.Value{}, rets[1].Interface().(error)
	}
	return rets[0], nil
}

func (b *builder) findSpanChildren(deps []*symbol, at, end int) ([]span, bool) {
	if len(deps) == 0 {
		return nil, at == end
	}
	if deps[0].TokenType != nil {
		return b.tokenSpan(deps, at, end)
	}
	return b.ruleSpan(deps, at, end)
}

func (b *builder) ruleSpan(deps []*symbol, at, end int) ([]span, bool) {
	sym := deps[0]
	for _, found := range b.state[at] {
		if found.rule.Implements != sym {
			continue
		}
		next, ok := b.findSpanChildren(deps[1:], found.position, end)
		if !ok {
			continue
		}
		inner, ok := b.findSpan(found, at)
		if !ok {
			continue
		}
		return append([]span{inner}, next...), true
	}
	return nil, false
}

func (b *builder) tokenSpan(deps []*symbol, at, end int) ([]span, bool) {
	sym := deps[0]
	if at >= len(b.seen) {
		return nil, false
	}
	if b.seen[at].Type().AssignableTo(sym.TokenType) {
		next, ok := b.findSpanChildren(deps[1:], at+1, end)
		if ok {
			return append([]span{{
				value: b.seen[at],
				at:    at,
			}}, next...), true
		}
	}
	return nil, false
}
