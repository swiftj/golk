package blocks

// Kind is a block or property kind, the "kind" field on the wire. A kind
// this package does not know is kept as is (DECISIONS.md D-008, A-28).
type Kind string

// Block kinds (S `Block`). The property-capable kinds are also property
// kinds.
const (
	KindParagraph          Kind = "paragraph"
	KindHeading            Kind = "heading"
	KindQuote              Kind = "quote"
	KindCode               Kind = "code"
	KindRule               Kind = "rule"
	KindBulletList         Kind = "bulletList"
	KindOrderedList        Kind = "orderedList"
	KindListItem           Kind = "listItem"
	KindTaskList           Kind = "taskList"
	KindTaskItem           Kind = "taskItem"
	KindTable              Kind = "table"
	KindTableRow           Kind = "tableRow"
	KindTableCell          Kind = "tableCell"
	KindCallout            Kind = "callout"
	KindExpand             Kind = "expand"
	KindLayout             Kind = "layout"
	KindLayoutColumn       Kind = "layoutColumn"
	KindImage              Kind = "image"
	KindEmbed              Kind = "embed"
	KindKeyValue           Kind = "keyValue"
	KindInfobox            Kind = "infobox"
	KindMeter              Kind = "meter"
	KindResourceCollection Kind = "resourceCollection"
	KindResourceLink       Kind = "resourceLink"
	KindMap                Kind = "map"
	KindSecret             Kind = "secret"
	KindTextField          Kind = "textField"
	KindUnsupported        Kind = "unsupported"
)

// BlockLayout is how the editor places a block that has a layout.
type BlockLayout string

// Layouts (S `BlockLayout`).
const (
	LayoutDefault    BlockLayout = "default"
	LayoutWrapLeft   BlockLayout = "wrapLeft"
	LayoutWrapRight  BlockLayout = "wrapRight"
	LayoutCenter     BlockLayout = "center"
	LayoutAlignStart BlockLayout = "alignStart"
	LayoutAlignEnd   BlockLayout = "alignEnd"
)

// Align is a paragraph or heading alignment. Reads omit AlignLeft, the
// default.
type Align string

// Alignments.
const (
	AlignLeft   Align = "left"
	AlignCenter Align = "center"
	AlignRight  Align = "right"
)

// Tone is a callout's tone. ToneCustom uses the callout's Icon and Color;
// the others use the app's icon and color for the tone.
type Tone string

// Callout tones.
const (
	ToneInfo    Tone = "info"
	ToneNote    Tone = "note"
	ToneSuccess Tone = "success"
	ToneWarning Tone = "warning"
	ToneError   Tone = "error"
	ToneCustom  Tone = "custom"
)

// EmbedProvider is the player an embed block draws.
type EmbedProvider string

// Embed providers.
const (
	EmbedYouTube    EmbedProvider = "youtube"
	EmbedSpotify    EmbedProvider = "spotify"
	EmbedSoundCloud EmbedProvider = "soundcloud"
)

// KeyValueDisplay is how a keyValue block or property draws its rows.
type KeyValueDisplay string

// Key-value displays.
const (
	DisplayList   KeyValueDisplay = "list"
	DisplayGrid   KeyValueDisplay = "grid"
	DisplayInline KeyValueDisplay = "inline"
)

// MeterMode is how a meter draws its items.
type MeterMode string

// Meter modes.
const (
	MeterBar      MeterMode = "bar"
	MeterWheel    MeterMode = "wheel"
	MeterGauge180 MeterMode = "gauge-180"
	MeterGauge270 MeterMode = "gauge-270"
	MeterPool     MeterMode = "pool"
	MeterRating   MeterMode = "rating"
)
