// Package blocks is LegendKeeper's page model: the block and property types,
// builders that make them, and readers that walk them.
//
// The types are the wire types. A block marshals to exactly the JSON the API
// takes: required fields always, optional fields only when set
// (lk.Optional), and "kind" from the type. A kind this package does not know
// decodes to *RawBlock, and fields it does not model are kept, so reading a page
// and writing it back never drops content.
package blocks

import (
	"encoding/json"
	"fmt"
)

// Node is anything Walk visits: a Block, or a Property in an infobox.
type Node interface {
	Kind() Kind
	// NodeID is the block or property id; "" when it has none yet.
	NodeID() string
}

// Block is one block of a page. It is one of the pointer types in this
// package (*ParagraphBlock, *HeadingBlock, …, *UnsupportedBlock) or
// *RawBlock for a kind this package does not know.
type Block interface {
	Node
	json.Marshaler
	isBlock()
}

// PropertyCapable is a block kind that can also be a property: a sidebar
// property or an infobox row. AsProperty turns one into a Property.
type PropertyCapable interface {
	Block
	asProperty(Envelope) Property
}

// container is a block with children.
type container interface {
	childList() *List
}

// List is a list of blocks: a page, or a container's children. It encodes a
// nil list as [].
type List []Block

// MarshalJSON encodes l, with nil as [].
func (l List) MarshalJSON() ([]byte, error) {
	if l == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Block(l))
}

// UnmarshalJSON decodes a JSON array of blocks by kind.
func (l *List) UnmarshalJSON(data []byte) error {
	items, err := arrayItems(data)
	if err != nil {
		return err
	}
	out := make(List, 0, len(items))
	for i, raw := range items {
		b, err := Unmarshal(raw)
		if err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
		out = append(out, b)
	}
	*l = out
	return nil
}

// Unmarshal decodes one block by its kind. An unknown kind, or a known
// kind in a shape this package does not expect, decodes to *RawBlock. Only
// JSON that is not an object with a string "kind" is an error.
func Unmarshal(data []byte) (Block, error) {
	kind, err := kindOf(data)
	if err != nil {
		return nil, err
	}
	newBlock, ok := blockKinds[Kind(kind)]
	if !ok {
		return &RawBlock{KindName: Kind(kind), JSON: append(json.RawMessage(nil), data...)}, nil
	}
	b := newBlock()
	if err := json.Unmarshal(data, b); err != nil {
		// A known kind in a shape this package does not expect is kept as
		// is rather than failing the whole page.
		return &RawBlock{KindName: Kind(kind), JSON: append(json.RawMessage(nil), data...)}, nil
	}
	return b, nil
}

var blockKinds = map[Kind]func() Block{
	KindParagraph:          func() Block { return new(ParagraphBlock) },
	KindHeading:            func() Block { return new(HeadingBlock) },
	KindQuote:              func() Block { return new(QuoteBlock) },
	KindCode:               func() Block { return new(CodeBlock) },
	KindRule:               func() Block { return new(RuleBlock) },
	KindBulletList:         func() Block { return new(BulletListBlock) },
	KindOrderedList:        func() Block { return new(OrderedListBlock) },
	KindListItem:           func() Block { return new(ListItemBlock) },
	KindTaskList:           func() Block { return new(TaskListBlock) },
	KindTaskItem:           func() Block { return new(TaskItemBlock) },
	KindTable:              func() Block { return new(TableBlock) },
	KindTableRow:           func() Block { return new(TableRowBlock) },
	KindTableCell:          func() Block { return new(TableCellBlock) },
	KindCallout:            func() Block { return new(CalloutBlock) },
	KindExpand:             func() Block { return new(ExpandBlock) },
	KindLayout:             func() Block { return new(LayoutBlock) },
	KindLayoutColumn:       func() Block { return new(LayoutColumnBlock) },
	KindImage:              func() Block { return new(ImageBlock) },
	KindEmbed:              func() Block { return new(EmbedBlock) },
	KindKeyValue:           func() Block { return new(KeyValueBlock) },
	KindInfobox:            func() Block { return new(InfoboxBlock) },
	KindMeter:              func() Block { return new(MeterBlock) },
	KindResourceCollection: func() Block { return new(ResourceCollectionBlock) },
	KindResourceLink:       func() Block { return new(ResourceLinkBlock) },
	KindMap:                func() Block { return new(MapBlock) },
	KindSecret:             func() Block { return new(SecretBlock) },
	KindTextField:          func() Block { return new(TextFieldBlock) },
	KindUnsupported:        func() Block { return new(UnsupportedBlock) },
}

// RawBlock is a block this package cannot decode: a kind it does not know,
// or a known kind in an unexpected shape. It keeps the JSON it was read from
// and writes it back unchanged.
type RawBlock struct {
	KindName Kind
	JSON     json.RawMessage
}

func (b *RawBlock) Kind() Kind { return b.KindName }

// NodeID returns the raw block's "id", if it has one.
func (b *RawBlock) NodeID() string {
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(b.JSON, &v)
	return v.ID
}

func (b *RawBlock) MarshalJSON() ([]byte, error) { return b.JSON, nil }
func (*RawBlock) isBlock()                       {}

// UnsupportedBlock is a block the public model cannot express. It is read-only:
// echo it by id in a full-page replace and the server keeps its content in
// place; leave it out and that content is deleted. Prefer single-block
// operations on pages that hold one.
type UnsupportedBlock struct {
	ID string `json:"id"`
	extras
}

func (b *UnsupportedBlock) setID(id string) { b.ID = id }
func (b *UnsupportedBlock) Kind() Kind      { return KindUnsupported }
func (b *UnsupportedBlock) NodeID() string  { return b.ID }
func (*UnsupportedBlock) isBlock()          {}

func (b *UnsupportedBlock) MarshalJSON() ([]byte, error) {
	type plain UnsupportedBlock
	return encodeObject(string(KindUnsupported), (*plain)(b), b.m)
}

func (b *UnsupportedBlock) UnmarshalJSON(data []byte) error {
	type plain UnsupportedBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
