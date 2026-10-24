package markdown

import (
	"bytes"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/opt"
)

// codec is the rendering the base matcher uses.
var codec = hooks{key: renderKey, opaque: rendersOpaque}

// How rendering stays lossless (M-OPQ-01): every block is rendered in its
// Markdown form only when explicit rules say the form can hold it, and the
// result is then parsed back and compared with the block (apart from hidden
// fields). A block that fails is retried with "safe" inline marks (span
// flags instead of ** * ~~, which have no flanking rules), and failing that
// is written as an lk-raw fence holding its exact JSON. The check runs for
// every block that can stand alone, so one odd paragraph inside a secret
// becomes lk-raw without the rest of the secret. The whole page is checked
// last; a top-level block that misparses in context (it never has, but the
// check costs one parse) is written as lk-raw, and if that does not settle
// it every top-level block is.

// render renders page (LK_MARKDOWN.md §2).
func render(page blocks.List, opts ToOptions) (string, error) {
	r := newRenderer(opts.IDs)
	forced := map[int]bool{}
	for {
		lines, spans := r.seq(page, forced, false)
		if r.err != nil {
			return "", r.err
		}
		out := joinLines(lines)
		if len(forced) == len(page) {
			return out, nil // every block is lk-raw, which is exact
		}
		parsed, err := parseDoc(out)
		if err == nil && sameShown(parsed, page) {
			return out, nil
		}
		i := firstBad(parsed, err, page, spans)
		if i < 0 || forced[i] {
			for j := range page {
				forced[j] = true
			}
			continue
		}
		forced[i] = true
	}
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// firstBad is the index of the first top-level block the page check
// disagrees on, or -1.
func firstBad(parsed blocks.List, err error, page blocks.List, spans [][2]int) int {
	var se *SyntaxError
	if errors.As(err, &se) {
		for i, s := range spans {
			if se.Line >= s[0] && se.Line <= s[1] {
				return i
			}
		}
		return -1
	}
	for i := range page {
		if i >= len(parsed) || !sameShown(parsed[i], page[i]) {
			return i
		}
	}
	return -1
}

// renderKey is the canonical rendering of one node without ids: a block as
// it renders on its own, an infobox row as it renders inside an infobox.
func renderKey(n blocks.Node) string {
	r := newRenderer(false)
	rd := r.node(n)
	if r.err != nil {
		return "\x00error: " + r.err.Error()
	}
	if rd == nil {
		return ""
	}
	return strings.Join(rd.lines, "\n")
}

// rendersOpaque reports whether n renders as ::unsupported or lk-raw.
func rendersOpaque(n blocks.Node) bool {
	r := newRenderer(false)
	rd := r.node(n)
	return rd != nil && rd.opaque
}

type renderer struct {
	ids  bool
	memo map[blocks.Node]*rendered
	err  error
}

// rendered is one node's Markdown.
type rendered struct {
	lines  []string
	opaque bool // ::unsupported or lk-raw
	safe   bool // inline marks rendered as span flags
}

// ropt are the options of one block rendering.
type ropt struct {
	safe   bool // span flags for bold, italic and strike
	alt    bool // the alternate list marker, after a list of the same family
	nested bool // a task list nested under an item: no attribute line
}

func newRenderer(ids bool) *renderer {
	return &renderer{ids: ids, memo: map[blocks.Node]*rendered{}}
}

func (r *renderer) node(n blocks.Node) *rendered {
	switch v := n.(type) {
	case blocks.Block:
		return r.checked(v)
	case blocks.Property:
		return r.checkedProp(v)
	}
	return nil
}

// seq renders sibling blocks, a blank line apart. A list after a list of the
// same family takes the other marker, so the two don't merge. Inside a list
// item (item), a list directly after the item's paragraph is written on the
// next line. spans holds each block's 1-based line range.
func (r *renderer) seq(list blocks.List, forced map[int]bool, item bool) ([]string, [][2]int) {
	var out []string
	var spans [][2]int
	var prev blocks.Block
	var prevRd *rendered
	prevAlt := false
	for i, b := range list {
		var rd *rendered
		if forced[i] {
			rd = r.raw(b)
		} else {
			rd = r.checked(b)
		}
		lines := rd.lines
		alt := false
		if prev != nil && !rd.opaque && !prevRd.opaque && family(prev) != 0 && family(prev) == family(b) {
			alt = !prevAlt
			if alt {
				if l, ok := r.blockLines(b, ropt{safe: rd.safe, alt: true}); ok {
					lines = l
				} else {
					rd = r.raw(b)
					lines, alt = rd.lines, false
				}
			}
		}
		if i > 0 && (!item || !tight(prev, prevRd, b, rd)) {
			out = append(out, "")
		}
		start := len(out) + 1
		out = append(out, lines...)
		spans = append(spans, [2]int{start, len(out)})
		prev, prevRd, prevAlt = b, rd, alt
	}
	return out, spans
}

func (r *renderer) children(list blocks.List) []string {
	lines, _ := r.seq(list, nil, false)
	return lines
}

// family groups the lists CommonMark would merge when adjacent.
func family(b blocks.Block) int {
	switch b.(type) {
	case *blocks.BulletListBlock, *blocks.TaskListBlock:
		return 1
	case *blocks.OrderedListBlock:
		return 2
	}
	return 0
}

// tight reports whether list b can follow paragraph prev in a list item
// without a blank line: a list interrupts a paragraph only when its first
// item isn't empty and, if numbered, it starts at 1.
func tight(prev blocks.Block, prd *rendered, b blocks.Block, rd *rendered) bool {
	if _, ok := prev.(*blocks.ParagraphBlock); !ok || prd.opaque || rd.opaque {
		return false
	}
	switch v := b.(type) {
	case *blocks.BulletListBlock:
		return len(v.Children) > 0 && hasChildren(v.Children[0])
	case *blocks.OrderedListBlock:
		return !v.Start.IsSet() && len(v.Children) > 0 && hasChildren(v.Children[0])
	case *blocks.TaskListBlock:
		return true
	}
	return false
}

func hasChildren(b blocks.Block) bool {
	l, ok := blocks.Children(b)
	return ok && len(l) > 0
}

// checked renders b in its Markdown form when that parses back to b, and
// as lk-raw otherwise (M-OPQ-01).
func (r *renderer) checked(b blocks.Block) *rendered {
	if rd, ok := r.memo[b]; ok {
		return rd
	}
	rd := r.compute(b)
	r.memo[b] = rd
	return rd
}

func (r *renderer) compute(b blocks.Block) *rendered {
	switch v := b.(type) {
	case *blocks.RawBlock:
		return r.raw(b)
	case *blocks.UnsupportedBlock:
		if !blocks.HasUnmodelled(v) {
			w := newAttrW()
			w.str("id", v.ID)
			if w.ok {
				return &rendered{lines: []string{"::unsupported " + w.String()}, opaque: true}
			}
		}
		return r.raw(b)
	}
	for _, safe := range []bool{false, true} {
		lines, ok := r.blockLines(b, ropt{safe: safe})
		if r.err != nil {
			return &rendered{opaque: true}
		}
		if ok && r.verify(b, lines) {
			return &rendered{lines: lines, safe: safe}
		}
	}
	return r.raw(b)
}

// verify reports whether lines parse back to exactly b, apart from hidden
// fields.
func (r *renderer) verify(b blocks.Block, lines []string) bool {
	parsed, err := parseDoc(joinLines(lines))
	return err == nil && len(parsed) == 1 && sameShown(parsed[0], b)
}

// raw renders v (a block or property) as an lk-raw fence holding its JSON.
func (r *renderer) raw(v any) *rendered {
	data, err := json.Marshal(v)
	if err != nil {
		if r.err == nil {
			r.err = err
		}
		return &rendered{opaque: true}
	}
	if !utf8.Valid(data) {
		// Only JSON this package keeps raw (an unknown kind, an unmodelled
		// field) can hold it; Markdown text can't.
		if r.err == nil {
			r.err = errors.New("markdown: a block's JSON is not valid UTF-8")
		}
		return &rendered{opaque: true}
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		if r.err == nil {
			r.err = err
		}
		return &rendered{opaque: true}
	}
	return &rendered{lines: fenced("lk-raw", buf.String()), opaque: true}
}

// fenced writes body in a backtick fence longer than any backtick run in it.
func fenced(info, body string) []string {
	fence := strings.Repeat("`", max(3, longestRun(body, '`')+1))
	lines := []string{fence + info}
	lines = append(lines, strings.Split(body, "\n")...)
	return append(lines, fence)
}

func longestRun(s string, c byte) int {
	run, longest := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	return longest
}

// prefixed puts p before every line, or bare before an empty one.
func prefixed(lines []string, p, bare string) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		if l == "" {
			out[i] = bare
		} else {
			out[i] = p + l
		}
	}
	return out
}

// directive writes a container directive around body.
func directive(name string, w *attrW, body []string) []string {
	head := ":::" + name
	if s := w.String(); s != "" {
		head += " " + s
	}
	out := append([]string{head}, body...)
	return append(out, ":::")
}

func leaf(name string, w *attrW) []string {
	head := "::" + name
	if s := w.String(); s != "" {
		head += " " + s
	}
	return []string{head}
}

// withAttrLine adds an attribute line {…} under a block, when w has any.
func withAttrLine(lines []string, w *attrW) []string {
	if s := w.String(); s != "" {
		return append(lines, "{"+s+"}")
	}
	return lines
}

// blockLines renders b in its Markdown form. ok is false when that form
// can't hold it.
func (r *renderer) blockLines(b blocks.Block, o ropt) ([]string, bool) {
	if blocks.HasUnmodelled(b) {
		return nil, false
	}
	w := newAttrW()
	w.id(r.ids, b.NodeID())
	var lines []string
	ok := true
	switch v := b.(type) {
	case *blocks.ParagraphBlock:
		optEnum(w, "align", v.Align, aligns)
		w.optStr("background", v.Background)
		if len(v.Content) == 0 {
			return leaf("paragraph", w), w.ok
		}
		var s string
		if s, ok = renderRuns(v.Content, ictx{safe: o.safe}); ok {
			lines = withAttrLine(strings.Split(s, "\n"), w)
		}
	case *blocks.HeadingBlock:
		optEnum(w, "align", v.Align, aligns)
		w.optStr("background", v.Background)
		var s string
		s, ok = renderRuns(v.Content, ictx{safe: o.safe, heading: true})
		line := strings.Repeat("#", v.Level)
		if s != "" {
			line += " " + s
		}
		if a := w.String(); a != "" {
			line += " {" + a + "}"
		}
		lines, ok = []string{line}, ok && v.Level >= 1 && v.Level <= 6
	case *blocks.QuoteBlock:
		w.optStr("color", v.Color)
		lines = prefixed(r.children(v.Children), "> ", ">")
		if len(lines) == 0 {
			lines = []string{">"}
		}
		lines = withAttrLine(lines, w)
	case *blocks.CodeBlock:
		lang, has := v.Language.Get()
		if has && !langOK(lang) || !utf8.ValidString(v.Text) || strings.ContainsAny(v.Text, "\r\x00") {
			return nil, false
		}
		lines = withAttrLine(fenced(lang, v.Text), w)
	case *blocks.RuleBlock:
		lines = withAttrLine([]string{"---"}, w)
	case *blocks.BulletListBlock:
		marker := "-"
		if o.alt {
			marker = "*"
		}
		for _, c := range v.Children {
			li, isItem := c.(*blocks.ListItemBlock)
			if !isItem || blocks.HasUnmodelled(li) {
				return nil, false
			}
			lines = append(lines, r.item(marker, li)...)
		}
		lines = withAttrLine(lines, w)
		ok = len(v.Children) > 0
	case *blocks.OrderedListBlock:
		lines, ok = r.ordered(v, o)
		lines = withAttrLine(lines, w)
	case *blocks.TaskListBlock:
		lines, ok = r.tasks(v, o)
		if !o.nested { // a nested task list's id has nowhere to go
			lines = withAttrLine(lines, w)
		}
	case *blocks.TableBlock:
		lines, ok = r.table(v, o, w)
	case *blocks.CalloutBlock:
		enum(w, "tone", v.Tone, tones)
		w.optStr("icon", v.Icon)
		w.optStr("color", v.Color)
		lines = directive("callout", w, r.children(v.Children))
	case *blocks.ExpandBlock:
		w.str("title", v.Title)
		lines = directive("expand", w, r.children(v.Children))
	case *blocks.LayoutBlock:
		var body []string
		for _, c := range v.Children {
			col, isCol := c.(*blocks.LayoutColumnBlock)
			if !isCol || blocks.HasUnmodelled(col) {
				return nil, false
			}
			cw := newAttrW()
			cw.id(r.ids, col.ID)
			cw.optFloat("width", col.Width)
			cw.optStr("background", col.Background)
			ok = ok && cw.ok
			body = append(body, directive("column", cw, r.children(col.Children))...)
		}
		lines = directive("layout", w, body)
	case *blocks.SecretBlock:
		optEnum(w, "layout", v.Layout, layouts)
		w.optInt("width", v.Width)
		lines, ok = directive("secret", w, r.children(v.Children)), len(v.Children) > 0
	case *blocks.TextFieldBlock:
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		lines = directive("textfield", w, r.children(v.Children))
	case *blocks.ImageBlock:
		var line string
		line, ok = r.image(w, v.URL, v.Alt, func() {
			w.optBool("fill", v.Fill)
			titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		})
		lines = []string{line}
	case *blocks.EmbedBlock:
		enum(w, "provider", v.Provider, providers)
		w.str("url", v.URL)
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		lines = leaf("embed", w)
	case *blocks.KeyValueBlock:
		optEnum(w, "display", v.Display, displays)
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		var body []string
		body, ok = r.kvRows(v.Rows, o)
		lines = directive("keyvalue", w, body)
	case *blocks.MeterBlock:
		meterW(w, v.Mode, v.ShowText, v.ShowIcon, v.ShowMax)
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		var body []string
		body, ok = r.meterItems(v.Items)
		lines = directive("meter", w, body)
	case *blocks.ResourceLinkBlock:
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		var body []string
		body, ok = r.resourceItems(v.Items)
		lines = directive("links", w, body)
	case *blocks.ResourceCollectionBlock:
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		var body []string
		body, ok = r.collectionBody(v.Items, v.Filter)
		lines = directive("collection", w, body)
	case *blocks.MapBlock:
		mapW(w, v.ResourceID, v.DocumentID, v.Viewport, v.DisplayHeight, v.Fill)
		titledW(w, v.Title, v.IsTitleHidden, v.Layout, v.Width)
		lines = leaf("map", w)
	case *blocks.InfoboxBlock:
		optEnum(w, "layout", v.Layout, layouts)
		w.optInt("width", v.Width)
		var body []string
		for i, p := range v.Rows {
			if i > 0 {
				body = append(body, "")
			}
			body = append(body, r.checkedProp(p).lines...)
		}
		lines = directive("infobox", w, body)
	default:
		return nil, false
	}
	return lines, ok && w.ok
}

func langOK(lang string) bool {
	return lang != "" && lang == strings.TrimSpace(lang) && textOK(lang) &&
		!strings.ContainsAny(lang, "`\\&\t") && !strings.HasPrefix(lang, "lk-raw") && !strings.HasPrefix(lang, "lk-filter")
}

func titledW(w *attrW, title opt.Optional[string], hidden opt.Optional[bool], layout opt.Optional[blocks.BlockLayout], width opt.Optional[int]) {
	w.optStr("title", title)
	w.optBool("isTitleHidden", hidden)
	optEnum(w, "layout", layout, layouts)
	w.optInt("width", width)
}

func meterW(w *attrW, mode blocks.MeterMode, text, icon, maxv opt.Optional[bool]) {
	enum(w, "mode", mode, modes)
	w.optBool("showText", text)
	w.optBool("showIcon", icon)
	w.optBool("showMax", maxv)
}

func mapW(w *attrW, rid, did opt.Optional[string], vp *blocks.Viewport, height opt.Optional[float64], fill opt.Optional[bool]) {
	w.optStr("resourceId", rid)
	w.optStr("documentId", did)
	if vp != nil {
		if blocks.HasUnmodelled(vp) || len(vp.Center) != 2 {
			w.ok = false
		} else {
			var parts []string
			for _, f := range []float64{vp.Center[0], vp.Center[1], vp.Zoom} {
				s, ok := fmtFloat(f)
				w.ok = w.ok && ok
				parts = append(parts, s)
			}
			w.str("viewport", strings.Join(parts, ","))
		}
	}
	w.optFloat("displayHeight", height)
	w.optBool("fill", fill)
}

func envW(w *attrW, env *blocks.Envelope) {
	w.optStr("title", env.Title)
	w.optBool("isTitleHidden", env.IsTitleHidden)
	w.optBool("isHidden", env.IsHidden)
}

// image writes ![alt](url){…}. more adds the attributes after the id and
// alt.
func (r *renderer) image(w *attrW, url string, alt opt.Optional[string], more func()) (string, bool) {
	dest, ok := linkDest(url)
	a, hasAlt := alt.Get()
	if hasAlt && a == "" {
		w.str("alt", "")
	}
	more()
	if !textOK(a) {
		return "", false
	}
	line := "![" + escapeText(a, escCtx{}) + "](" + dest + ")"
	if s := w.String(); s != "" {
		line += "{" + s + "}"
	}
	return line, ok
}

// item writes one list item: its children, the first line after the marker
// and the rest indented under it.
func (r *renderer) item(marker string, li *blocks.ListItemBlock) []string {
	if len(li.Children) == 0 {
		return []string{marker}
	}
	body, _ := r.seq(li.Children, nil, true)
	if len(body) == 0 {
		return []string{marker}
	}
	if p, isP := li.Children[0].(*blocks.ParagraphBlock); isP && r.ids && li.ID != "" && len(p.Content) > 0 && !r.checked(p).opaque {
		w := newAttrW()
		w.id(true, li.ID)
		if w.ok {
			body[0] = "{" + w.String() + "} " + body[0]
		}
	}
	return indentUnder(marker, body)
}

func indentUnder(marker string, body []string) []string {
	if len(body) == 0 { // only after an error, which render returns
		return []string{marker}
	}
	out := []string{marker + " " + body[0]}
	return append(out, prefixed(body[1:], strings.Repeat(" ", len(marker)+1), "")...)
}

func (r *renderer) ordered(v *blocks.OrderedListBlock, o ropt) ([]string, bool) {
	start := 1
	if s, ok := v.Start.Get(); ok {
		if s == 1 || s < 0 || s+len(v.Children) > 999999999 {
			return nil, false
		}
		start = s
	}
	delim := "."
	if o.alt {
		delim = ")"
	}
	var lines []string
	for i, c := range v.Children {
		li, ok := c.(*blocks.ListItemBlock)
		if !ok || blocks.HasUnmodelled(li) {
			return nil, false
		}
		lines = append(lines, r.item(strconv.Itoa(start+i)+delim, li)...)
	}
	return lines, len(v.Children) > 0
}

// tasks writes a task list; a nested task list goes under the item before
// it (§2.6).
func (r *renderer) tasks(v *blocks.TaskListBlock, o ropt) ([]string, bool) {
	marker := "-"
	if o.alt {
		marker = "*"
	}
	kids := v.Children
	var lines []string
	for i := 0; i < len(kids); {
		ti, ok := kids[i].(*blocks.TaskItemBlock)
		if !ok || blocks.HasUnmodelled(ti) {
			return nil, false
		}
		i++
		head := "[ ]"
		if ti.Checked {
			head = "[x]"
		}
		if r.ids && ti.ID != "" {
			w := newAttrW()
			w.id(true, ti.ID)
			if !w.ok {
				return nil, false
			}
			head += " {" + w.String() + "}"
		}
		text, ok := renderRuns(ti.Content, ictx{safe: o.safe})
		if !ok {
			return nil, false
		}
		body := strings.Split(text, "\n")
		if text != "" {
			body[0] = head + " " + body[0]
		} else {
			body[0] = head
		}
		for j := 0; i < len(kids); i, j = i+1, j+1 {
			tl, ok := kids[i].(*blocks.TaskListBlock)
			if !ok {
				break
			}
			sub, ok := r.blockLines(tl, ropt{safe: o.safe, alt: j%2 == 1, nested: true})
			if !ok {
				return nil, false
			}
			body = append(body, sub...)
		}
		lines = append(lines, indentUnder(marker, body)...)
	}
	return lines, len(kids) > 0
}

func (r *renderer) table(v *blocks.TableBlock, o ropt, w *attrW) ([]string, bool) {
	if lines, ok := r.gfm(v, o); ok {
		return withAttrLine(lines, w), true
	}
	var body []string
	for _, rb := range v.Children {
		row, ok := rb.(*blocks.TableRowBlock)
		if !ok || blocks.HasUnmodelled(row) {
			return nil, false
		}
		rw := newAttrW()
		rw.id(r.ids, row.ID)
		var cells []string
		for _, cb := range row.Children {
			cell, ok := cb.(*blocks.TableCellBlock)
			if !ok || blocks.HasUnmodelled(cell) {
				return nil, false
			}
			cw := newAttrW()
			cw.id(r.ids, cell.ID)
			cw.optBool("header", cell.Header)
			cw.optInt("colspan", cell.Colspan)
			cw.optInt("rowspan", cell.Rowspan)
			if !cw.ok {
				return nil, false
			}
			cells = append(cells, directive("cell", cw, r.children(cell.Children))...)
		}
		if !rw.ok {
			return nil, false
		}
		body = append(body, directive("row", rw, cells)...)
	}
	return directive("table", w, body), true
}

// gfm writes a table as a GFM pipe table, when it can hold it (§2.5).
func (r *renderer) gfm(v *blocks.TableBlock, o ropt) ([]string, bool) {
	if len(v.Children) == 0 {
		return nil, false
	}
	var lines []string
	width := -1
	for ri, rb := range v.Children {
		row, ok := rb.(*blocks.TableRowBlock)
		if !ok || blocks.HasUnmodelled(row) || r.ids && row.ID != "" {
			return nil, false
		}
		if width < 0 {
			width = len(row.Children)
		}
		if len(row.Children) != width || width == 0 {
			return nil, false
		}
		var cells []string
		for _, cb := range row.Children {
			cell, ok := cb.(*blocks.TableCellBlock)
			if !ok || blocks.HasUnmodelled(cell) || r.ids && cell.ID != "" ||
				cell.Colspan.IsSet() || cell.Rowspan.IsSet() || len(cell.Children) != 1 {
				return nil, false
			}
			if h, set := cell.Header.Get(); ri == 0 && (!set || !h) || ri > 0 && set {
				return nil, false
			}
			p, ok := cell.Children[0].(*blocks.ParagraphBlock)
			if !ok || blocks.HasUnmodelled(p) || p.Align.IsSet() || p.Background.IsSet() || r.ids && p.ID != "" {
				return nil, false
			}
			s, ok := renderRuns(p.Content, ictx{safe: o.safe, cell: true})
			if !ok && len(p.Content) > 0 {
				return nil, false
			}
			cells = append(cells, s)
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
		if ri == 0 {
			lines = append(lines, "|"+strings.Repeat(" --- |", width))
		}
	}
	return lines, true
}

// kvRows writes "- key :: value {…}" rows (M-DIR-03).
func (r *renderer) kvRows(rows blocks.KVRows, o ropt) ([]string, bool) {
	var lines []string
	for _, row := range rows {
		if blocks.HasUnmodelled(row) || !textOK(row.Key) || row.Key == "" && row.Content == nil {
			return nil, false
		}
		w := newAttrW()
		w.id(r.ids, row.ID)
		if t, ok := row.Text.Get(); ok {
			if row.Content != nil && t == blocks.RunsText(row.Content) {
				w.raw("text")
			} else {
				w.str("text", t)
			}
		}
		body := []string{escapeText(row.Key, escCtx{lineStart: true, lineEnd: true, colons: true})}
		if row.Content != nil {
			body[0] += " ::"
			s, ok := renderRuns(row.Content, ictx{safe: o.safe})
			if !ok && len(row.Content) > 0 {
				return nil, false
			}
			if s != "" {
				vl := strings.Split(s, "\n")
				body[0] += " " + vl[0]
				body = append(body, vl[1:]...)
			}
		}
		if a := w.String(); a != "" {
			body[len(body)-1] += " {" + a + "}"
		}
		if !w.ok {
			return nil, false
		}
		lines = append(lines, indentUnder("-", body)...)
	}
	return lines, true
}

func (r *renderer) meterItems(items blocks.MeterItems) ([]string, bool) {
	var lines []string
	for _, m := range items {
		if blocks.HasUnmodelled(m) || !textOK(m.Name) {
			return nil, false
		}
		cur, ok1 := fmtFloat(m.Current)
		mx, ok2 := fmtFloat(m.Max)
		w := newAttrW()
		w.id(r.ids, m.ID)
		w.str("icon", m.Icon)
		w.optStr("ratingIcon", m.RatingIcon)
		w.optStr("color", m.Color)
		if !ok1 || !ok2 || !w.ok {
			return nil, false
		}
		name := escapeText(m.Name, escCtx{lineStart: true, lineEnd: true, colons: true})
		lines = append(lines, "- "+name+" :: "+cur+"/"+mx+" "+w.String())
	}
	return lines, true
}

func (r *renderer) resourceItems(items blocks.ResourceItems) ([]string, bool) {
	var lines []string
	for _, it := range items {
		if blocks.HasUnmodelled(it) {
			return nil, false
		}
		w := newAttrW()
		w.value(attrResource, it.ResourceID)
		w.id(r.ids, it.ID)
		w.optStr("doc", it.DocumentID)
		if !w.ok {
			return nil, false
		}
		lines = append(lines, "- "+w.String())
	}
	return lines, true
}

func (r *renderer) collectionBody(items blocks.ResourceItems, f *blocks.Filter) ([]string, bool) {
	if items != nil && len(items) == 0 {
		return nil, false // [] and absent items are different; Markdown shows neither
	}
	lines, ok := r.resourceItems(items)
	if !ok || f == nil {
		return lines, ok
	}
	data, err := json.Marshal(f)
	if err != nil {
		return nil, false
	}
	if len(lines) > 0 {
		lines = append(lines, "")
	}
	return append(lines, fenced("lk-filter", string(data))...), true
}

// checkedProp renders an infobox row, checked the way checked checks a
// block: inside an infobox of its own.
func (r *renderer) checkedProp(p blocks.Property) *rendered {
	if rd, ok := r.memo[p]; ok {
		return rd
	}
	rd := r.computeProp(p)
	r.memo[p] = rd
	return rd
}

func (r *renderer) computeProp(p blocks.Property) *rendered {
	switch v := p.(type) {
	case *blocks.RawProperty:
		return r.raw(p)
	case *blocks.UnsupportedProperty:
		if !blocks.HasUnmodelled(v) {
			w := newAttrW()
			w.str("id", v.ID)
			envW(w, &v.Envelope)
			if w.ok {
				return &rendered{lines: leaf("unsupported", w), opaque: true}
			}
		}
		return r.raw(p)
	}
	for _, safe := range []bool{false, true} {
		lines, ok := r.propLines(p, ropt{safe: safe})
		if r.err != nil {
			return &rendered{opaque: true}
		}
		if ok {
			box := &blocks.InfoboxBlock{Rows: blocks.Properties{p}}
			parsed, err := parseDoc(joinLines(directive("infobox", newAttrW(), lines)))
			if err == nil && len(parsed) == 1 && sameShown(parsed[0], box) {
				return &rendered{lines: lines, safe: safe}
			}
		}
	}
	return r.raw(p)
}

func (r *renderer) propLines(p blocks.Property, o ropt) ([]string, bool) {
	if blocks.HasUnmodelled(p) {
		return nil, false
	}
	env := p.PropertyEnvelope()
	w := newAttrW()
	w.id(r.ids, env.ID)
	envW(w, env)
	var lines []string
	ok := true
	switch v := p.(type) {
	case *blocks.TextFieldProperty:
		lines = directive("textfield", w, r.children(v.Children))
	case *blocks.KeyValueProperty:
		optEnum(w, "display", v.Display, displays)
		var body []string
		body, ok = r.kvRows(v.Rows, o)
		lines = directive("keyvalue", w, body)
	case *blocks.ImageProperty:
		var line string
		line, ok = r.image(w, v.URL, v.Alt, func() { w.optBool("fill", v.Fill) })
		lines = []string{line}
	case *blocks.MeterProperty:
		meterW(w, v.Mode, v.ShowText, v.ShowIcon, v.ShowMax)
		var body []string
		body, ok = r.meterItems(v.Items)
		lines = directive("meter", w, body)
	case *blocks.ResourceLinkProperty:
		var body []string
		body, ok = r.resourceItems(v.Items)
		lines = directive("links", w, body)
	case *blocks.ResourceCollectionProperty:
		var body []string
		body, ok = r.collectionBody(v.Items, v.Filter)
		lines = directive("collection", w, body)
	case *blocks.EmbedProperty:
		enum(w, "provider", v.Provider, providers)
		w.str("url", v.URL)
		lines = leaf("embed", w)
	case *blocks.MapProperty:
		mapW(w, v.ResourceID, v.DocumentID, v.Viewport, v.DisplayHeight, v.Fill)
		lines = leaf("map", w)
	default:
		return nil, false
	}
	return lines, ok && w.ok
}
