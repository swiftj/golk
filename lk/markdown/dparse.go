package markdown

import (
	"encoding/json"
	"strings"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/opt"
	"github.com/yuin/goldmark/ast"
)

// Directive names (LK_MARKDOWN.md §2.3). The value is true for a leaf (::)
// directive.
var directiveNames = map[string]bool{
	"callout": false, "expand": false, "layout": false, "column": false, "secret": false,
	"textfield": false, "keyvalue": false, "meter": false, "links": false, "collection": false,
	"table": false, "row": false, "cell": false, "infobox": false,
	"embed": true, "map": true, "unsupported": true, "paragraph": true,
}

// checkDirective validates a directive's fence and returns its attributes.
func (c *conv) checkDirective(d *directiveNode) (*attrR, error) {
	if d.bad != "" {
		return nil, c.errf(d.Pos(), "%s", d.bad)
	}
	leaf, known := directiveNames[d.name]
	switch {
	case !known:
		return nil, c.errf(d.Pos(), "unknown directive %q", d.name)
	case leaf && !d.leaf:
		return nil, c.errf(d.Pos(), "%s is a one-line directive: write ::%s", d.name, d.name)
	case !leaf && d.leaf:
		return nil, c.errf(d.Pos(), "%s holds content: write :::%s, the content, then :::", d.name, d.name)
	case !d.leaf && !d.closed:
		return nil, c.errf(d.Pos(), ":::%s is never closed (end it with a ::: line)", d.name)
	}
	prefix := ":::"
	if d.leaf {
		prefix = "::"
	}
	return c.attrReader(d.attrs, d.Pos(), prefix+d.name)
}

func (c *conv) directive(d *directiveNode, depth int) (blocks.Block, error) {
	a, err := c.checkDirective(d)
	if err != nil {
		return nil, err
	}
	b, err := c.directiveBlock(d, a, depth)
	if err != nil {
		return nil, err
	}
	return b, a.done()
}

func (c *conv) directiveBlock(d *directiveNode, a *attrR, depth int) (blocks.Block, error) {
	var err error
	id := a.id()
	kids := func() (blocks.List, error) { return c.blockList(d, depth+1) }
	switch d.name {
	case "paragraph":
		b := &blocks.ParagraphBlock{ID: id, Content: blocks.Runs{}}
		if b.Align, err = readEnum(a, "align", aligns); err != nil {
			return nil, err
		}
		b.Background, err = a.str("background")
		return b, err
	case "callout":
		b := &blocks.CalloutBlock{ID: id}
		if b.Tone, err = reqEnum(a, "tone", tones); err != nil {
			return nil, err
		}
		if b.Icon, err = a.str("icon"); err != nil {
			return nil, err
		}
		if b.Color, err = a.str("color"); err != nil {
			return nil, err
		}
		b.Children, err = kids()
		return b, err
	case "expand":
		b := &blocks.ExpandBlock{ID: id}
		if b.Title, err = a.reqStr("title"); err != nil {
			return nil, err
		}
		b.Children, err = kids()
		return b, err
	case "layout":
		b := &blocks.LayoutBlock{ID: id, Children: blocks.List{}}
		err = c.each(d, "column", depth+1, func(cd *directiveNode, ca *attrR) error {
			col := &blocks.LayoutColumnBlock{ID: ca.id()}
			var err error
			if col.Width, err = ca.float("width"); err != nil {
				return err
			}
			if col.Background, err = ca.str("background"); err != nil {
				return err
			}
			if col.Children, err = c.blockList(cd, depth+2); err != nil {
				return err
			}
			b.Children = append(b.Children, col)
			return nil
		})
		return b, err
	case "secret":
		b := &blocks.SecretBlock{ID: id}
		if b.Layout, b.Width, err = placed(a); err != nil {
			return nil, err
		}
		c.secrets++
		b.Children, err = kids()
		c.secrets--
		if err != nil {
			return nil, err
		}
		if err := a.done(); err != nil {
			return nil, err
		}
		if len(b.Children) == 0 {
			// A bare ":::secret" then ":::" is more likely a mistyped fence
			// than an empty secret, and what follows it would be visible.
			return nil, c.errf(d.Pos(), "empty :::secret")
		}
		return b, nil
	case "textfield":
		b := &blocks.TextFieldBlock{ID: id}
		if b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a); err != nil {
			return nil, err
		}
		b.Children, err = kids()
		return b, err
	case "keyvalue":
		b := &blocks.KeyValueBlock{ID: id}
		if b.Display, err = readEnum(a, "display", displays); err != nil {
			return nil, err
		}
		if b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a); err != nil {
			return nil, err
		}
		b.Rows, err = c.kvRows(d)
		return b, err
	case "meter":
		b := &blocks.MeterBlock{ID: id}
		if err = c.meterAttrs(a, &b.Mode, &b.ShowText, &b.ShowIcon, &b.ShowMax); err != nil {
			return nil, err
		}
		if b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a); err != nil {
			return nil, err
		}
		b.Items, err = c.meterItems(d)
		return b, err
	case "links":
		b := &blocks.ResourceLinkBlock{ID: id}
		if b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a); err != nil {
			return nil, err
		}
		b.Items, _, err = c.resourceBody(d, false)
		return b, err
	case "collection":
		b := &blocks.ResourceCollectionBlock{ID: id}
		if b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a); err != nil {
			return nil, err
		}
		b.Items, b.Filter, err = c.resourceBody(d, true)
		return b, err
	case "table":
		if depth+2 > maxDepth {
			return nil, depthErr(c, d)
		}
		b := &blocks.TableBlock{ID: id, Children: blocks.List{}}
		err = c.each(d, "row", depth+1, func(rd *directiveNode, ra *attrR) error {
			row := &blocks.TableRowBlock{ID: ra.id(), Children: blocks.List{}}
			b.Children = append(b.Children, row)
			return c.each(rd, "cell", depth+2, func(cd *directiveNode, ca *attrR) error {
				cell := &blocks.TableCellBlock{ID: ca.id()}
				var err error
				if cell.Header, err = ca.boolean("header"); err != nil {
					return err
				}
				if cell.Colspan, err = ca.integer("colspan"); err != nil {
					return err
				}
				if cell.Rowspan, err = ca.integer("rowspan"); err != nil {
					return err
				}
				if cell.Children, err = c.blockList(cd, depth+3); err != nil {
					return err
				}
				row.Children = append(row.Children, cell)
				return nil
			})
		})
		return b, err
	case "infobox":
		b := &blocks.InfoboxBlock{ID: id, Rows: blocks.Properties{}}
		if b.Layout, b.Width, err = placed(a); err != nil {
			return nil, err
		}
		b.Rows, err = c.infoboxRows(d, depth+1)
		return b, err
	case "embed":
		b := &blocks.EmbedBlock{ID: id}
		if b.Provider, b.URL, err = embedAttrs(a); err != nil {
			return nil, err
		}
		b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a)
		return b, err
	case "map":
		b := &blocks.MapBlock{ID: id}
		if b.ResourceID, b.DocumentID, b.Viewport, b.DisplayHeight, b.Fill, err = mapAttrs(a); err != nil {
			return nil, err
		}
		b.Title, b.IsTitleHidden, b.Layout, b.Width, err = titled(a)
		return b, err
	case "unsupported":
		if id != "" {
			return nil, a.errf("write the id of an unsupported block as id=")
		}
		u, err := a.str("id")
		return &blocks.UnsupportedBlock{ID: u.Or("")}, err
	}
	return nil, c.errf(d.Pos(), ":::%s belongs inside a %s", d.name, map[string]string{"column": ":::layout", "row": ":::table", "cell": ":::row"}[d.name])
}

// each converts every child of d, which must all be closed directives
// named name.
func (c *conv) each(d *directiveNode, name string, depth int, f func(*directiveNode, *attrR) error) error {
	if depth > maxDepth {
		return depthErr(c, d)
	}
	for ch := d.FirstChild(); ch != nil; ch = ch.NextSibling() {
		cd, ok := ch.(*directiveNode)
		if !ok || cd.name != name || cd.bad != "" {
			if ok && cd.bad != "" {
				return c.errf(cd.Pos(), "%s", cd.bad)
			}
			return c.errf(nodePos(ch), ":::%s may hold only :::%s directives", d.name, name)
		}
		a, err := c.checkDirective(cd)
		if err != nil {
			return err
		}
		if err := f(cd, a); err != nil {
			return err
		}
		if err := a.done(); err != nil {
			return err
		}
	}
	return nil
}

func (c *conv) meterAttrs(a *attrR, mode *blocks.MeterMode, text, icon, maxv *opt.Optional[bool]) error {
	var err error
	if *mode, err = reqEnum(a, "mode", modes); err != nil {
		return err
	}
	if *text, err = a.boolean("showText"); err != nil {
		return err
	}
	if *icon, err = a.boolean("showIcon"); err != nil {
		return err
	}
	*maxv, err = a.boolean("showMax")
	return err
}

func embedAttrs(a *attrR) (blocks.EmbedProvider, string, error) {
	p, err := reqEnum(a, "provider", providers)
	if err != nil {
		return "", "", err
	}
	u, err := a.reqStr("url")
	return p, u, err
}

func mapAttrs(a *attrR) (rid, did opt.Optional[string], vp *blocks.Viewport, height opt.Optional[float64], fill opt.Optional[bool], err error) {
	if rid, err = a.str("resourceId"); err != nil {
		return
	}
	if did, err = a.str("documentId"); err != nil {
		return
	}
	var v opt.Optional[string]
	if v, err = a.str("viewport"); err != nil {
		return
	}
	if s, ok := v.Get(); ok {
		parts := strings.Split(s, ",")
		var f [3]float64
		good := len(parts) == 3
		for i := 0; good && i < 3; i++ {
			f[i], good = parseFloat(strings.TrimSpace(parts[i]))
		}
		if !good {
			err = a.errf(`viewport must be "lat,lng,zoom", not %q`, s)
			return
		}
		vp = &blocks.Viewport{Center: []float64{f[0], f[1]}, Zoom: f[2]}
	}
	if height, err = a.float("displayHeight"); err != nil {
		return
	}
	fill, err = a.boolean("fill")
	return
}

// bodyList returns the one bullet list a row directive's body holds, or nil
// for an empty body; rest is whatever follows the list.
func (c *conv) bodyList(d *directiveNode) (l *ast.List, rest ast.Node, err error) {
	ch := d.FirstChild()
	if ch == nil {
		return nil, nil, nil
	}
	if v, ok := ch.(*ast.List); ok && !v.IsOrdered() {
		return v, v.NextSibling(), nil
	}
	return nil, ch, nil
}

func (c *conv) onlyList(d *directiveNode, what string) (*ast.List, error) {
	l, rest, err := c.bodyList(d)
	if err == nil && rest != nil {
		err = c.errf(nodePos(rest), ":::%s holds only a list of %s", d.name, what)
	}
	return l, err
}

// itemText is the raw text of a row item: its one paragraph.
func (c *conv) itemText(it ast.Node) (string, int, error) {
	p := it.FirstChild()
	_, isTB := p.(*ast.TextBlock)
	_, isP := p.(*ast.Paragraph)
	if p == nil || !isTB && !isP || p.NextSibling() != nil {
		return "", nodePos(it), c.errf(nodePos(it), "each row is one line: - key :: value")
	}
	if err := c.fenceLines(p, true); err != nil {
		return "", 0, err
	}
	var b strings.Builder
	lines := p.Lines()
	for i := range lines.Len() {
		s := lines.At(i)
		b.Write(c.src[s.Start:s.Stop])
	}
	return b.String(), lines.At(0).Start, nil
}

// findSep finds the first unescaped "::" in s.
func findSep(s string) int {
	for i := 0; i+1 < len(s); i++ {
		switch {
		case s[i] == '\\' && isASCIIPunct(s[i+1]):
			i++
		case s[i] == ':' && s[i+1] == ':':
			return i
		}
	}
	return -1
}

// plainKey decodes a row key or meter name, which is plain text: markup
// characters must be escaped.
func (c *conv) plainKey(raw string, pos int) (string, error) {
	raw = strings.Trim(raw, " \t")
	if strings.ContainsRune(raw, '\n') {
		return "", c.errf(pos, "a key must be on one line")
	}
	for i := 0; i < len(raw); i++ {
		switch ch := raw[i]; {
		case ch == '\\' && i+1 < len(raw) && isASCIIPunct(raw[i+1]):
			i++
		case strings.IndexByte("*_`[]<{}~|", ch) >= 0:
			return "", c.errf(pos, "a key is plain text: escape %q as \\%c", ch, ch)
		}
	}
	return decodeText([]byte(raw)), nil
}

// subInline parses s, a row value, as inline LK Markdown with an optional
// trailing attribute set.
func (c *conv) subInline(s string, pos int) (blocks.Runs, *attrSetNode, []attr, error) {
	sub := newConv([]byte(s), c.line(pos)-1)
	doc, err := sub.goldmark()
	if err != nil {
		return nil, nil, nil, err
	}
	p := doc.FirstChild()
	switch v := p.(type) {
	case nil:
		return blocks.Runs{}, nil, nil, nil
	case *attrLineNode:
		if v.NextSibling() == nil {
			return blocks.Runs{}, nil, v.attrs, nil
		}
	case *ast.Paragraph:
		if v.NextSibling() != nil {
			break
		}
		if err := sub.checkLines(v); err != nil {
			return nil, nil, nil, err
		}
		set, err := sub.takeTrailingAttr(v)
		if err != nil {
			return nil, nil, nil, err
		}
		runs, err := sub.runs(v)
		if err != nil {
			return nil, nil, nil, err
		}
		if set != nil {
			return runs, set, set.attrs, nil
		}
		return runs, nil, nil, nil
	}
	return nil, nil, nil, c.errf(pos, "a row value must be inline text")
}

func (c *conv) kvRows(d *directiveNode) (blocks.KVRows, error) {
	l, err := c.onlyList(d, "key :: value rows")
	if err != nil || l == nil {
		return nil, err
	}
	rows := blocks.KVRows{}
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		raw, pos, err := c.itemText(it)
		if err != nil {
			return nil, err
		}
		row, err := c.kvRow(raw, pos)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (c *conv) kvRow(raw string, pos int) (blocks.KVRow, error) {
	var row blocks.KVRow
	var attrs []attr
	keyRaw := raw
	sep := findSep(raw)
	if sep >= 0 {
		keyRaw = raw[:sep]
		runs, _, list, err := c.subInline(raw[sep+2:], pos)
		if err != nil {
			return row, err
		}
		row.Content, attrs = runs, list
	} else if i := trailingSet(raw); i >= 0 {
		keyRaw = raw[:i]
		attrs, _, _ = scanAttrSet([]byte(raw[i:]))
	}
	key, err := c.plainKey(keyRaw, pos)
	if err != nil {
		return row, err
	}
	row.Key = key
	a, err := c.attrReader(attrs, pos, "row")
	if err != nil {
		return row, err
	}
	row.ID = a.id()
	if x, ok := a.find("text"); ok {
		switch {
		case !x.flag:
			row.Text = opt.Some(x.value)
		case row.Content == nil:
			return row, a.errf(`a bare "text" needs a value after ::`)
		default:
			row.Text = opt.Some(blocks.RunsText(row.Content))
		}
	}
	return row, a.done()
}

// trailingSet is the index of an attribute set that ends s, preceded by a
// space, or -1.
func trailingSet(s string) int {
	for i := 1; i < len(s); i++ {
		if s[i] != '{' || s[i-1] != ' ' {
			continue
		}
		if _, n, ok := scanAttrSet([]byte(s[i:])); ok && i+n == len(s) {
			return i
		}
	}
	return -1
}

func (c *conv) meterItems(d *directiveNode) (blocks.MeterItems, error) {
	l, err := c.onlyList(d, "name :: current/max items")
	if err != nil || l == nil {
		return nil, err
	}
	items := blocks.MeterItems{}
	for it := l.FirstChild(); it != nil; it = it.NextSibling() {
		raw, pos, err := c.itemText(it)
		if err != nil {
			return nil, err
		}
		sep := findSep(raw)
		if sep < 0 || strings.ContainsRune(raw, '\n') {
			return nil, c.errf(pos, "a meter item is one line: - name :: current/max icon=…")
		}
		var m blocks.MeterItem
		if m.Name, err = c.plainKey(raw[:sep], pos); err != nil {
			return nil, err
		}
		rest := strings.TrimLeft(raw[sep+2:], " \t")
		valEnd := strings.IndexAny(rest, " \t")
		if valEnd < 0 {
			valEnd = len(rest)
		}
		cur, maxv, ok := strings.Cut(rest[:valEnd], "/")
		if m.Current, ok = parseFloatOK(cur, ok); ok {
			m.Max, ok = parseFloatOK(maxv, true)
		}
		if !ok {
			return nil, c.errf(pos, "a meter item needs current/max numbers, as in 3/5")
		}
		list, _, reason := scanAttrs([]byte(rest[valEnd:]), 0, false, false)
		if reason != "" {
			return nil, c.errf(pos, "meter item: %s", reason)
		}
		a, err := c.attrReader(list, pos, "meter item")
		if err != nil {
			return nil, err
		}
		m.ID = a.id()
		if m.Icon, err = a.reqStr("icon"); err != nil {
			return nil, err
		}
		if m.RatingIcon, err = a.str("ratingIcon"); err != nil {
			return nil, err
		}
		if m.Color, err = a.str("color"); err != nil {
			return nil, err
		}
		if err := a.done(); err != nil {
			return nil, err
		}
		items = append(items, m)
	}
	return items, nil
}

func parseFloatOK(s string, ok bool) (float64, bool) {
	if !ok {
		return 0, false
	}
	return parseFloat(s)
}

// resourceBody reads a list of "- @resourceId doc=…" items and, for a
// collection, an lk-filter fence after it.
func (c *conv) resourceBody(d *directiveNode, filter bool) (blocks.ResourceItems, *blocks.Filter, error) {
	l, rest, err := c.bodyList(d)
	if err != nil {
		return nil, nil, err
	}
	var items blocks.ResourceItems
	if l != nil {
		items = blocks.ResourceItems{}
		for it := l.FirstChild(); it != nil; it = it.NextSibling() {
			raw, pos, err := c.itemText(it)
			if err != nil {
				return nil, nil, err
			}
			list, _, reason := scanAttrs([]byte(raw), 0, false, true)
			if reason != "" || len(list) == 0 || list[0].name != attrResource || strings.ContainsRune(raw, '\n') {
				return nil, nil, c.errf(pos, "a resource item is - @resourceId, optionally with doc=")
			}
			a, err := c.attrReader(list, pos, "resource item")
			if err != nil {
				return nil, nil, err
			}
			x, _ := a.find(attrResource)
			ri := blocks.ResourceItem{ID: a.id(), ResourceID: x.value}
			if ri.DocumentID, err = a.str("doc"); err != nil {
				return nil, nil, err
			}
			if err := a.done(); err != nil {
				return nil, nil, err
			}
			items = append(items, ri)
		}
	}
	if rest == nil {
		return items, nil, nil
	}
	f, ok := rest.(*ast.FencedCodeBlock)
	if !filter || !ok || rest.NextSibling() != nil {
		what := "a list of - @resourceId items"
		if filter {
			what += " and an lk-filter fence"
		}
		return nil, nil, c.errf(nodePos(rest), ":::%s holds only %s", d.name, what)
	}
	info, body, err := c.fenceText(f)
	if err != nil {
		return nil, nil, err
	}
	if info != "lk-filter" {
		return nil, nil, c.errf(f.Pos(), "a collection's filter is an lk-filter fence")
	}
	data, err := c.rawJSON(f, body)
	if err != nil {
		return nil, nil, err
	}
	var flt blocks.Filter
	if err := json.Unmarshal(data, &flt); err != nil {
		return nil, nil, c.errf(f.Pos(), "lk-filter: %v", err)
	}
	return items, &flt, nil
}

func (c *conv) infoboxRows(d *directiveNode, depth int) (blocks.Properties, error) {
	if depth > maxDepth {
		return nil, depthErr(c, d)
	}
	rows := blocks.Properties{}
	for ch := d.FirstChild(); ch != nil; ch = ch.NextSibling() {
		var p blocks.Property
		var err error
		switch v := ch.(type) {
		case *directiveNode:
			p, err = c.property(v, depth)
		case *ast.Paragraph, *ast.TextBlock:
			im, set, ok := imageParagraph(v)
			if !ok {
				return nil, c.errf(nodePos(v), "an infobox holds property directives, images and lk-raw rows, not text")
			}
			p, err = c.imageProperty(im, set)
		case *ast.FencedCodeBlock:
			var info, body string
			if info, body, err = c.fenceText(v); err != nil {
				return nil, err
			}
			if info != "lk-raw" {
				return nil, c.errf(v.Pos(), "an infobox holds property directives, images and lk-raw rows, not code")
			}
			var data []byte
			if data, err = c.rawJSON(v, body); err != nil {
				return nil, err
			}
			if p, err = blocks.UnmarshalProperty(data); err != nil {
				err = c.errf(v.Pos(), "lk-raw: %v", err)
			}
		default:
			return nil, c.errf(nodePos(ch), "an infobox holds property directives, images and lk-raw rows")
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, p)
	}
	return rows, nil
}

func envelope(a *attrR) (blocks.Envelope, error) {
	env := blocks.Envelope{ID: a.id()}
	var err error
	if env.Title, err = a.str("title"); err != nil {
		return env, err
	}
	if env.IsTitleHidden, err = a.boolean("isTitleHidden"); err != nil {
		return env, err
	}
	env.IsHidden, err = a.boolean("isHidden")
	return env, err
}

func (c *conv) imageProperty(im *ast.Image, set *attrSetNode) (blocks.Property, error) {
	a, url, alt, err := c.imageAttrs(im, set, "image row")
	if err != nil {
		return nil, err
	}
	env, err := envelope(a)
	if err != nil {
		return nil, err
	}
	p := &blocks.ImageProperty{Envelope: env, URL: url, Alt: alt}
	if p.Fill, err = a.boolean("fill"); err != nil {
		return nil, err
	}
	return p, a.done()
}

func (c *conv) property(d *directiveNode, depth int) (blocks.Property, error) {
	a, err := c.checkDirective(d)
	if err != nil {
		return nil, err
	}
	if d.name == "unsupported" {
		u, err := a.str("id")
		if err != nil {
			return nil, err
		}
		if _, ok := a.find(attrID); ok {
			return nil, a.errf("write the id of an unsupported row as id=")
		}
		env, err := envelope(a)
		env.ID = u.Or("")
		if err != nil {
			return nil, err
		}
		return &blocks.UnsupportedProperty{Envelope: env}, a.done()
	}
	env, err := envelope(a)
	if err != nil {
		return nil, err
	}
	var p blocks.Property
	switch d.name {
	case "textfield":
		kids, err := c.blockList(d, depth+1)
		if err != nil {
			return nil, err
		}
		p = &blocks.TextFieldProperty{Envelope: env, Children: kids}
	case "keyvalue":
		v := &blocks.KeyValueProperty{Envelope: env}
		if v.Display, err = readEnum(a, "display", displays); err != nil {
			return nil, err
		}
		if v.Rows, err = c.kvRows(d); err != nil {
			return nil, err
		}
		p = v
	case "meter":
		v := &blocks.MeterProperty{Envelope: env}
		if err = c.meterAttrs(a, &v.Mode, &v.ShowText, &v.ShowIcon, &v.ShowMax); err != nil {
			return nil, err
		}
		if v.Items, err = c.meterItems(d); err != nil {
			return nil, err
		}
		p = v
	case "links":
		v := &blocks.ResourceLinkProperty{Envelope: env}
		if v.Items, _, err = c.resourceBody(d, false); err != nil {
			return nil, err
		}
		p = v
	case "collection":
		v := &blocks.ResourceCollectionProperty{Envelope: env}
		if v.Items, v.Filter, err = c.resourceBody(d, true); err != nil {
			return nil, err
		}
		p = v
	case "embed":
		v := &blocks.EmbedProperty{Envelope: env}
		if v.Provider, v.URL, err = embedAttrs(a); err != nil {
			return nil, err
		}
		p = v
	case "map":
		v := &blocks.MapProperty{Envelope: env}
		if v.ResourceID, v.DocumentID, v.Viewport, v.DisplayHeight, v.Fill, err = mapAttrs(a); err != nil {
			return nil, err
		}
		p = v
	default:
		return nil, c.errf(d.Pos(), "%s can't be an infobox row", d.name)
	}
	return p, a.done()
}
