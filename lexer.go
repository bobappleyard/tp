package tp

import (
	"unicode/utf8"
)

type LexerState int

// Lexer is a simple Thompson-style NFA.
//
// It maintains a description of a state machine where movement between states is driven by reading
// an input text.
type Lexer[T any] struct {
	closeTransitions []closeTransition
	moveTransitions  []moveTransition
	finalStates      []finalState[T]
	maxState         LexerState
}

type TokenSpec[T any] func(l *Lexer[T]) error

func NewLexer[T any](tokens ...TokenSpec[T]) (*Lexer[T], error) {
	l := new(Lexer[T])
	for _, s := range tokens {
		if err := s(l); err != nil {
			return nil, err
		}
	}
	return l, nil
}

type closeTransition struct {
	Given, Then LexerState
}

type moveTransition struct {
	Given, Then LexerState
	Min, Max    rune
}

type finalState[T any] struct {
	Given LexerState
	Then  TokenConstructor[T]
}

type TokenConstructor[T any] func(start int, text string) (T, error)

type Stream[T any] struct {
	prog       *Lexer[T]
	src        []byte
	srcPos     int
	this, next []bool
	tok        T
	err        error
}

// Create a new state in the state machine.
func (p *Lexer[T]) State() LexerState {
	p.maxState++
	return p.maxState
}

// Given two states to move between, declare that encountering the rune r when in the from state
// will cause the machine to enter the to state.
func (p *Lexer[T]) Rune(from, to LexerState, r rune) {
	p.Range(from, to, r, r)
}

// Given two states to move between, declare that encountering any rune in the specified range
// (inclusive) when in the from state will cause the machine to enter the to state.
func (p *Lexer[T]) Range(from, to LexerState, min, max rune) {
	p.moveTransitions = append(p.moveTransitions, moveTransition{
		Given: from,
		Then:  to,
		Min:   min,
		Max:   max,
	})
}

// Create an empty transition, which is to say that entering the from state will cause the machine
// to immediately enter the to state as well.
func (p *Lexer[T]) Empty(from, to LexerState) {
	var pending []closeTransition
	for _, t := range p.closeTransitions {
		// avoid adding duplicates
		if t.Given == from && t.Then == to {
			return
		}
		// ensure transitive property is maintained
		if t.Given == to {
			pending = append(pending, closeTransition{
				Given: from,
				Then:  t.Then,
			})
		}
		if t.Then == from {
			pending = append(pending, closeTransition{
				Given: t.Given,
				Then:  to,
			})
		}
	}
	p.closeTransitions = append(p.closeTransitions, closeTransition{
		Given: from,
		Then:  to,
	})
	for _, t := range pending {
		p.Empty(t.Given, t.Then)
	}
}

// Indicate that a particular state is a final state, and attach a token constructor to it that will
// be invoked if the machine terminates in that state. The behaviour is undefined if the machine
// terminates in two final states, so be careful not to allow that to happen.
func (p *Lexer[T]) Final(given LexerState, then TokenConstructor[T]) {
	p.finalStates = append(p.finalStates, finalState[T]{
		Given: given,
		Then:  then,
	})
}

// Begin executing the described machine against a particular piece of text.
func (p *Lexer[T]) Tokenize(src []byte) *Stream[T] {
	return &Stream[T]{
		prog: p,
		src:  src,
		this: make([]bool, p.maxState+1),
		next: make([]bool, p.maxState+1),
	}
}

// Execute the machine until there are no more tokens and collect the tokens into a slice.
func (l *Stream[T]) Force() ([]T, error) {
	var res []T
	for l.Next() {
		res = append(res, l.This())
	}
	return res, l.Err()
}

// The error state of the execution. Once entered, the error state is permanent.
func (l *Stream[T]) Err() error {
	return l.err
}

// Execute the machine against the text and return whether successful.
func (l *Stream[T]) Next() bool {
	if l.err != nil {
		return false
	}
	return l.exec()
}

// Return the last matched token.
func (l *Stream[T]) This() T {
	return l.tok
}

func (l *Stream[T]) exec() bool {
	pos := l.srcPos
	start := pos
	end := pos
	final := -1
	running := true
	l.this[0] = true

	for running {
		c, n := utf8.DecodeRune(l.src[pos:])
		running = false
		clear(l.next)

		l.closeState()
		l.detectFinal(&final, &end, pos)

		if pos >= len(l.src) {
			break
		}

		l.moveState(&running, c)

		l.this, l.next = l.next, l.this
		pos = pos + n
	}

	if final == -1 {
		return false
	}

	l.tok, l.err = l.prog.finalStates[final].Then(start, string(l.src[start:end]))
	l.srcPos = end

	return l.err == nil
}

func (l *Stream[T]) closeState() {
	for _, op := range l.prog.closeTransitions {
		if !l.this[op.Given] {
			continue
		}
		l.this[op.Then] = true
	}
}

func (l *Stream[T]) detectFinal(final, end *int, pos int) {
	for i, op := range l.prog.finalStates {
		if !l.this[op.Given] {
			continue
		}

		if pos > *end || (pos == *end && i < *final) {
			*end = pos
			*final = i
		}
	}
}

func (l *Stream[T]) moveState(running *bool, c rune) {
	for _, op := range l.prog.moveTransitions {
		if !l.this[op.Given] {
			continue
		}

		if c < op.Min || c > op.Max {
			continue
		}

		l.next[op.Then] = true
		*running = true
	}
}
