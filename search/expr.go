package search

import (
	"strings"
)

// Queries combine terms: terms next to each other must all match, | or
// OR between them means either, and brackets group, also negated with a
// leading -: (smith | jones) born:<1850 -(place:leeds | place:york). A |
// right after a field's value, without spaces, gives the field another
// value: place:leeds|york is (place:leeds | place:york). Quotes keep |,
// brackets and OR as text. The end of the input is read leniently, as it
// is typed: a bracket still open counts as closed, and a | at the end is
// left out.

type tokenKind int

const (
	tokWord tokenKind = iota
	tokOpen
	tokClose
	tokOr
)

type token struct {
	kind   tokenKind
	text   string // a word, without its quotes
	quoted bool   // the word has quoted parts
	field  bool   // the word begins with a field name and a colon
	negate bool   // an opening bracket after -
	space  bool   // the token follows a space or the start
	alts   []string
}

// lex splits a query into words, brackets and |, honoring double quotes.
func lex(s string) ([]token, error) {
	var toks []token
	var cur strings.Builder
	inQuote, have, quoted, field, space := false, false, false, false, true
	add := func(t token) {
		t.space = space
		toks = append(toks, t)
		space = false
	}
	flush := func() {
		if have {
			add(token{kind: tokWord, text: cur.String(), quoted: quoted, field: field})
		}
		cur.Reset()
		have, quoted, field = false, false, false
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			have, quoted = true, true
		case inQuote:
			cur.WriteRune(r)
		case r == ' ' || r == '\t' || r == '\n':
			flush()
			space = true
		case r == '(':
			negate := have && !quoted && cur.String() == "-"
			if negate {
				cur.Reset()
				have = false
			}
			flush()
			add(token{kind: tokOpen, negate: negate})
		case r == ')':
			flush()
			add(token{kind: tokClose})
		case r == '|':
			flush()
			add(token{kind: tokOr})
		default:
			if r == ':' && have && !quoted && !field {
				field = true
			}
			cur.WriteRune(r)
			have = true
		}
	}
	if inQuote {
		return nil, &ParseError{"unterminated quote"}
	}
	flush()

	var out []token
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		// field:a|b: further values of the field.
		for t.kind == tokWord && t.field && i+2 < len(toks) && toks[i+1].kind == tokOr && !toks[i+1].space &&
			toks[i+2].kind == tokWord && !toks[i+2].space && !toks[i+2].field && !strings.HasPrefix(toks[i+2].text, "-") {
			t.alts = append(t.alts, toks[i+2].text)
			i += 2
		}
		if t.kind == tokWord && !t.quoted && t.text == "OR" {
			t = token{kind: tokOr, space: t.space}
		}
		out = append(out, t)
	}
	return out, nil
}

// expr is a parsed query: a term (kids nil), or the terms (or) or all
// (and) of its kids, possibly negated.
type expr struct {
	or     bool
	negate bool
	term   int // index into the query's terms
	kids   []*expr
}

type parser struct {
	toks []token
	pos  int
	// term parses a word into the index of a term, or -1 for a word that
	// adds nothing.
	term func(text string) (int, error)
}

// parseExpr parses the tokens of a query; nil means that there are no
// terms.
func parseExpr(toks []token, term func(string) (int, error)) (*expr, error) {
	p := parser{toks: toks, term: term}
	x, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.pos < len(p.toks) { // only a closing bracket stops before the end
		return nil, &ParseError{"unmatched )"}
	}
	return x, nil
}

func (p *parser) or() (*expr, error) {
	var kids []*expr
	for {
		x, err := p.and()
		if err != nil {
			return nil, err
		}
		if x != nil {
			kids = append(kids, x)
		}
		if p.pos >= len(p.toks) || p.toks[p.pos].kind != tokOr {
			break
		}
		p.pos++
	}
	switch len(kids) {
	case 0:
		return nil, nil
	case 1:
		return kids[0], nil
	}
	return &expr{or: true, kids: kids}, nil
}

func (p *parser) and() (*expr, error) {
	var kids []*expr
loop:
	for p.pos < len(p.toks) {
		t := p.toks[p.pos]
		var x *expr
		var err error
		switch t.kind {
		case tokOr, tokClose:
			break loop
		case tokOpen:
			p.pos++
			if x, err = p.or(); err != nil {
				return nil, err
			}
			if p.pos < len(p.toks) { // the closing bracket; at the end it may be missing
				p.pos++
			}
			switch {
			case x == nil || !t.negate:
			case x.negate: // -(-(…))
				x = &expr{kids: []*expr{x}, negate: true}
			default:
				x.negate = true
			}
		case tokWord:
			p.pos++
			if x, err = p.word(t); err != nil {
				return nil, err
			}
		}
		if x != nil {
			kids = append(kids, x)
		}
	}
	switch len(kids) {
	case 0:
		return nil, nil
	case 1:
		return kids[0], nil
	}
	return &expr{kids: kids}, nil
}

// word makes a term of a word, or of field:a|b the terms of every value.
func (p *parser) word(t token) (*expr, error) {
	if len(t.alts) == 0 {
		i, err := p.term(t.text)
		if err != nil || i < 0 {
			return nil, err
		}
		return &expr{term: i}, nil
	}
	text, negate := t.text, false
	if strings.HasPrefix(text, "-") && len(text) > 1 {
		text, negate = text[1:], true
	}
	field, value, _ := strings.Cut(text, ":")
	x := &expr{or: true, negate: negate}
	for _, v := range append([]string{value}, t.alts...) {
		i, err := p.term(field + ":" + v)
		if err != nil {
			return nil, err
		}
		if i >= 0 {
			x.kids = append(x.kids, &expr{term: i})
		}
	}
	return x, nil
}

// render writes an expression out; term writes a term.
func render(x *expr, term func(int) string) string {
	if x == nil {
		return ""
	}
	var s string
	switch {
	case x.kids == nil:
		s = term(x.term)
	default:
		parts := make([]string, len(x.kids))
		for i, k := range x.kids {
			parts[i] = render(k, term)
			if !x.or && k.or && !k.negate {
				parts[i] = "(" + parts[i] + ")"
			}
		}
		sep := " "
		if x.or {
			sep = " | "
		}
		s = strings.Join(parts, sep)
	}
	if x.negate {
		s = "-(" + s + ")"
	}
	return s
}

// holds evaluates an expression with a test for its terms.
func holds(x *expr, term func(int) bool) bool {
	var ok bool
	switch {
	case x == nil:
		return true
	case x.kids == nil:
		ok = term(x.term)
	case x.or:
		for _, k := range x.kids {
			if holds(k, term) {
				ok = true
				break
			}
		}
	default:
		ok = true
		for _, k := range x.kids {
			if !holds(k, term) {
				ok = false
				break
			}
		}
	}
	return ok != x.negate
}
