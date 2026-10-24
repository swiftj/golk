package blocks

import (
	"fmt"

	"github.com/swiftj/golk/lk/internal/opt"
)

// Builders return the same structs you can write by hand; set any other
// field on the result. Arguments typed any follow two conventions:
//
//   - Text arguments (P, H, Task, the run helpers) take strings, Runs, single
//     runs and []any of these, in any mix: P("Hello ", Bold("world"), "!").
//   - Children arguments take blocks and strings; a string is a paragraph.
//     Lists of blocks (List, []Block, []any) are flattened.
//
// Any other value is written as its fmt.Sprint text. Builders do not check
// what the server checks (colors, widths, callout icons); its 400 is the
// authority.

// R normalizes text arguments into runs.
func R(text ...any) Runs {
	var out Runs
	for _, a := range text {
		switch v := a.(type) {
		case nil:
		case string:
			out = append(out, &Text{Text: v})
		case Run:
			out = append(out, v)
		case Runs:
			out = append(out, v...)
		case []Run:
			out = append(out, v...)
		case []any:
			out = append(out, R(v...)...)
		default:
			out = append(out, &Text{Text: fmt.Sprint(v)})
		}
	}
	return out
}

// children normalizes children arguments into blocks.
func children(args ...any) List {
	out := List{}
	for _, a := range args {
		switch v := a.(type) {
		case nil:
		case Block:
			out = append(out, v)
		case List:
			out = append(out, v...)
		case []Block:
			out = append(out, v...)
		case []any:
			out = append(out, children(v...)...)
		case Runs, Run, []Run:
			out = append(out, P(v))
		default:
			out = append(out, P(fmt.Sprint(v)))
		}
	}
	return out
}

// P is a paragraph of text.
func P(text ...any) *ParagraphBlock { return &ParagraphBlock{Content: R(text...)} }

// H is a heading at level 1 through 6.
func H(level int, text ...any) *HeadingBlock { return &HeadingBlock{Level: level, Content: R(text...)} }

// Quote is a block quote with at least one child.
func Quote(first any, rest ...any) *QuoteBlock {
	return &QuoteBlock{Children: children(append([]any{first}, rest...)...)}
}

// Code is a code block of plain text; language "" sends none.
func Code(text, language string) *CodeBlock {
	b := &CodeBlock{Text: text}
	if language != "" {
		b.Language = opt.Some(language)
	}
	return b
}

// Rule is a horizontal rule.
func Rule() *RuleBlock { return &RuleBlock{} }

// listItem makes one list entry from a list argument: a *ListItem as is,
// another block wrapped in an item, and text as a one-paragraph item.
func listItem(a any) Block {
	switch v := a.(type) {
	case *ListItemBlock:
		return v
	case *ParagraphBlock:
		return &ListItemBlock{Children: List{v}}
	case Block:
		return &ListItemBlock{Children: List{v}}
	}
	return LI(a)
}

func listItems(first any, rest []any) List {
	out := List{listItem(first)}
	for _, a := range rest {
		out = append(out, listItem(a))
	}
	return out
}

// UL is an unordered list of at least one item. An item is a *ListItem, or
// text (a string, runs, or an []any of them) for a one-paragraph item.
func UL(first any, rest ...any) *BulletListBlock {
	return &BulletListBlock{Children: listItems(first, rest)}
}

// OL is a numbered list of at least one item, with items as in UL. Set
// Start on the result to start at another number.
func OL(first any, rest ...any) *OrderedListBlock {
	return &OrderedListBlock{Children: listItems(first, rest)}
}

// LI is one list item. first is its text or a *ParagraphBlock; rest can be
// nested lists.
func LI(first any, rest ...any) *ListItemBlock {
	p, ok := first.(*ParagraphBlock)
	if !ok {
		p = P(first)
	}
	return &ListItemBlock{Children: append(List{p}, children(rest...)...)}
}

// Tasks is a checklist of at least one *TaskItem or nested *TaskList.
func Tasks(first Block, rest ...Block) *TaskListBlock {
	return &TaskListBlock{Children: append(List{first}, rest...)}
}

// Task is one checklist entry.
func Task(checked bool, text ...any) *TaskItemBlock {
	return &TaskItemBlock{Checked: checked, Content: R(text...)}
}

// Table is a table of at least one row.
func Table(first *TableRowBlock, rest ...*TableRowBlock) *TableBlock {
	t := &TableBlock{Children: List{first}}
	for _, r := range rest {
		t.Children = append(t.Children, r)
	}
	return t
}

// TR is one table row.
func TR(cells ...*TableCellBlock) *TableRowBlock {
	r := &TableRowBlock{Children: List{}}
	for _, c := range cells {
		r.Children = append(r.Children, c)
	}
	return r
}

// TD is one table cell. Set Colspan and Rowspan on the result.
func TD(content ...any) *TableCellBlock { return &TableCellBlock{Children: children(content...)} }

// TH is one header cell.
func TH(content ...any) *TableCellBlock {
	return &TableCellBlock{Header: opt.Some(true), Children: children(content...)}
}

// Callout is a callout in tone. For ToneCustom, set Icon (a Font Awesome
// name) and Color (#RRGGBB) on the result.
func Callout(tone Tone, content ...any) *CalloutBlock {
	return &CalloutBlock{Tone: tone, Children: children(content...)}
}

// Expand is a collapsible section.
func Expand(title string, content ...any) *ExpandBlock {
	return &ExpandBlock{Title: title, Children: children(content...)}
}

// Layout is a row of two or three layout columns.
func Layout(first, second *LayoutColumnBlock, third ...*LayoutColumnBlock) *LayoutBlock {
	l := &LayoutBlock{Children: List{first, second}}
	for _, c := range third {
		l.Children = append(l.Children, c)
	}
	return l
}

// Column is one layout column, width percent wide; a width of 0 or less
// sends none.
func Column(width float64, content ...any) *LayoutColumnBlock {
	c := &LayoutColumnBlock{Children: children(content...)}
	if width > 0 {
		c.Width = opt.Some(width)
	}
	return c
}

// Image is an image by the url of an image asset of the project; alt ""
// sends none.
func Image(url, alt string) *ImageBlock {
	b := &ImageBlock{URL: url}
	if alt != "" {
		b.Alt = opt.Some(alt)
	}
	return b
}

// Embed is an embedded player. The API accepts any http or https URL and
// does not check it against the provider.
func Embed(provider EmbedProvider, url string) *EmbedBlock {
	return &EmbedBlock{Provider: provider, URL: url}
}

// KeyValue is a block of key-value rows.
func KeyValue(rows ...KVRow) *KeyValueBlock {
	return &KeyValueBlock{Rows: KVRows(append([]KVRow{}, rows...))}
}

// KV is one key-value row; value takes text arguments.
func KV(key string, value ...any) KVRow { return KVRow{Key: key, Content: R(value...)} }

// Infobox is an infobox of properties (see AsProperty).
func Infobox(rows ...Property) *InfoboxBlock {
	return &InfoboxBlock{Rows: Properties(append([]Property{}, rows...))}
}

// Meter is a meter of items drawn in mode.
func Meter(mode MeterMode, items ...MeterItem) *MeterBlock {
	return &MeterBlock{Mode: mode, Items: MeterItems(append([]MeterItem{}, items...))}
}

// resourceItems takes resource ids (strings) and ResourceItems.
func resourceItems(items []any) ResourceItems {
	out := ResourceItems{}
	for _, a := range items {
		switch v := a.(type) {
		case string:
			out = append(out, ResourceItem{ResourceID: v})
		case ResourceItem:
			out = append(out, v)
		case *ResourceItem:
			out = append(out, *v)
		default:
			out = append(out, ResourceItem{ResourceID: fmt.Sprint(v)})
		}
	}
	return out
}

// Links is a list of links to resources; a string is a resource id.
func Links(items ...any) *ResourceLinkBlock { return &ResourceLinkBlock{Items: resourceItems(items)} }

// Collection is a collection of the given resources; a string is a resource
// id. For a filtered collection, use CollectionWhere.
func Collection(items ...any) *ResourceCollectionBlock {
	return &ResourceCollectionBlock{Items: resourceItems(items)}
}

// CollectionWhere is a collection selected by a filter predicate.
func CollectionWhere(f Filter) *ResourceCollectionBlock { return &ResourceCollectionBlock{Filter: &f} }

// Map draws the map document documentID of resource resourceID. Without
// a Viewport, the map centers on a pin that links to the resource holding
// the page.
func Map(resourceID, documentID string) *MapBlock {
	return &MapBlock{ResourceID: opt.Some(resourceID), DocumentID: opt.Some(documentID)}
}

// EmptyMap is a map block linked to no map, which the app shows as "Link a
// map" (DECISIONS.md D-008, A-25).
func EmptyMap() *MapBlock { return &MapBlock{} }

// Secret is content hidden from players.
func Secret(content ...any) *SecretBlock { return &SecretBlock{Children: children(content...)} }

// TextField is a rich-text container, used mostly as a property body.
func TextField(content ...any) *TextFieldBlock {
	return &TextFieldBlock{Children: children(content...)}
}

// Style is the style fields Styled sets. Paragraphs and headings take Align
// and Background, a quote takes Color, and a layout column takes Background;
// a field the block does not have is ignored. Colors are #RRGGBB.
type Style struct {
	Align      opt.Optional[Align]
	Background opt.Optional[string]
	Color      opt.Optional[string]
}

// Stylable is a block Styled applies to.
type Stylable interface {
	Block
	applyStyle(Style)
}

// Styled sets s on b and returns b.
func Styled[T Stylable](b T, s Style) T {
	b.applyStyle(s)
	return b
}

func (b *ParagraphBlock) applyStyle(s Style) {
	setIf(&b.Align, s.Align)
	setIf(&b.Background, s.Background)
}

func (b *HeadingBlock) applyStyle(s Style) {
	setIf(&b.Align, s.Align)
	setIf(&b.Background, s.Background)
}

func (b *QuoteBlock) applyStyle(s Style)        { setIf(&b.Color, s.Color) }
func (b *LayoutColumnBlock) applyStyle(s Style) { setIf(&b.Background, s.Background) }

func setIf[T any](dst *opt.Optional[T], v opt.Optional[T]) {
	if v.IsSet() {
		*dst = v
	}
}

// WithID sets b's id and returns b. Scripts that rebuild a known layout
// use it to keep the same ids on every run. It has no effect on a *RawBlock.
func WithID[T Block](id string, b T) T {
	if s, ok := any(b).(interface{ setID(string) }); ok {
		s.setID(id)
	}
	return b
}

// AsProperty turns a property-capable block into a property with env. The
// block's title fields move to the envelope unless env sets them; layout
// and width, which properties do not have, are dropped.
func AsProperty(b PropertyCapable, env Envelope) Property { return b.asProperty(env) }

func envWithTitle(env Envelope, title opt.Optional[string], hidden opt.Optional[bool]) Envelope {
	if !env.Title.IsSet() {
		env.Title = title
	}
	if !env.IsTitleHidden.IsSet() {
		env.IsTitleHidden = hidden
	}
	return env
}

func (b *TextFieldBlock) asProperty(env Envelope) Property {
	return &TextFieldProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), Children: b.Children}
}

func (b *KeyValueBlock) asProperty(env Envelope) Property {
	return &KeyValueProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), Display: b.Display, Rows: b.Rows}
}

func (b *ImageBlock) asProperty(env Envelope) Property {
	return &ImageProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), URL: b.URL, Alt: b.Alt, Fill: b.Fill}
}

func (b *MeterBlock) asProperty(env Envelope) Property {
	return &MeterProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), Mode: b.Mode, Items: b.Items,
		ShowText: b.ShowText, ShowIcon: b.ShowIcon, ShowMax: b.ShowMax}
}

func (b *ResourceCollectionBlock) asProperty(env Envelope) Property {
	return &ResourceCollectionProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), Items: b.Items, Filter: b.Filter}
}

func (b *ResourceLinkBlock) asProperty(env Envelope) Property {
	return &ResourceLinkProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), Items: b.Items}
}

func (b *EmbedBlock) asProperty(env Envelope) Property {
	return &EmbedProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), Provider: b.Provider, URL: b.URL}
}

func (b *MapBlock) asProperty(env Envelope) Property {
	return &MapProperty{Envelope: envWithTitle(env, b.Title, b.IsTitleHidden), ResourceID: b.ResourceID, DocumentID: b.DocumentID,
		Viewport: b.Viewport, DisplayHeight: b.DisplayHeight, Fill: b.Fill}
}

// Run helpers. Each returns new runs and leaves its arguments unchanged.

// mark copies the text runs in text, applies f to each one that is not
// mono, and keeps every other run as is.
func mark(text []any, f func(*Text)) Runs {
	out := R(text...)
	for i, r := range out {
		if t, ok := r.(*Text); ok && !t.Code {
			c := *t
			f(&c)
			out[i] = &c
		}
	}
	return out
}

// Bold is bold text.
func Bold(text ...any) Runs { return mark(text, func(t *Text) { t.Bold = true }) }

// Italic is italic text.
func Italic(text ...any) Runs { return mark(text, func(t *Text) { t.Italic = true }) }

// Underline is underlined text.
func Underline(text ...any) Runs { return mark(text, func(t *Text) { t.Underline = true }) }

// Strike is struck-through text.
func Strike(text ...any) Runs { return mark(text, func(t *Text) { t.Strike = true }) }

// Sub is subscript text. It clears superscript.
func Sub(text ...any) Runs { return mark(text, func(t *Text) { t.Sub, t.Sup = true, false }) }

// Sup is superscript text. It clears subscript.
func Sup(text ...any) Runs { return mark(text, func(t *Text) { t.Sup, t.Sub = true, false }) }

// Link is text linking to href (http, https, mailto or lk).
func Link(href string, text ...any) Runs {
	return mark(text, func(t *Text) { t.Link = opt.Some(href) })
}

// Mono is inline code. It clears every other mark, color and highlight, and
// later marks skip it.
func Mono(text ...any) Runs {
	out := R(text...)
	for i, r := range out {
		if t, ok := r.(*Text); ok {
			out[i] = &Text{Text: t.Text, Code: true, extras: t.extras}
		}
	}
	return out
}

// Color is text, and mentions, in a #RRGGBB color. It skips mono runs.
func Color(hex string, text ...any) Runs { return paint(text, hex, false) }

// Highlight is text, and mentions, with a #RRGGBB highlight behind it. It
// skips mono runs.
func Highlight(hex string, text ...any) Runs { return paint(text, hex, true) }

func paint(text []any, hex string, highlight bool) Runs {
	set := func(color, hl *opt.Optional[string]) {
		if highlight {
			*hl = opt.Some(hex)
		} else {
			*color = opt.Some(hex)
		}
	}
	out := R(text...)
	for i, r := range out {
		switch v := r.(type) {
		case *Text:
			if !v.Code {
				c := *v
				set(&c.Color, &c.Highlight)
				out[i] = &c
			}
		case *Mention:
			c := *v
			set(&c.Color, &c.Highlight)
			out[i] = &c
		}
	}
	return out
}

// MentionOf is a mention of a resource, labelled label ("" for none, which
// the app draws as the resource's name). For a mention of one document, set
// DocumentID on a *Mention directly.
func MentionOf(resourceID, label string) Runs {
	m := &Mention{ResourceID: resourceID}
	if label != "" {
		m.Label = opt.Some(label)
	}
	return Runs{m}
}

// BR is a line break.
func BR() Run { return &Break{} }
