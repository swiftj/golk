package markdown

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/opt"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

const (
	maxInput = 1 << 20
	maxDepth = 64
)

// parse parses src into blocks (LK_MARKDOWN.md §2, §5). Ids are set only
// where the input writes them: {#id} and ::unsupported id=….
func parse(src string) (blocks.List, error) {
	if len(src) > maxInput {
		return nil, &SyntaxError{Reason: "input over 1 MiB"}
	}
	return parseDoc(src)
}

// parseDoc is parse without the size limit, which the renderer's own checks
// use: a large page still renders losslessly.
func parseDoc(src string) (blocks.List, error) { return parseIn(src, 0) }

// parseIn is parseDoc for Markdown that sits inside secrets levels of
// secret, which the renderer's checks of a secret's descendants use.
func parseIn(src string, secrets int) (blocks.List, error) {
	if !utf8.ValidString(src) {
		return nil, &SyntaxError{Reason: "input is not valid UTF-8"}
	}
	src = strings.ReplaceAll(src, "\r\n", "\n")
	c := newConv([]byte(src), 0)
	c.secrets = secrets
	if i := strings.IndexAny(src, "\r\x00"); i >= 0 {
		return nil, c.errf(i, "carriage return or NUL character")
	}
	doc, err := c.goldmark()
	if err != nil {
		return nil, err
	}
	l, err := c.blockList(doc, 1)
	if err != nil {
		return nil, err
	}
	if err := c.strayMentions(src); err != nil {
		return nil, err
	}
	return l, nil
}

// secretMention matches "::secret" in any case: every spelling of an
// attempt to open a secret. ":: secret" with a space is ordinary text ("Key
// :: Secret society" in a row); at the start of a line it is a mistyped
// fence, which fenceLike rejects.
var secretMention = regexp.MustCompile(`(?i)::secret`)

// strayMentions fails unless every secretMention in src opened a secret
// block. A secret typed wrongly, or in a place where Markdown reads it as
// text (a lazy continuation line, a table row, code), would otherwise put
// the DM's text in front of players. Rather than recognise each such place,
// the converter accepts "::secret" only where it made a secret block
// (M-SAFE-02). The renderer writes text containing it as lk-raw, whose JSON
// escapes "::".
func (c *conv) strayMentions(src string) error {
	left := maps.Clone(c.secretLines)
	for _, m := range secretMention.FindAllStringIndex(src, -1) {
		if ln := c.line(m[0]); left[ln] > 0 {
			left[ln]--
			continue
		}
		return c.errf(m[0], "%q here doesn't open a secret block: open one with \":::secret\" on its own line, or escape the colons", src[m[0]:m[1]])
	}
	return nil
}

// conv converts a goldmark document into blocks.
type conv struct {
	src      []byte
	nl       []int // offsets of '\n'
	lineBase int   // added to line numbers, for a sub-parse
	secrets  int   // how many secrets enclose the block being converted
	// secretLines counts the :::secret directives opened on each line.
	secretLines map[int]int
}

func newConv(src []byte, lineBase int) *conv {
	c := &conv{src: src, lineBase: lineBase}
	for i, b := range src {
		if b == '\n' {
			c.nl = append(c.nl, i)
		}
	}
	return c
}

func (c *conv) goldmark() (ast.Node, error) {
	pc := parser.NewContext()
	doc := gm().Parse(text.NewReader(c.src), parser.WithContext(pc))
	if pos, ok := pc.Get(depthErrKey).(int); ok {
		return nil, c.errf(pos, "nested deeper than %d levels", maxDepth)
	}
	if pos, ok := pc.Get(markErrKey).(int); ok {
		return nil, c.errf(pos, "more than %d [, *, _ and ~ in one paragraph or heading; split it", maxMarks)
	}
	if pos, ok := pc.Get(refErrKey).(int); ok {
		return nil, c.errf(pos, "link reference definitions and footnotes are not supported")
	}
	return doc, nil
}

func (c *conv) line(off int) int {
	if off < 0 {
		return 0
	}
	return sort.SearchInts(c.nl, off) + 1 + c.lineBase
}

func (c *conv) errf(off int, format string, args ...any) error {
	return &SyntaxError{Line: c.line(off), Reason: fmt.Sprintf(format, args...)}
}

// nodePos is a source offset inside n, for error lines.
func nodePos(n ast.Node) int {
	for ; n != nil; n = n.FirstChild() {
		if t, ok := n.(*ast.Text); ok {
			return t.Segment.Start
		}
		if n.Type() == ast.TypeBlock && n.Lines().Len() > 0 {
			return n.Lines().At(0).Start
		}
		if p := n.Pos(); p >= 0 {
			return p
		}
	}
	return -1
}

func depthErr(c *conv, n ast.Node) error {
	return c.errf(nodePos(n), "nested deeper than %d levels", maxDepth)
}

// blockList converts the child blocks of parent, at depth.
func (c *conv) blockList(parent ast.Node, depth int) (blocks.List, error) {
	out := blocks.List{}
	var target blocks.Block // the block an attribute line may apply to
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if al, ok := n.(*attrLineNode); ok {
			if target == nil || c.blankBefore(al.Pos()) {
				return nil, c.errf(al.Pos(), "an attribute line must directly follow a paragraph, quote, list, code block, rule or table (image attributes go right after the image)")
			}
			if err := c.lineAttrs(target, al); err != nil {
				return nil, err
			}
			target = nil
			continue
		}
		b, attrable, err := c.block(n, depth)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
		target = nil
		if attrable {
			target = b
		}
	}
	return out, nil
}

// blankBefore reports whether the line before the one at off is blank,
// ignoring quote markers and indentation. (goldmark's blank-line flag is
// also set after a closed fence, so it can't be used.)
func (c *conv) blankBefore(off int) bool {
	start := bytes.LastIndexByte(c.src[:max(off, 0)], '\n')
	if start < 0 {
		return true
	}
	prev := bytes.LastIndexByte(c.src[:start], '\n') + 1
	return len(bytes.Trim(c.src[prev:start], " \t>")) == 0
}

// lineAttrs applies an attribute line to the block above it.
func (c *conv) lineAttrs(b blocks.Block, al *attrLineNode) error {
	a, err := c.attrReader(al.attrs, al.Pos(), "attribute line")
	if err != nil {
		return err
	}
	id := a.id()
	switch v := b.(type) {
	case *blocks.ParagraphBlock:
		v.ID = id
		if v.Align, err = readEnum(a, "align", aligns); err != nil {
			return err
		}
		if v.Background, err = a.str("background"); err != nil {
			return err
		}
	case *blocks.QuoteBlock:
		v.ID = id
		if v.Color, err = a.str("color"); err != nil {
			return err
		}
	case *blocks.CodeBlock:
		v.ID = id
	case *blocks.RuleBlock:
		v.ID = id
	case *blocks.BulletListBlock:
		v.ID = id
	case *blocks.OrderedListBlock:
		v.ID = id
	case *blocks.TaskListBlock:
		v.ID = id
	case *blocks.TableBlock:
		v.ID = id
	}
	return a.done()
}

// block converts one block node. attrable reports whether an attribute line
// may follow it.
func (c *conv) block(n ast.Node, depth int) (b blocks.Block, attrable bool, err error) {
	if depth > maxDepth {
		return nil, false, depthErr(c, n)
	}
	switch v := n.(type) {
	case *ast.Paragraph, *ast.TextBlock:
		return c.paragraph(v)
	case *ast.Heading:
		b, err := c.heading(v)
		return b, false, err
	case *ast.Blockquote:
		kids, err := c.blockList(v, depth+1)
		if err != nil {
			return nil, false, err
		}
		return &blocks.QuoteBlock{Children: kids}, true, nil
	case *ast.FencedCodeBlock:
		return c.fence(v)
	case *ast.ThematicBreak:
		return &blocks.RuleBlock{}, true, nil
	case *ast.List:
		b, err := c.list(v, depth)
		return b, err == nil, err
	case *east.Table:
		b, err := c.gfmTable(v, depth)
		return b, err == nil, err
	case *directiveNode:
		b, err := c.directive(v, depth)
		return b, false, err
	case *ast.CodeBlock:
		return nil, false, c.errf(nodePos(v), "indented code blocks are not supported: use a ``` fence, and don't indent text 4 or more spaces")
	case *ast.HTMLBlock:
		return nil, false, c.errf(nodePos(v), "raw HTML is not supported")
	}
	return nil, false, c.errf(nodePos(n), "unsupported Markdown (%s)", n.Kind())
}

// checkLines rejects a directive fence that ended up as paragraph text, as
// a lazy continuation line or with too much indentation, so a :::secret can
// never become visible text (M-SAFE-02).
func (c *conv) checkLines(n ast.Node) error { return c.fenceLines(n, false) }

// fenceLines is checkLines; for a row (row), whose first line may be an
// empty key's ":: value", only "::name" counts there.
func (c *conv) fenceLines(n ast.Node, row bool) error {
	lines := n.Lines()
	for i := range lines.Len() {
		s := lines.At(i)
		if fenceLike(c.src[s.Start:s.Stop], row && i == 0) {
			return c.fenceErr(s.Start)
		}
	}
	return nil
}

func (c *conv) fenceErr(off int) error {
	return c.errf(off, "a directive fence must start its own line, indented at most 3 spaces (escape a literal one as \\:)")
}

// fenceLike reports whether a line of text looks like a directive fence:
// ":::", or "::" then a name. With rowStart (the first line of a key-value
// row, where ":: value" is an empty key) the name must follow directly.
func fenceLike(line []byte, rowStart bool) bool {
	v := util.TrimLeftSpace(line)
	if bytes.HasPrefix(v, []byte(":::")) {
		return true
	}
	if !bytes.HasPrefix(v, []byte("::")) {
		return false
	}
	after := v[2:]
	if !rowStart {
		after = util.TrimLeftSpace(after)
	}
	return startsName(after)
}

// startsName reports whether b starts with a directive name. "::" then a
// name, even after spaces (":: secret"), is a mistyped directive fence, not
// text, so a mistyped secret can't become visible text (M-SAFE-02).
func startsName(b []byte) bool { return len(b) > 0 && isNameStart(b[0]) }

func (c *conv) paragraph(n ast.Node) (blocks.Block, bool, error) {
	if err := c.checkLines(n); err != nil {
		return nil, false, err
	}
	if im, attrs, ok := imageParagraph(n); ok {
		b, err := c.imageBlock(im, attrs)
		return b, false, err
	}
	runs, err := c.runs(n)
	if err != nil {
		return nil, false, err
	}
	return &blocks.ParagraphBlock{Content: runs}, true, nil
}

// imageParagraph reports whether n is exactly an image, optionally followed
// by its attribute set.
func imageParagraph(n ast.Node) (*ast.Image, *attrSetNode, bool) {
	im, ok := n.FirstChild().(*ast.Image)
	if !ok {
		return nil, nil, false
	}
	switch next := im.NextSibling().(type) {
	case nil:
		return im, nil, true
	case *attrSetNode:
		if next.NextSibling() == nil {
			return im, next, true
		}
	}
	return nil, nil, false
}

// image reads an image's url and alt text.
func (c *conv) image(im *ast.Image) (url, alt string, err error) {
	if len(im.Title) > 0 {
		return "", "", c.errf(nodePos(im), "image titles are not supported; use the title attribute")
	}
	var b strings.Builder
	for ch := im.FirstChild(); ch != nil; ch = ch.NextSibling() {
		t, ok := ch.(*ast.Text)
		if !ok || t.HardLineBreak() {
			return "", "", c.errf(nodePos(im), "image alt text must be plain text")
		}
		b.WriteString(decodeText(t.Segment.Value(c.src)))
		if t.SoftLineBreak() {
			b.WriteByte(' ')
		}
	}
	return decodeText(im.Destination), b.String(), nil
}

func (c *conv) imageAttrs(im *ast.Image, set *attrSetNode, what string) (*attrR, string, opt.Optional[string], error) {
	url, alt, err := c.image(im)
	if err != nil {
		return nil, "", opt.Optional[string]{}, err
	}
	var list []attr
	pos := nodePos(im)
	if set != nil {
		list, pos = set.attrs, set.Pos()
	}
	a, err := c.attrReader(list, pos, what)
	if err != nil {
		return nil, "", opt.Optional[string]{}, err
	}
	altAttr, err := a.str("alt")
	if err != nil {
		return nil, "", opt.Optional[string]{}, err
	}
	o := altAttr
	if alt != "" {
		if altAttr.IsSet() {
			return nil, "", opt.Optional[string]{}, a.errf("alt text given twice")
		}
		o = opt.Some(alt)
	}
	return a, url, o, nil
}

func (c *conv) imageBlock(im *ast.Image, set *attrSetNode) (blocks.Block, error) {
	a, url, alt, err := c.imageAttrs(im, set, "image")
	if err != nil {
		return nil, err
	}
	b := &blocks.ImageBlock{ID: a.id(), URL: url, Alt: alt}
	if b.Fill, err = a.boolean("fill"); err != nil {
		return nil, err
	}
	if b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a); err != nil {
		return nil, err
	}
	return b, a.done()
}

// titled reads the title, isTitleHidden, layout and width attributes.
func titled(a *attrR) (title opt.Optional[string], hidden opt.Optional[bool], layout opt.Optional[blocks.BlockLayout], width opt.Optional[int], err error) {
	if title, err = a.str("title"); err != nil {
		return
	}
	if hidden, err = a.boolean("isTitleHidden"); err != nil {
		return
	}
	if layout, err = readEnum(a, "layout", layouts); err != nil {
		return
	}
	width, err = a.integer("width")
	return
}

func placed(a *attrR) (layout opt.Optional[blocks.BlockLayout], width opt.Optional[int], err error) {
	if layout, err = readEnum(a, "layout", layouts); err != nil {
		return
	}
	width, err = a.integer("width")
	return
}

func (c *conv) heading(h *ast.Heading) (blocks.Block, error) {
	if err := c.checkLines(h); err != nil {
		return nil, err
	}
	set, err := c.takeTrailingAttr(h)
	if err != nil {
		return nil, err
	}
	runs, err := c.runs(h)
	if err != nil {
		return nil, err
	}
	b := &blocks.HeadingBlock{Level: h.Level, Content: runs}
	if set == nil {
		return b, nil
	}
	a, err := c.attrReader(set.attrs, set.Pos(), "heading")
	if err != nil {
		return nil, err
	}
	b.ID = a.id()
	if b.Align, err = readEnum(a, "align", aligns); err != nil {
		return nil, err
	}
	if b.Background, err = a.str("background"); err != nil {
		return nil, err
	}
	return b, a.done()
}

// fenceText is the content of a fenced code block.
func (c *conv) fenceText(f *ast.FencedCodeBlock) (info, body string, err error) {
	if !fenceClosed(f) {
		return "", "", c.errf(f.Pos(), "unclosed code fence")
	}
	if f.Info != nil {
		info = decodeText(f.Info.Segment.Value(c.src))
	}
	var b []byte
	lines := f.Lines()
	for i := range lines.Len() {
		s := lines.At(i)
		b = append(b, s.Value(c.src)...)
	}
	return info, strings.TrimSuffix(string(b), "\n"), nil
}

func (c *conv) fence(f *ast.FencedCodeBlock) (blocks.Block, bool, error) {
	info, body, err := c.fenceText(f)
	if err != nil {
		return nil, false, err
	}
	if w, _, _ := strings.Cut(info, " "); (w == "lk-raw" || w == "lk-filter") && w != info {
		return nil, false, c.errf(f.Pos(), "write the %s info string alone", w)
	}
	switch info {
	case "lk-raw":
		data, err := c.rawJSON(f, body)
		if err != nil {
			return nil, false, err
		}
		b, err := blocks.Unmarshal(data)
		if err != nil {
			return nil, false, c.errf(f.Pos(), "lk-raw: %v", err)
		}
		return b, false, nil
	case "lk-filter":
		return nil, false, c.errf(f.Pos(), "an lk-filter fence belongs at the end of a :::collection")
	}
	if i := secretFenceLine(body); i >= 0 && c.secrets == 0 {
		// A fence left unclosed before a secret (a dropped closing line)
		// would otherwise publish the secret, or the rest of one closed
		// early, as code (M-SAFE-02).
		return nil, false, c.errf(f.Lines().At(i).Start, "a code block outside a secret holds a :::secret or ::: line; close the code fence before it")
	}
	b := &blocks.CodeBlock{Text: body}
	if info != "" {
		b.Language = opt.Some(info)
	}
	return b, true, nil
}

// secretFenceLine is the index of the first line of code text that would
// open a secret outside a fence (three or more colons, then "secret" in any
// case, then the end of the line or a space) or close a directive (three or
// more colons alone), or -1. Quote markers and list markers before the
// colons count too: a fence can swallow "> :::secret" lines of a quoted
// secret. A swallowed closing line matters as much: it would otherwise
// hide the stray closer left by a secret closed early.
func secretFenceLine(text string) int {
	for i, l := range strings.Split(text, "\n") {
		l = strings.TrimLeft(l, " \t>-*+.)0123456789")
		n := len(l) - len(strings.TrimLeft(l, ":"))
		if n < 3 {
			continue
		}
		rest := l[n:]
		if strings.TrimSpace(rest) == "" {
			return i
		}
		if len(rest) >= 6 && strings.EqualFold(rest[:6], "secret") && (len(rest) == 6 || rest[6] == ' ' || rest[6] == '\t') {
			return i
		}
	}
	return -1
}

// rawJSON checks that an lk-raw or lk-filter fence holds one JSON object
// (M-OPQ-02).
func (c *conv) rawJSON(f *ast.FencedCodeBlock, body string) ([]byte, error) {
	data := []byte(strings.TrimSpace(body))
	if len(data) == 0 || data[0] != '{' || !json.Valid(data) {
		return nil, c.errf(f.Pos(), "an lk-raw or lk-filter fence must hold exactly one JSON object")
	}
	return data, nil
}

func checkbox(item ast.Node) (*east.TaskCheckBox, bool) {
	first := item.FirstChild()
	if first == nil {
		return nil, false
	}
	if _, ok := first.(*ast.TextBlock); !ok {
		if _, ok := first.(*ast.Paragraph); !ok {
			return nil, false
		}
	}
	cb, ok := first.FirstChild().(*east.TaskCheckBox)
	return cb, ok
}

func (c *conv) list(l *ast.List, depth int) (blocks.Block, error) {
	tasks, plain := 0, 0
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		if _, ok := checkbox(it); ok {
			tasks++
		} else {
			plain++
		}
	}
	if tasks > 0 {
		if plain > 0 {
			return nil, c.errf(nodePos(l), "a list mixes task items and plain items")
		}
		if l.IsOrdered() {
			return nil, c.errf(nodePos(l), "a task list can't be numbered")
		}
		return c.taskList(l, depth)
	}
	if depth+1 > maxDepth {
		return nil, depthErr(c, l)
	}
	items := blocks.List{}
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		li := &blocks.ListItemBlock{}
		if first := it.FirstChild(); first != nil {
			set, err := c.takeLeadingAttr(first)
			if err != nil {
				return nil, err
			}
			if li.ID, err = c.onlyID(set, "list item"); err != nil {
				return nil, err
			}
		}
		kids, err := c.blockList(it, depth+2)
		if err != nil {
			return nil, err
		}
		li.Children = kids
		items = append(items, li)
	}
	if l.IsOrdered() {
		b := &blocks.OrderedListBlock{Children: items}
		if l.Start != 1 {
			b.Start = opt.Some(l.Start)
		}
		return b, nil
	}
	return &blocks.BulletListBlock{Children: items}, nil
}

// onlyID reads an attribute set that may hold only an #id.
func (c *conv) onlyID(set *attrSetNode, what string) (string, error) {
	if set == nil {
		return "", nil
	}
	a, err := c.attrReader(set.attrs, set.Pos(), what)
	if err != nil {
		return "", err
	}
	id := a.id()
	return id, a.done()
}

func (c *conv) taskList(l *ast.List, depth int) (blocks.Block, error) {
	if depth+1 > maxDepth {
		return nil, depthErr(c, l)
	}
	out := blocks.List{}
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		cb, _ := checkbox(it)
		first := it.FirstChild()
		first.RemoveChild(first, cb)
		if err := c.checkLines(first); err != nil {
			return nil, err
		}
		set, err := c.takeLeadingAttr(first)
		if err != nil {
			return nil, err
		}
		id, err := c.onlyID(set, "task item")
		if err != nil {
			return nil, err
		}
		runs, err := c.runs(first)
		if err != nil {
			return nil, err
		}
		out = append(out, &blocks.TaskItemBlock{ID: id, Checked: cb.IsChecked, Content: runs})
		for ch := first.NextSibling(); ch != nil; ch = ch.NextSibling() {
			sub, ok := ch.(*ast.List)
			if !ok {
				return nil, c.errf(nodePos(ch), "a task item holds only its text and nested task lists")
			}
			b, err := c.list(sub, depth+1)
			if err != nil {
				return nil, err
			}
			if _, ok := b.(*blocks.TaskListBlock); !ok {
				return nil, c.errf(nodePos(sub), "a list nested under a task item must be a task list")
			}
			out = append(out, b)
		}
	}
	return &blocks.TaskListBlock{Children: out}, nil
}

func (c *conv) gfmTable(t *east.Table, depth int) (blocks.Block, error) {
	if depth+3 > maxDepth {
		return nil, depthErr(c, t)
	}
	for _, a := range t.Alignments {
		if a != east.AlignNone {
			return nil, c.errf(nodePos(t), "table column alignment (:---) is not supported")
		}
	}
	rows := blocks.List{}
	for r := t.FirstChild(); r != nil; r = r.NextSibling() {
		// A row is a source line of its own; like paragraph text, it can't
		// be a mistyped fence (M-SAFE-02).
		if pos := r.Pos(); pos >= 0 && pos < len(c.src) {
			line := c.src[pos:]
			if end := bytes.IndexByte(line, '\n'); end >= 0 {
				line = line[:end]
			}
			if fenceLike(line, false) {
				return nil, c.fenceErr(pos)
			}
		}
		_, header := r.(*east.TableHeader)
		if !header {
			if n := c.rowCells(r.Pos()); n != len(t.Alignments) {
				return nil, c.errf(r.Pos(), "a table row has %d cells; the header has %d", n, len(t.Alignments))
			}
		}
		cells := blocks.List{}
		for cell := r.FirstChild(); cell != nil; cell = cell.NextSibling() {
			runs, err := c.runs(cell)
			if err != nil {
				return nil, err
			}
			tc := &blocks.TableCellBlock{Children: blocks.List{&blocks.ParagraphBlock{Content: runs}}}
			if header {
				tc.Header = opt.Some(true)
			}
			cells = append(cells, tc)
		}
		rows = append(rows, &blocks.TableRowBlock{Children: cells})
	}
	return &blocks.TableBlock{Children: rows}, nil
}

// rowCells counts the cells of the table row whose line starts at pos, the
// way GFM splits it, so a row with extra cells is an error rather than
// silently cut short.
func (c *conv) rowCells(pos int) int {
	if pos < 0 || pos >= len(c.src) {
		return -1
	}
	end := bytes.IndexByte(c.src[pos:], '\n')
	line := c.src[pos:]
	if end >= 0 {
		line = line[:end]
	}
	line = util.TrimRightSpace(util.TrimLeftSpace(line))
	start, limit := 0, len(line)
	if limit > 0 && line[0] == '|' {
		start++
	}
	if limit > start && line[limit-1] == '|' {
		limit--
	}
	n := 0
	for p := start; p < limit; {
		n++
		q := p
		for q < limit && (line[q] != '|' || q > 0 && line[q-1] == '\\') {
			q++
		}
		p = q + 1
	}
	return n
}
