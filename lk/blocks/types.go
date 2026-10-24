package blocks

import "github.com/swiftj/golk/lk/internal/opt"

// ParagraphBlock is a paragraph of text.
type ParagraphBlock struct {
	ID         string               `json:"id,omitempty"`
	Align      opt.Optional[Align]  `json:"align,omitzero"`
	Background opt.Optional[string] `json:"background,omitzero"`
	Content    Runs                 `json:"content"`
	extras
}

// HeadingBlock is a heading at Level 1 through 6.
type HeadingBlock struct {
	ID         string               `json:"id,omitempty"`
	Level      int                  `json:"level"`
	Align      opt.Optional[Align]  `json:"align,omitzero"`
	Background opt.Optional[string] `json:"background,omitzero"`
	Content    Runs                 `json:"content"`
	extras
}

// QuoteBlock is a block quote. Color (#RRGGBB) tints the quote bar.
type QuoteBlock struct {
	ID       string               `json:"id,omitempty"`
	Color    opt.Optional[string] `json:"color,omitzero"`
	Children List                 `json:"children"`
	extras
}

// CodeBlock is a code block of plain text.
type CodeBlock struct {
	ID       string               `json:"id,omitempty"`
	Language opt.Optional[string] `json:"language,omitzero"`
	Text     string               `json:"text"`
	extras
}

// RuleBlock is a horizontal rule.
type RuleBlock struct {
	ID string `json:"id,omitempty"`
	extras
}

// BulletListBlock is an unordered list of ListItems.
type BulletListBlock struct {
	ID       string `json:"id,omitempty"`
	Children List   `json:"children"`
	extras
}

// OrderedListBlock is a numbered list of ListItems. Start is omitted on read
// when it is 1, the default.
type OrderedListBlock struct {
	ID       string            `json:"id,omitempty"`
	Start    opt.Optional[int] `json:"start,omitzero"`
	Children List              `json:"children"`
	extras
}

// ListItemBlock is one list entry. Its first child must be a paragraph; nested
// lists follow it.
type ListItemBlock struct {
	ID       string `json:"id,omitempty"`
	Children List   `json:"children"`
	extras
}

// TaskListBlock is a checklist of TaskItems and nested TaskLists.
type TaskListBlock struct {
	ID       string `json:"id,omitempty"`
	Children List   `json:"children"`
	extras
}

// TaskItemBlock is one checklist entry.
type TaskItemBlock struct {
	ID      string `json:"id,omitempty"`
	Checked bool   `json:"checked"`
	Content Runs   `json:"content"`
	extras
}

// TableBlock is a table of TableRows.
type TableBlock struct {
	ID       string `json:"id,omitempty"`
	Children List   `json:"children"`
	extras
}

// TableRowBlock is one table row of TableCells.
type TableRowBlock struct {
	ID       string `json:"id,omitempty"`
	Children List   `json:"children"`
	extras
}

// TableCellBlock is one table cell; Header makes it a header cell.
type TableCellBlock struct {
	ID       string             `json:"id,omitempty"`
	Header   opt.Optional[bool] `json:"header,omitzero"`
	Colspan  opt.Optional[int]  `json:"colspan,omitzero"`
	Rowspan  opt.Optional[int]  `json:"rowspan,omitzero"`
	Children List               `json:"children"`
	extras
}

// CalloutBlock is a callout box. Icon (a Font Awesome name) and Color (#RRGGBB)
// apply only with ToneCustom; the server rejects them with any other tone.
type CalloutBlock struct {
	ID       string               `json:"id,omitempty"`
	Tone     Tone                 `json:"tone"`
	Icon     opt.Optional[string] `json:"icon,omitzero"`
	Color    opt.Optional[string] `json:"color,omitzero"`
	Children List                 `json:"children"`
	extras
}

// ExpandBlock is a collapsible section.
type ExpandBlock struct {
	ID       string `json:"id,omitempty"`
	Title    string `json:"title"`
	Children List   `json:"children"`
	extras
}

// LayoutBlock is a row of two or three LayoutColumns.
type LayoutBlock struct {
	ID       string `json:"id,omitempty"`
	Children List   `json:"children"`
	extras
}

// LayoutColumnBlock is one column of a Layout. Width is a percentage.
type LayoutColumnBlock struct {
	ID         string                `json:"id,omitempty"`
	Width      opt.Optional[float64] `json:"width,omitzero"`
	Background opt.Optional[string]  `json:"background,omitzero"`
	Children   List                  `json:"children"`
	extras
}

// ImageBlock is an image block. URL must be the url of an image asset of the
// same project. Fill applies while the title is hidden.
type ImageBlock struct {
	ID            string                    `json:"id,omitempty"`
	URL           string                    `json:"url"`
	Alt           opt.Optional[string]      `json:"alt,omitzero"`
	Fill          opt.Optional[bool]        `json:"fill,omitzero"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	extras
}

// EmbedBlock is an embedded player.
type EmbedBlock struct {
	ID            string                    `json:"id,omitempty"`
	Provider      EmbedProvider             `json:"provider"`
	URL           string                    `json:"url"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	extras
}

// KeyValueBlock is a block of key-value rows.
type KeyValueBlock struct {
	ID            string                        `json:"id,omitempty"`
	Display       opt.Optional[KeyValueDisplay] `json:"display,omitzero"`
	Rows          KVRows                        `json:"rows"`
	Title         opt.Optional[string]          `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]            `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout]     `json:"layout,omitzero"`
	Width         opt.Optional[int]             `json:"width,omitzero"`
	extras
}

// InfoboxBlock is a box of properties beside the text. A new infobox floats
// right at width 40.
type InfoboxBlock struct {
	ID     string                    `json:"id,omitempty"`
	Layout opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width  opt.Optional[int]         `json:"width,omitzero"`
	Rows   Properties                `json:"rows"`
	extras
}

// MeterBlock draws tracked values.
type MeterBlock struct {
	ID            string                    `json:"id,omitempty"`
	Mode          MeterMode                 `json:"mode"`
	Items         MeterItems                `json:"items"`
	ShowText      opt.Optional[bool]        `json:"showText,omitzero"`
	ShowIcon      opt.Optional[bool]        `json:"showIcon,omitzero"`
	ShowMax       opt.Optional[bool]        `json:"showMax,omitzero"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	extras
}

// ResourceCollectionBlock lists resources, by Items or by Filter.
type ResourceCollectionBlock struct {
	ID            string                    `json:"id,omitempty"`
	Items         ResourceItems             `json:"items,omitzero"`
	Filter        *Filter                   `json:"filter,omitempty"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	extras
}

// ResourceLinkBlock is a list of links to resources.
type ResourceLinkBlock struct {
	ID            string                    `json:"id,omitempty"`
	Items         ResourceItems             `json:"items"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	extras
}

// MapBlock draws a map document of the project. Set both ResourceID and
// DocumentID, or neither for an empty "Link a map" block.
type MapBlock struct {
	ID            string                    `json:"id,omitempty"`
	ResourceID    opt.Optional[string]      `json:"resourceId,omitzero"`
	DocumentID    opt.Optional[string]      `json:"documentId,omitzero"`
	Viewport      *Viewport                 `json:"viewport,omitempty"`
	DisplayHeight opt.Optional[float64]     `json:"displayHeight,omitzero"`
	Fill          opt.Optional[bool]        `json:"fill,omitzero"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	extras
}

// SecretBlock is content hidden from players. Only admins can read or write it;
// for anyone else the server omits it on read and refuses a write that
// holds or removes one.
type SecretBlock struct {
	ID       string                    `json:"id,omitempty"`
	Layout   opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width    opt.Optional[int]         `json:"width,omitzero"`
	Children List                      `json:"children"`
	extras
}

// TextFieldBlock is a rich-text container, used mostly as a property body.
type TextFieldBlock struct {
	ID            string                    `json:"id,omitempty"`
	Title         opt.Optional[string]      `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]        `json:"isTitleHidden,omitzero"`
	Layout        opt.Optional[BlockLayout] `json:"layout,omitzero"`
	Width         opt.Optional[int]         `json:"width,omitzero"`
	Children      List                      `json:"children"`
	extras
}

func (b *ParagraphBlock) Kind() Kind     { return KindParagraph }
func (b *ParagraphBlock) NodeID() string { return b.ID }
func (*ParagraphBlock) isBlock()         {}

func (b *ParagraphBlock) MarshalJSON() ([]byte, error) {
	type plain ParagraphBlock
	return encodeObject(string(KindParagraph), (*plain)(b), b.m)
}

func (b *ParagraphBlock) UnmarshalJSON(data []byte) error {
	type plain ParagraphBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *HeadingBlock) Kind() Kind     { return KindHeading }
func (b *HeadingBlock) NodeID() string { return b.ID }
func (*HeadingBlock) isBlock()         {}

func (b *HeadingBlock) MarshalJSON() ([]byte, error) {
	type plain HeadingBlock
	return encodeObject(string(KindHeading), (*plain)(b), b.m)
}

func (b *HeadingBlock) UnmarshalJSON(data []byte) error {
	type plain HeadingBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *QuoteBlock) Kind() Kind     { return KindQuote }
func (b *QuoteBlock) NodeID() string { return b.ID }
func (*QuoteBlock) isBlock()         {}

func (b *QuoteBlock) MarshalJSON() ([]byte, error) {
	type plain QuoteBlock
	return encodeObject(string(KindQuote), (*plain)(b), b.m)
}

func (b *QuoteBlock) UnmarshalJSON(data []byte) error {
	type plain QuoteBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *QuoteBlock) childList() *List { return &b.Children }

func (b *CodeBlock) Kind() Kind     { return KindCode }
func (b *CodeBlock) NodeID() string { return b.ID }
func (*CodeBlock) isBlock()         {}

func (b *CodeBlock) MarshalJSON() ([]byte, error) {
	type plain CodeBlock
	return encodeObject(string(KindCode), (*plain)(b), b.m)
}

func (b *CodeBlock) UnmarshalJSON(data []byte) error {
	type plain CodeBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *RuleBlock) Kind() Kind     { return KindRule }
func (b *RuleBlock) NodeID() string { return b.ID }
func (*RuleBlock) isBlock()         {}

func (b *RuleBlock) MarshalJSON() ([]byte, error) {
	type plain RuleBlock
	return encodeObject(string(KindRule), (*plain)(b), b.m)
}

func (b *RuleBlock) UnmarshalJSON(data []byte) error {
	type plain RuleBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *BulletListBlock) Kind() Kind     { return KindBulletList }
func (b *BulletListBlock) NodeID() string { return b.ID }
func (*BulletListBlock) isBlock()         {}

func (b *BulletListBlock) MarshalJSON() ([]byte, error) {
	type plain BulletListBlock
	return encodeObject(string(KindBulletList), (*plain)(b), b.m)
}

func (b *BulletListBlock) UnmarshalJSON(data []byte) error {
	type plain BulletListBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *BulletListBlock) childList() *List { return &b.Children }

func (b *OrderedListBlock) Kind() Kind     { return KindOrderedList }
func (b *OrderedListBlock) NodeID() string { return b.ID }
func (*OrderedListBlock) isBlock()         {}

func (b *OrderedListBlock) MarshalJSON() ([]byte, error) {
	type plain OrderedListBlock
	return encodeObject(string(KindOrderedList), (*plain)(b), b.m)
}

func (b *OrderedListBlock) UnmarshalJSON(data []byte) error {
	type plain OrderedListBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *OrderedListBlock) childList() *List { return &b.Children }

func (b *ListItemBlock) Kind() Kind     { return KindListItem }
func (b *ListItemBlock) NodeID() string { return b.ID }
func (*ListItemBlock) isBlock()         {}

func (b *ListItemBlock) MarshalJSON() ([]byte, error) {
	type plain ListItemBlock
	return encodeObject(string(KindListItem), (*plain)(b), b.m)
}

func (b *ListItemBlock) UnmarshalJSON(data []byte) error {
	type plain ListItemBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *ListItemBlock) childList() *List { return &b.Children }

func (b *TaskListBlock) Kind() Kind     { return KindTaskList }
func (b *TaskListBlock) NodeID() string { return b.ID }
func (*TaskListBlock) isBlock()         {}

func (b *TaskListBlock) MarshalJSON() ([]byte, error) {
	type plain TaskListBlock
	return encodeObject(string(KindTaskList), (*plain)(b), b.m)
}

func (b *TaskListBlock) UnmarshalJSON(data []byte) error {
	type plain TaskListBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *TaskListBlock) childList() *List { return &b.Children }

func (b *TaskItemBlock) Kind() Kind     { return KindTaskItem }
func (b *TaskItemBlock) NodeID() string { return b.ID }
func (*TaskItemBlock) isBlock()         {}

func (b *TaskItemBlock) MarshalJSON() ([]byte, error) {
	type plain TaskItemBlock
	return encodeObject(string(KindTaskItem), (*plain)(b), b.m)
}

func (b *TaskItemBlock) UnmarshalJSON(data []byte) error {
	type plain TaskItemBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *TableBlock) Kind() Kind     { return KindTable }
func (b *TableBlock) NodeID() string { return b.ID }
func (*TableBlock) isBlock()         {}

func (b *TableBlock) MarshalJSON() ([]byte, error) {
	type plain TableBlock
	return encodeObject(string(KindTable), (*plain)(b), b.m)
}

func (b *TableBlock) UnmarshalJSON(data []byte) error {
	type plain TableBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *TableBlock) childList() *List { return &b.Children }

func (b *TableRowBlock) Kind() Kind     { return KindTableRow }
func (b *TableRowBlock) NodeID() string { return b.ID }
func (*TableRowBlock) isBlock()         {}

func (b *TableRowBlock) MarshalJSON() ([]byte, error) {
	type plain TableRowBlock
	return encodeObject(string(KindTableRow), (*plain)(b), b.m)
}

func (b *TableRowBlock) UnmarshalJSON(data []byte) error {
	type plain TableRowBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *TableRowBlock) childList() *List { return &b.Children }

func (b *TableCellBlock) Kind() Kind     { return KindTableCell }
func (b *TableCellBlock) NodeID() string { return b.ID }
func (*TableCellBlock) isBlock()         {}

func (b *TableCellBlock) MarshalJSON() ([]byte, error) {
	type plain TableCellBlock
	return encodeObject(string(KindTableCell), (*plain)(b), b.m)
}

func (b *TableCellBlock) UnmarshalJSON(data []byte) error {
	type plain TableCellBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *TableCellBlock) childList() *List { return &b.Children }

func (b *CalloutBlock) Kind() Kind     { return KindCallout }
func (b *CalloutBlock) NodeID() string { return b.ID }
func (*CalloutBlock) isBlock()         {}

func (b *CalloutBlock) MarshalJSON() ([]byte, error) {
	type plain CalloutBlock
	return encodeObject(string(KindCallout), (*plain)(b), b.m)
}

func (b *CalloutBlock) UnmarshalJSON(data []byte) error {
	type plain CalloutBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *CalloutBlock) childList() *List { return &b.Children }

func (b *ExpandBlock) Kind() Kind     { return KindExpand }
func (b *ExpandBlock) NodeID() string { return b.ID }
func (*ExpandBlock) isBlock()         {}

func (b *ExpandBlock) MarshalJSON() ([]byte, error) {
	type plain ExpandBlock
	return encodeObject(string(KindExpand), (*plain)(b), b.m)
}

func (b *ExpandBlock) UnmarshalJSON(data []byte) error {
	type plain ExpandBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *ExpandBlock) childList() *List { return &b.Children }

func (b *LayoutBlock) Kind() Kind     { return KindLayout }
func (b *LayoutBlock) NodeID() string { return b.ID }
func (*LayoutBlock) isBlock()         {}

func (b *LayoutBlock) MarshalJSON() ([]byte, error) {
	type plain LayoutBlock
	return encodeObject(string(KindLayout), (*plain)(b), b.m)
}

func (b *LayoutBlock) UnmarshalJSON(data []byte) error {
	type plain LayoutBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *LayoutBlock) childList() *List { return &b.Children }

func (b *LayoutColumnBlock) Kind() Kind     { return KindLayoutColumn }
func (b *LayoutColumnBlock) NodeID() string { return b.ID }
func (*LayoutColumnBlock) isBlock()         {}

func (b *LayoutColumnBlock) MarshalJSON() ([]byte, error) {
	type plain LayoutColumnBlock
	return encodeObject(string(KindLayoutColumn), (*plain)(b), b.m)
}

func (b *LayoutColumnBlock) UnmarshalJSON(data []byte) error {
	type plain LayoutColumnBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *ImageBlock) Kind() Kind     { return KindImage }
func (b *ImageBlock) NodeID() string { return b.ID }
func (*ImageBlock) isBlock()         {}

func (b *ImageBlock) MarshalJSON() ([]byte, error) {
	type plain ImageBlock
	return encodeObject(string(KindImage), (*plain)(b), b.m)
}

func (b *ImageBlock) UnmarshalJSON(data []byte) error {
	type plain ImageBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *EmbedBlock) Kind() Kind     { return KindEmbed }
func (b *EmbedBlock) NodeID() string { return b.ID }
func (*EmbedBlock) isBlock()         {}

func (b *EmbedBlock) MarshalJSON() ([]byte, error) {
	type plain EmbedBlock
	return encodeObject(string(KindEmbed), (*plain)(b), b.m)
}

func (b *EmbedBlock) UnmarshalJSON(data []byte) error {
	type plain EmbedBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *KeyValueBlock) Kind() Kind     { return KindKeyValue }
func (b *KeyValueBlock) NodeID() string { return b.ID }
func (*KeyValueBlock) isBlock()         {}

func (b *KeyValueBlock) MarshalJSON() ([]byte, error) {
	type plain KeyValueBlock
	return encodeObject(string(KindKeyValue), (*plain)(b), b.m)
}

func (b *KeyValueBlock) UnmarshalJSON(data []byte) error {
	type plain KeyValueBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *InfoboxBlock) Kind() Kind     { return KindInfobox }
func (b *InfoboxBlock) NodeID() string { return b.ID }
func (*InfoboxBlock) isBlock()         {}

func (b *InfoboxBlock) MarshalJSON() ([]byte, error) {
	type plain InfoboxBlock
	return encodeObject(string(KindInfobox), (*plain)(b), b.m)
}

func (b *InfoboxBlock) UnmarshalJSON(data []byte) error {
	type plain InfoboxBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *MeterBlock) Kind() Kind     { return KindMeter }
func (b *MeterBlock) NodeID() string { return b.ID }
func (*MeterBlock) isBlock()         {}

func (b *MeterBlock) MarshalJSON() ([]byte, error) {
	type plain MeterBlock
	return encodeObject(string(KindMeter), (*plain)(b), b.m)
}

func (b *MeterBlock) UnmarshalJSON(data []byte) error {
	type plain MeterBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *ResourceCollectionBlock) Kind() Kind     { return KindResourceCollection }
func (b *ResourceCollectionBlock) NodeID() string { return b.ID }
func (*ResourceCollectionBlock) isBlock()         {}

func (b *ResourceCollectionBlock) MarshalJSON() ([]byte, error) {
	type plain ResourceCollectionBlock
	return encodeObject(string(KindResourceCollection), (*plain)(b), b.m)
}

func (b *ResourceCollectionBlock) UnmarshalJSON(data []byte) error {
	type plain ResourceCollectionBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *ResourceLinkBlock) Kind() Kind     { return KindResourceLink }
func (b *ResourceLinkBlock) NodeID() string { return b.ID }
func (*ResourceLinkBlock) isBlock()         {}

func (b *ResourceLinkBlock) MarshalJSON() ([]byte, error) {
	type plain ResourceLinkBlock
	return encodeObject(string(KindResourceLink), (*plain)(b), b.m)
}

func (b *ResourceLinkBlock) UnmarshalJSON(data []byte) error {
	type plain ResourceLinkBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *MapBlock) Kind() Kind     { return KindMap }
func (b *MapBlock) NodeID() string { return b.ID }
func (*MapBlock) isBlock()         {}

func (b *MapBlock) MarshalJSON() ([]byte, error) {
	type plain MapBlock
	return encodeObject(string(KindMap), (*plain)(b), b.m)
}

func (b *MapBlock) UnmarshalJSON(data []byte) error {
	type plain MapBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}

func (b *SecretBlock) Kind() Kind     { return KindSecret }
func (b *SecretBlock) NodeID() string { return b.ID }
func (*SecretBlock) isBlock()         {}

func (b *SecretBlock) MarshalJSON() ([]byte, error) {
	type plain SecretBlock
	return encodeObject(string(KindSecret), (*plain)(b), b.m)
}

func (b *SecretBlock) UnmarshalJSON(data []byte) error {
	type plain SecretBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *SecretBlock) childList() *List { return &b.Children }

func (b *TextFieldBlock) Kind() Kind     { return KindTextField }
func (b *TextFieldBlock) NodeID() string { return b.ID }
func (*TextFieldBlock) isBlock()         {}

func (b *TextFieldBlock) MarshalJSON() ([]byte, error) {
	type plain TextFieldBlock
	return encodeObject(string(KindTextField), (*plain)(b), b.m)
}

func (b *TextFieldBlock) UnmarshalJSON(data []byte) error {
	type plain TextFieldBlock
	return decodeObject(data, (*plain)(b), &b.extras, "kind")
}
func (b *ParagraphBlock) setID(id string)          { b.ID = id }
func (b *HeadingBlock) setID(id string)            { b.ID = id }
func (b *QuoteBlock) setID(id string)              { b.ID = id }
func (b *CodeBlock) setID(id string)               { b.ID = id }
func (b *RuleBlock) setID(id string)               { b.ID = id }
func (b *BulletListBlock) setID(id string)         { b.ID = id }
func (b *OrderedListBlock) setID(id string)        { b.ID = id }
func (b *ListItemBlock) setID(id string)           { b.ID = id }
func (b *TaskListBlock) setID(id string)           { b.ID = id }
func (b *TaskItemBlock) setID(id string)           { b.ID = id }
func (b *TableBlock) setID(id string)              { b.ID = id }
func (b *TableRowBlock) setID(id string)           { b.ID = id }
func (b *TableCellBlock) setID(id string)          { b.ID = id }
func (b *CalloutBlock) setID(id string)            { b.ID = id }
func (b *ExpandBlock) setID(id string)             { b.ID = id }
func (b *LayoutBlock) setID(id string)             { b.ID = id }
func (b *LayoutColumnBlock) setID(id string)       { b.ID = id }
func (b *ImageBlock) setID(id string)              { b.ID = id }
func (b *EmbedBlock) setID(id string)              { b.ID = id }
func (b *KeyValueBlock) setID(id string)           { b.ID = id }
func (b *InfoboxBlock) setID(id string)            { b.ID = id }
func (b *MeterBlock) setID(id string)              { b.ID = id }
func (b *ResourceCollectionBlock) setID(id string) { b.ID = id }
func (b *ResourceLinkBlock) setID(id string)       { b.ID = id }
func (b *MapBlock) setID(id string)                { b.ID = id }
func (b *SecretBlock) setID(id string)             { b.ID = id }
func (b *TextFieldBlock) setID(id string)          { b.ID = id }
func (b *LayoutColumnBlock) childList() *List      { return &b.Children }
func (b *TextFieldBlock) childList() *List         { return &b.Children }
