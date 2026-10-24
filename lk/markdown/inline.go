package markdown

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/opt"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// ---- Rendering ----

// ictx says where inline text is rendered.
type ictx struct {
	safe    bool // bold, italic and strike as span flags instead of delimiters
	heading bool // one line; '#' escaped
	cell    bool // one line, in a GFM table cell
}

type tokKind int

const (
	tText tokKind = iota
	tMark
	tBreak
)

type tok struct {
	k tokKind
	s string
}

// renderRuns writes runs as inline LK Markdown (§2.2). ok is false when they
// can't be written so that they parse back identically: an unknown run, an
// unmodelled field, empty text, two neighbours a parse would merge, or a
// line break where none can go.
func renderRuns(rs blocks.Runs, x ictx) (string, bool) {
	if !runsOK(rs, x) {
		return "", false
	}
	e := &emitter{x: x, ok: true}
	e.level(rs, lvLink)
	if !e.ok {
		return "", false
	}
	return assemble(e.toks, x)
}

func runsOK(rs blocks.Runs, x ictx) bool {
	for i, r := range rs {
		if blocks.HasUnmodelled(r) {
			return false
		}
		switch v := r.(type) {
		case *blocks.Text:
			if v.Text == "" || !textOK(v.Text) {
				return false
			}
			if i > 0 {
				if p, ok := rs[i-1].(*blocks.Text); ok && sameMarks(p, v) {
					return false
				}
			}
		case *blocks.Mention:
			if !textOK(v.Label.Or("")) || !textOK(v.Name.Or("")) {
				return false
			}
		case *blocks.Break:
			if x.heading || x.cell || i == len(rs)-1 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// sameMarks reports whether two text runs have the same marks, so a parse
// would merge them.
func sameMarks(a, b *blocks.Text) bool {
	return a.Bold == b.Bold && a.Italic == b.Italic && a.Underline == b.Underline &&
		a.Strike == b.Strike && a.Code == b.Code && a.Sub == b.Sub && a.Sup == b.Sup &&
		a.Link == b.Link && a.Color == b.Color && a.Highlight == b.Highlight
}

// Mark levels, outermost first (M-INL-02).
const (
	lvLink = iota
	lvBold
	lvItalic
	lvStrike
	lvSpan
	lvLeaf
)

type emitter struct {
	x    ictx
	toks []tok
	ok   bool
}

func (e *emitter) mark(s string) { e.toks = append(e.toks, tok{tMark, s}) }

func (e *emitter) next(lv int) int {
	if e.x.safe && lv == lvLink {
		return lvSpan
	}
	return lv + 1
}

// key is the value of mark level lv on r; on is false when r doesn't carry
// the mark (mentions and breaks carry none).
func (e *emitter) key(r blocks.Run, lv int) (key string, on bool) {
	t, ok := r.(*blocks.Text)
	if !ok {
		return "", false
	}
	switch lv {
	case lvLink:
		u, set := t.Link.Get()
		return u, set
	case lvBold:
		return "", t.Bold
	case lvItalic:
		return "", t.Italic
	case lvStrike:
		return "", t.Strike
	case lvSpan:
		k := e.spanAttrs(t)
		return k, k != ""
	}
	return "", false
}

func (e *emitter) spanAttrs(t *blocks.Text) string {
	w := newAttrW()
	if e.x.safe {
		flagIf(w, "bold", t.Bold)
		flagIf(w, "italic", t.Italic)
		flagIf(w, "strike", t.Strike)
	}
	flagIf(w, "underline", t.Underline)
	flagIf(w, "sub", t.Sub)
	flagIf(w, "sup", t.Sup)
	w.optStr("color", t.Color)
	w.optStr("highlight", t.Highlight)
	if !w.ok {
		e.ok = false
	}
	return w.String()
}

func flagIf(w *attrW, name string, on bool) {
	if on {
		w.raw(name)
	}
}

// level writes rs, grouping neighbours that share mark level lv.
func (e *emitter) level(rs []blocks.Run, lv int) {
	if lv == lvLeaf {
		for _, r := range rs {
			e.leaf(r)
		}
		return
	}
	for i := 0; i < len(rs); {
		k, on := e.key(rs[i], lv)
		j := i + 1
		for j < len(rs) {
			k2, on2 := e.key(rs[j], lv)
			if on2 != on || k2 != k {
				break
			}
			j++
		}
		g := rs[i:j]
		i = j
		if !on {
			e.level(g, e.next(lv))
			continue
		}
		switch lv {
		case lvLink:
			dest, ok := linkDest(k)
			if !ok {
				e.ok = false
				return
			}
			e.mark("[")
			e.level(g, e.next(lv))
			e.mark("](" + dest + ")")
		case lvBold, lvItalic, lvStrike:
			sub := &emitter{x: e.x, ok: true}
			sub.level(g, lv+1)
			e.ok = e.ok && sub.ok
			delim, name := [...]string{"**", "*", "~~"}[lv-lvBold], [...]string{"bold", "italic", "strike"}[lv-lvBold]
			if edgeSpace(sub.toks) {
				// Emphasis can't start or end with whitespace; a span can.
				e.mark("[")
				e.toks = append(e.toks, sub.toks...)
				e.mark("]{" + name + "}")
			} else {
				e.mark(delim)
				e.toks = append(e.toks, sub.toks...)
				e.mark(delim)
			}
		case lvSpan:
			e.mark("[")
			e.level(g, lvLeaf)
			e.mark("]{" + k + "}")
		}
	}
}

func edgeSpace(ts []tok) bool {
	if len(ts) == 0 {
		return true
	}
	if f := ts[0]; f.k == tText {
		if r, _ := utf8.DecodeRuneInString(f.s); unicode.IsSpace(r) {
			return true
		}
	}
	if l := ts[len(ts)-1]; l.k == tText {
		if r, _ := utf8.DecodeLastRuneInString(l.s); unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

func (e *emitter) leaf(r blocks.Run) {
	switch v := r.(type) {
	case *blocks.Break:
		e.toks = append(e.toks, tok{tBreak, ""})
	case *blocks.Text:
		if v.Code {
			if strings.ContainsAny(v.Text, "\n\r") {
				e.ok = false
				return
			}
			e.mark(codeSpan(v.Text, e.x.cell))
			return
		}
		e.toks = append(e.toks, tok{tText, v.Text})
	case *blocks.Mention:
		w := newAttrW()
		w.str("mention", v.ResourceID)
		w.optStr("doc", v.DocumentID)
		var text string
		if l, ok := v.Label.Get(); ok {
			text = l
			w.raw("label")
			w.optStr("name", v.Name)
		} else if n, ok := v.Name.Get(); ok {
			text = n
			if n == "" {
				w.str("name", "")
			}
		}
		w.optStr("color", v.Color)
		w.optStr("highlight", v.Highlight)
		if !w.ok {
			e.ok = false
			return
		}
		e.mark("[")
		if text != "" {
			e.toks = append(e.toks, tok{tText, text})
		}
		e.mark("]{" + w.String() + "}")
	}
}

// assemble joins tokens, escaping text for where it sits.
func assemble(ts []tok, x ictx) (string, bool) {
	var b strings.Builder
	lineStart := true
	for i, t := range ts {
		switch t.k {
		case tBreak:
			if x.heading || x.cell || i == len(ts)-1 {
				return "", false
			}
			b.WriteString("\\\n")
			lineStart = true
		case tMark:
			b.WriteString(t.s)
			lineStart = false
		case tText:
			last := i == len(ts)-1
			ctx := escCtx{
				lineStart: lineStart,
				lineEnd:   last, // spaces before a backslash break are kept
				heading:   x.heading,
				bang:      !last && ts[i+1].k == tMark && strings.HasPrefix(ts[i+1].s, "["),
			}
			b.WriteString(escapeText(t.s, ctx))
			lineStart = false
		}
	}
	return b.String(), true
}

// ---- goldmark inline parsers ----

// spanNode is a bracketed span [x]{…}: marks, or a mention.
type spanNode struct {
	ast.BaseInline
	attrs []attr
}

var kindSpan = ast.NewNodeKind("LKSpan")

func (n *spanNode) Kind() ast.NodeKind { return kindSpan }
func (n *spanNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

// attrSetNode is an attribute set {…} in inline text: after an image, at the
// end of a heading or row value, or at the start of a list item.
type attrSetNode struct {
	ast.BaseInline
	attrs []attr
}

var kindAttrSet = ast.NewNodeKind("LKAttrSet")

func (n *attrSetNode) Kind() ast.NodeKind { return kindAttrSet }
func (n *attrSetNode) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

// errInline is inline input the parsers reject; converting it fails.
type errInline struct {
	ast.BaseInline
	reason string
}

var kindErrInline = ast.NewNodeKind("LKErrInline")

func (n *errInline) Kind() ast.NodeKind { return kindErrInline }
func (n *errInline) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Reason": n.reason}, nil)
}

// spanState is an open '[' of a span, closed at the source offset close.
type spanState struct {
	ast.BaseInline
	close  int
	bottom ast.Node
}

var kindSpanState = ast.NewNodeKind("LKSpanState")

func (n *spanState) Kind() ast.NodeKind { return kindSpanState }
func (n *spanState) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, nil, nil)
}

var (
	spanStackKey   = parser.NewContextKey()
	bracketsKey    = parser.NewContextKey()
	autolinkPrefix = regexp.MustCompile(`^<([A-Za-z][A-Za-z0-9.+-]{1,31}:[^<>\x00-\x20]*|[A-Za-z0-9.!#$%&'*+/=?^_` + "`" + `{|}~-]+@[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*)>`)
)

// spanParser parses bracketed spans. It runs before the link parser: at a
// '[' whose matching ']' is directly followed by an attribute set it opens
// a span, and at that ']' it closes it, the way links handle their labels.
// Every other bracket is left to the link parser.
type spanParser struct{}

func (spanParser) Trigger() []byte { return []byte{'[', ']'} }

func (spanParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, seg := block.PeekLine()
	stack, _ := pc.Get(spanStackKey).([]*spanState)
	if line[0] == '[' {
		closeAt, ok := bracketTable(parent, block.Source(), pc)[seg.Start]
		if !ok {
			return nil
		}
		st := &spanState{close: closeAt}
		if d := pc.LastDelimiter(); d != nil {
			st.bottom = d
		}
		pc.Set(spanStackKey, append(stack, st))
		block.Advance(1)
		return st
	}
	if len(stack) == 0 || stack[len(stack)-1].close != seg.Start {
		return nil
	}
	st := stack[len(stack)-1]
	pc.Set(spanStackKey, stack[:len(stack)-1])
	block.Advance(1)
	rest, _ := block.PeekLine()
	attrs, n, ok := scanAttrSet(rest)
	if !ok {
		return &errInline{reason: "malformed span"}
	}
	block.Advance(n)
	parser.ProcessDelimiters(st.bottom, pc)
	span := &spanNode{attrs: attrs}
	span.SetPos(st.Pos())
	p := st.Parent()
	for c := st.NextSibling(); c != nil; {
		next := c.NextSibling()
		p.RemoveChild(p, c)
		span.AppendChild(span, c)
		c = next
	}
	p.RemoveChild(p, st)
	return span
}

// CloseBlock turns any span left open into an error.
func (spanParser) CloseBlock(parent ast.Node, block text.Reader, pc parser.Context) {
	stack, _ := pc.Get(spanStackKey).([]*spanState)
	for _, st := range stack {
		if p := st.Parent(); p != nil {
			p.ReplaceChild(p, st, &errInline{reason: "unclosed span"})
		}
	}
	pc.Set(spanStackKey, nil)
	pc.Set(bracketsKey, nil)
}

type bracketCache struct {
	node ast.Node
	m    map[int]int
}

// bracketTable maps the source offset of every '[' of parent's text that
// opens a span to the offset of its ']'. It is computed once per block.
// Brackets match like nested parentheses; escapes, code spans, autolinks
// and attribute sets are skipped as the inline parsers skip them.
func bracketTable(parent ast.Node, src []byte, pc parser.Context) map[int]int {
	if c, ok := pc.Get(bracketsKey).(*bracketCache); ok && c.node == parent {
		return c.m
	}
	var buf []byte
	var offs []int
	lines := parent.Lines()
	for i := range lines.Len() {
		s := lines.At(i)
		for p := s.Start; p < s.Stop; p++ {
			buf = append(buf, src[p])
			offs = append(offs, p)
		}
	}
	m := map[int]int{}
	type open struct {
		at       int
		eligible bool
	}
	var stack []open
	bang := -2
	for i := 0; i < len(buf); {
		c := buf[i]
		switch {
		case c == '\\' && i+1 < len(buf) && isASCIIPunct(buf[i+1]):
			i += 2
		case c == '`':
			n := 0
			for i+n < len(buf) && buf[i+n] == '`' {
				n++
			}
			i += n
			for j := i; j < len(buf); {
				if buf[j] != '`' {
					j++
					continue
				}
				k := 0
				for j+k < len(buf) && buf[j+k] == '`' {
					k++
				}
				if k == n {
					i = j + k
					break
				}
				j += k
			}
		case c == '!':
			bang = i
			i++
		case c == '[':
			stack = append(stack, open{i, bang != i-1})
			i++
		case c == ']':
			if len(stack) > 0 {
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if _, n, ok := scanAttrSet(buf[i+1:]); o.eligible && ok {
					m[offs[o.at]] = offs[i]
					i += 1 + n
					continue
				}
			}
			i++
		case c == '{':
			if _, n, ok := scanAttrSet(buf[i:]); ok {
				i += n
				continue
			}
			i++
		case c == '<':
			if loc := autolinkPrefix.FindIndex(buf[i:]); loc != nil {
				i += loc[1]
				continue
			}
			i++
		default:
			i++
		}
	}
	pc.Set(bracketsKey, &bracketCache{node: parent, m: m})
	return m
}

// attrSetParser parses an attribute set in inline text. An unescaped '{'
// that doesn't start one is an error.
type attrSetParser struct{}

func (attrSetParser) Trigger() []byte { return []byte{'{'} }

func (attrSetParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	line, _ := block.PeekLine()
	attrs, n, ok := scanAttrSet(line)
	if !ok {
		block.Advance(1)
		return &errInline{reason: `unescaped "{" (write \{ for a literal brace)`}
	}
	block.Advance(n)
	return &attrSetNode{attrs: attrs}
}

// ---- Converting inline nodes to runs ----

// marks are the marks in effect while converting inline nodes.
type marks struct {
	link                                            opt.Optional[string]
	bold, italic, strike, underline, sub, sup, code bool
	color, highlight                                opt.Optional[string]
	marked                                          bool // inside any mark
}

const maxInlineDepth = 64

// runs converts the inline children of parent.
func (c *conv) runs(parent ast.Node) (blocks.Runs, error) {
	var out blocks.Runs
	if err := c.inline(parent, marks{}, &out, 0); err != nil {
		return nil, err
	}
	return mergeRuns(out), nil
}

func (c *conv) inline(n ast.Node, m marks, out *blocks.Runs, depth int) error {
	if depth > maxInlineDepth {
		return c.errf(nodePos(n), "inline marks nested deeper than %d levels", maxInlineDepth)
	}
	for ch := n.FirstChild(); ch != nil; ch = ch.NextSibling() {
		mm := m
		mm.marked = true
		switch v := ch.(type) {
		case *ast.Text:
			raw := v.Segment.Value(c.src)
			if err := c.footnoteCheck(v); err != nil {
				return err
			}
			s := string(raw)
			if !v.IsRaw() {
				s = decodeText(raw)
			}
			addText(out, s, m)
			if v.SoftLineBreak() {
				addText(out, " ", m)
			}
			if v.HardLineBreak() {
				*out = append(*out, &blocks.Break{})
			}
		case *ast.CodeSpan:
			var b strings.Builder
			for t := v.FirstChild(); t != nil; t = t.NextSibling() {
				tx, ok := t.(*ast.Text)
				if !ok {
					return c.errf(nodePos(v), "unexpected content in a code span")
				}
				b.Write(tx.Segment.Value(c.src))
			}
			mm = m
			mm.code = true
			addText(out, strings.ReplaceAll(b.String(), "\n", " "), mm)
		case *ast.Emphasis:
			if v.Level == 2 {
				mm.bold = true
			} else {
				mm.italic = true
			}
			if err := c.inline(v, mm, out, depth+1); err != nil {
				return err
			}
		case *east.Strikethrough:
			mm.strike = true
			if err := c.inline(v, mm, out, depth+1); err != nil {
				return err
			}
		case *ast.Link:
			if m.link.IsSet() {
				return c.errf(nodePos(v), "a link inside a link")
			}
			if len(v.Title) > 0 {
				return c.errf(nodePos(v), "link titles are not supported")
			}
			mm.link = opt.Some(decodeText(v.Destination))
			if err := c.inline(v, mm, out, depth+1); err != nil {
				return err
			}
		case *ast.AutoLink:
			if m.link.IsSet() {
				return c.errf(nodePos(v), "a link inside a link")
			}
			u := string(v.URL(c.src))
			if v.AutoLinkType == ast.AutoLinkEmail {
				u = "mailto:" + u
			}
			mm.link = opt.Some(u)
			addText(out, string(v.Label(c.src)), mm)
		case *ast.Image:
			return c.errf(nodePos(v), "an image must be alone in its paragraph (images can't be inline, in a heading or in a table cell)")
		case *ast.RawHTML:
			return c.errf(nodePos(v), "raw HTML is not supported")
		case *spanNode:
			if _, ok := findAttr(v.attrs, "mention"); ok {
				if m.marked {
					return c.errf(nodePos(v), "a mention can't be inside a link or other marks; give it color= or highlight= itself")
				}
				r, err := c.mention(v)
				if err != nil {
					return err
				}
				*out = append(*out, r)
				continue
			}
			if err := c.spanMarks(v, &mm); err != nil {
				return err
			}
			if err := c.inline(v, mm, out, depth+1); err != nil {
				return err
			}
		case *attrSetNode:
			return c.errf(nodePos(v), "an attribute set isn't allowed here")
		case *errInline:
			return c.errf(nodePos(v), "%s", v.reason)
		case *east.TaskCheckBox:
			return c.errf(nodePos(v), "a task checkbox must start a list item")
		default:
			return c.errf(nodePos(v), "unsupported Markdown (%s)", v.Kind())
		}
	}
	return nil
}

func findAttr(list []attr, name string) (attr, bool) {
	for _, a := range list {
		if a.name == name {
			return a, true
		}
	}
	return attr{}, false
}

func addText(out *blocks.Runs, s string, m marks) {
	if s == "" {
		return
	}
	*out = append(*out, &blocks.Text{
		Text: s, Bold: m.bold, Italic: m.italic, Underline: m.underline, Strike: m.strike,
		Code: m.code, Link: m.link, Sub: m.sub, Sup: m.sup, Color: m.color, Highlight: m.highlight,
	})
}

// mergeRuns merges neighbouring text runs with the same marks, as the
// server does.
func mergeRuns(rs blocks.Runs) blocks.Runs {
	out := blocks.Runs{}
	for i := 0; i < len(rs); {
		t, ok := rs[i].(*blocks.Text)
		if !ok {
			out = append(out, rs[i])
			i++
			continue
		}
		j := i + 1
		for j < len(rs) {
			if u, ok := rs[j].(*blocks.Text); !ok || !sameMarks(t, u) {
				break
			}
			j++
		}
		if j > i+1 {
			var b strings.Builder
			for _, r := range rs[i:j] {
				b.WriteString(r.(*blocks.Text).Text)
			}
			q := *t
			q.Text = b.String()
			t = &q
		}
		out = append(out, t)
		i = j
	}
	return out
}

func (c *conv) spanMarks(v *spanNode, m *marks) error {
	a, err := c.attrReader(v.attrs, nodePos(v), "span")
	if err != nil {
		return err
	}
	for _, f := range []struct {
		name string
		dst  *bool
	}{{"bold", &m.bold}, {"italic", &m.italic}, {"strike", &m.strike}, {"underline", &m.underline}, {"sub", &m.sub}, {"sup", &m.sup}} {
		b, err := a.boolean(f.name)
		if err != nil {
			return err
		}
		if b.Or(false) {
			*f.dst = true
		}
	}
	for _, f := range []struct {
		name string
		dst  *opt.Optional[string]
	}{{"color", &m.color}, {"highlight", &m.highlight}} {
		s, err := a.str(f.name)
		if err != nil {
			return err
		}
		if s.IsSet() {
			*f.dst = s
		}
	}
	return a.done()
}

func (c *conv) mention(v *spanNode) (blocks.Run, error) {
	pos := nodePos(v)
	var b strings.Builder
	for ch := v.FirstChild(); ch != nil; ch = ch.NextSibling() {
		t, ok := ch.(*ast.Text)
		if !ok || t.HardLineBreak() {
			return nil, c.errf(pos, "a mention's text must be plain text")
		}
		if err := c.footnoteCheck(t); err != nil {
			return nil, err
		}
		b.WriteString(decodeText(t.Segment.Value(c.src)))
		if t.SoftLineBreak() {
			b.WriteByte(' ')
		}
	}
	text := b.String()
	a, err := c.attrReader(v.attrs, pos, "mention")
	if err != nil {
		return nil, err
	}
	m := &blocks.Mention{}
	if m.ResourceID, err = a.reqStr("mention"); err != nil {
		return nil, err
	}
	if m.DocumentID, err = a.str("doc"); err != nil {
		return nil, err
	}
	label, err := a.boolean("label")
	if err != nil {
		return nil, err
	}
	name, err := a.str("name")
	if err != nil {
		return nil, err
	}
	switch {
	case label.Or(false):
		m.Label, m.Name = opt.Some(text), name
	case name.IsSet():
		if text != "" {
			return nil, c.errf(pos, "mention: name= is only for a labelled mention, or one with empty text")
		}
		m.Name = name
	case text != "":
		// The server ignores an echoed name (D-009, A-24); setting it keeps
		// the rendering of a parsed mention equal to the base mention's.
		m.Name = opt.Some(text)
	}
	if m.Color, err = a.str("color"); err != nil {
		return nil, err
	}
	if m.Highlight, err = a.str("highlight"); err != nil {
		return nil, err
	}
	return m, a.done()
}

// footnoteCheck rejects a footnote reference "[^" (M-SAFE-03).
func (c *conv) footnoteCheck(t *ast.Text) error {
	if t.IsRaw() {
		return nil
	}
	for p := t.Segment.Start; p < t.Segment.Stop && p+1 < len(c.src); p++ {
		if c.src[p] != '[' || c.src[p+1] != '^' {
			continue
		}
		bs := 0
		for q := p - 1; q >= 0 && c.src[q] == '\\'; q-- {
			bs++
		}
		if bs%2 == 0 {
			return c.errf(p, "footnotes are not supported")
		}
	}
	return nil
}

// takeTrailingAttr removes an attribute set ending parent's inline content,
// with the one space before it.
func (c *conv) takeTrailingAttr(parent ast.Node) (*attrSetNode, error) {
	last, ok := parent.LastChild().(*attrSetNode)
	if !ok {
		return nil, nil
	}
	if prev := last.PreviousSibling(); prev != nil {
		t, ok := prev.(*ast.Text)
		if !ok || t.Segment.Len() == 0 || c.src[t.Segment.Stop-1] != ' ' || t.SoftLineBreak() || t.HardLineBreak() {
			return nil, c.errf(nodePos(last), "write one space before the attribute set")
		}
		t.Segment = t.Segment.WithStop(t.Segment.Stop - 1)
	}
	parent.RemoveChild(parent, last)
	return last, nil
}

// takeLeadingAttr removes an attribute set starting parent's inline content,
// with the one space after it.
func (c *conv) takeLeadingAttr(parent ast.Node) (*attrSetNode, error) {
	first, ok := parent.FirstChild().(*attrSetNode)
	if !ok {
		return nil, nil
	}
	if next := first.NextSibling(); next != nil {
		t, ok := next.(*ast.Text)
		if !ok || t.Segment.Len() == 0 || c.src[t.Segment.Start] != ' ' {
			return nil, c.errf(nodePos(first), "write one space after the attribute set")
		}
		t.Segment = t.Segment.WithStart(t.Segment.Start + 1)
	}
	parent.RemoveChild(parent, first)
	return first, nil
}
