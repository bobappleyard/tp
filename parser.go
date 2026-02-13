package tp

import (
	"errors"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sync"
)

type ErrUnexpectedToken struct {
	Token any
}

func (e *ErrUnexpectedToken) Error() string {
	return fmt.Sprintf("unexpected token: %#v", e.Token)
}

type Grammar[T any] interface {
	Parse(T) T
}

func Parse[T, U any](g Grammar[U], toks []T) (U, error) {
	var zero U

	tokVals := make([]reflect.Value, len(toks))
	for i, t := range toks {
		tokVals[i] = reflect.ValueOf(t)
	}

	m := &matcher{
		root:  scanGrammar(reflect.ValueOf(g), reflect.TypeFor[U]()),
		state: make([][]item, 1, len(tokVals)),
		toks:  tokVals,
	}

	if err := m.run(); err != nil {
		return zero, err
	}

	rv, err := m.builder().build()
	if err != nil {
		return zero, err
	}

	return g.Parse(rv.Interface().(U)), nil
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
	} else if m, ok := key.MethodByName("Parser"); ok {
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

var (
	ErrFailedMatch    = errors.New("failed to match")
	ErrAmbiguousParse = errors.New("ambiguous parse")
)

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
