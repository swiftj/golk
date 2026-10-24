package guard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// What is visible (X-GUARD-03, D-011, D-012):
//
// The value is encoded with encoding/json and the encoding is walked as a
// token stream. Every string VALUE is checked (object keys are not), at any
// depth, whatever the surrounding shape: block text, titles, alt text,
// URLs, mention names and labels, tags, unmodelled fields and lk-raw
// blocks (which encode as their own JSON). Numbers, booleans and null are
// not text.
//
// An object is DM-only, and its whole subtree is skipped, when it is a
// block (an element of a "blocks" or "children" array, or the checked
// value itself or an element of it) with a "kind" member whose value is the string "secret"
// (a "kind":"secret" member on a text run or any other object exempts
// nothing, as for "isHidden" below), or when it is a
// property (a sidebar property or an infobox row: its "kind" is one of the
// property-capable kinds) with an "isHidden" member whose value is true.
// Only properties can be hidden; "isHidden" on any other object (a text
// run, a paragraph) exempts nothing, and both exemptions apply only where
// the API reads such an object (see at below), so a field the API ignores can't hide
// player-visible text from the guard. Key names and values are matched
// exactly (case-sensitive, as the API reads them).
//
// Duplicate keys. Raw JSON (lk-raw blocks, unmodelled fields) can repeat a
// key, and JSON parsers disagree on which occurrence wins. The guard is
// safe under either reading: an object is exempt only when EVERY "kind"
// member in it is "secret", or EVERY "isHidden" member is true. (For an
// object without duplicates this is the plain rule above; with duplicates
// it is at least as strict as both first-wins and last-wins, the latter
// being what lk/blocks and encoding/json do.) And every occurrence of a
// duplicated string member is checked, not only the one a parser keeps.

type walker struct {
	// run receives the "text" of the run being read, when the walker is in
	// a content array (see array).
	run  *runText
	dec  *json.Decoder
	m    *matcher
	path []seg
	hit  []bool
	buf  []rune
	idx  []int
}

type runText struct {
	text string
	ok   bool
}

type seg struct {
	key   string
	index int
	isKey bool
}

func check(m *matcher, v any) ([]Violation, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("guard: encode content: %w", err)
	}
	if len(m.terms) == 0 {
		return nil, nil
	}
	w := &walker{dec: json.NewDecoder(bytes.NewReader(data)), m: m, hit: make([]bool, len(m.terms))}
	w.dec.UseNumber()
	vs, err := w.value()
	if err != nil {
		return nil, fmt.Errorf("guard: read encoded content: %w", err)
	}
	if _, err := w.dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("guard: read encoded content: trailing data")
	}
	return vs, nil
}

// value reads one JSON value and returns the violations in it.
func (w *walker) value() ([]Violation, error) {
	tok, err := w.dec.Token()
	if err != nil {
		return nil, err
	}
	return w.after(tok)
}

func (w *walker) after(tok json.Token) ([]Violation, error) {
	switch t := tok.(type) {
	case string:
		return w.text(t), nil
	case json.Delim:
		switch t {
		case '{':
			return w.object()
		case '[':
			return w.array()
		}
		return nil, fmt.Errorf("unexpected %v", t)
	}
	return nil, nil
}

func (w *walker) object() ([]Violation, error) {
	var vs []Violation
	run := w.run // this object's own text, if it is a run
	w.run = nil
	asBlock, asProp := w.at(blockListKeys, nil), w.at(propListKeys, propKeys)
	kinds, secretKinds, propKinds, hiddens, trueHiddens := 0, 0, 0, 0, 0
	for w.dec.More() {
		kt, err := w.dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := kt.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected object key %v", kt)
		}
		tok, err := w.dec.Token()
		if err != nil {
			return nil, err
		}
		switch key {
		case "kind":
			kinds++
			if s, ok := tok.(string); ok && s == "secret" {
				secretKinds++
			} else if ok && propertyKinds[s] {
				propKinds++
			}
		case "isHidden":
			hiddens++
			if b, ok := tok.(bool); ok && b {
				trueHiddens++
			}
		case "text":
			if s, ok := tok.(string); ok && run != nil {
				run.text, run.ok = s, true
			}
		case "break":
			if b, ok := tok.(bool); ok && b && run != nil && !run.ok {
				run.text, run.ok = "\n", true
			}
		}
		w.path = append(w.path, seg{key: key, isKey: true})
		sub, err := w.after(tok)
		w.path = w.path[:len(w.path)-1]
		if err != nil {
			return nil, err
		}
		vs = append(vs, sub...)
	}
	if _, err := w.dec.Token(); err != nil { // '}'
		return nil, err
	}
	if (asBlock && kinds > 0 && kinds == secretKinds) || (asProp && hiddens > 0 && hiddens == trueHiddens && kinds > 0 && kinds == propKinds) {
		return nil, nil
	}
	return vs, nil
}

// Exemptions are positional: an object can be DM-only only where the API
// reads that kind of object. "kind":"secret" exempts a block, an element
// of a block list; "isHidden":true exempts a property, a sidebar property
// or an infobox row. The same members on a text run or any other object
// exempt nothing. The checked value itself, and each element of it when it
// is an array, may be either.
var (
	blockListKeys = map[string]bool{"blocks": true, "children": true}
	propListKeys  = map[string]bool{"properties": true, "rows": true}
	propKeys      = map[string]bool{"property": true}
)

// at reports whether the object being read is an element of an array that
// is the value of a member named in lists, or the value of a member named
// in members, or the checked value or one of its elements.
func (w *walker) at(lists, members map[string]bool) bool {
	n := len(w.path)
	switch {
	case n == 0, n == 1 && !w.path[0].isKey:
		return true
	case w.path[n-1].isKey:
		return members[w.path[n-1].key]
	case n >= 2 && w.path[n-2].isKey:
		return lists[w.path[n-2].key]
	}
	return false
}

func (w *walker) array() ([]Violation, error) {
	var vs []Violation
	// A content array holds a block's runs. Players read them as one text,
	// so a term split across runs ("the Crimson [Accord](url)", or
	// "Zor**blax**") is looked for in the joined text too. A run without
	// text (a mention) joins as a separator no term spans.
	n := len(w.path)
	runs := n > 0 && w.path[n-1].isKey && w.path[n-1].key == "content"
	var joined strings.Builder
	for i := 0; w.dec.More(); i++ {
		var rt runText
		if runs {
			w.run = &rt
		}
		w.path = append(w.path, seg{index: i})
		sub, err := w.value()
		w.path = w.path[:len(w.path)-1]
		w.run = nil
		if err != nil {
			return nil, err
		}
		vs = append(vs, sub...)
		if runs {
			if rt.ok {
				joined.WriteString(rt.text)
			} else {
				joined.WriteString(" | ")
			}
		}
	}
	if _, err := w.dec.Token(); err != nil { // ']'
		return nil, err
	}
	if runs {
		found := map[string]bool{}
		for _, v := range vs {
			found[v.Term] = true
		}
		for _, v := range w.text(joined.String()) {
			if !found[v.Term] {
				vs = append(vs, v)
			}
		}
	}
	return vs, nil
}

func (w *walker) text(s string) []Violation {
	w.buf = normalize(w.buf[:0], s)
	w.idx = w.m.scan(w.buf, w.hit, w.idx[:0])
	if len(w.idx) == 0 {
		return nil
	}
	p := renderPath(w.path)
	vs := make([]Violation, 0, len(w.idx))
	for _, ti := range w.idx {
		vs = append(vs, Violation{Term: w.m.terms[ti].raw, Path: p})
	}
	return vs
}

// renderPath writes a JSONPath-style path: $.name, $.blocks[2].text,
// $["weird key"]. A key is written with a dot when it is an ASCII
// identifier, and bracket-quoted (Go/JSON string syntax) otherwise.
func renderPath(path []seg) string {
	var b strings.Builder
	b.WriteByte('$')
	for _, s := range path {
		switch {
		case !s.isKey:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.index))
			b.WriteByte(']')
		case isIdent(s.key):
			b.WriteByte('.')
			b.WriteString(s.key)
		default:
			b.WriteByte('[')
			b.WriteString(strconv.Quote(s.key))
			b.WriteByte(']')
		}
	}
	return b.String()
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '_' || c == '$' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || i > 0 && '0' <= c && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// propertyKinds are the kinds a property (sidebar property or infobox row)
// can have: the property-capable block kinds, and unsupported.
var propertyKinds = map[string]bool{
	"textField": true, "keyValue": true, "image": true, "meter": true, "resourceCollection": true,
	"resourceLink": true, "embed": true, "map": true, "unsupported": true,
}
