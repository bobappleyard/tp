# Text parsing library for Go

This allows you to specify a textual language in terms of a context-free grammar. This is a common,
well-understood abstraction for describing languge grammars. See
[here](https://en.wikipedia.org/wiki/Context-free_grammar) for more information.

This library differs from many others in that it does not require any pre-processing of source code
nor a complicated imperative API. It instead operates as an embedded domain-specific language using
Go's reflection capabilities.

In addition, there is a lexical analysis facility. This is primrily included as a fully-worked
example of the parser (see `regex.go`), but it does have an API that makes it well-suited to act as
the tokenisation step in a language parser.

The parser is an Earley parser, mostly following the guide given
[here](https://loup-vaillant.fr/tutorials/earley-parsing/).
