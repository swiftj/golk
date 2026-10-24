package lk

import (
	"encoding/json"
	"errors"

	"github.com/swiftj/golk/lk/blocks"
)

// ErrPatchConflict reports a patch that sets two fields the wire carries
// under one name, such as both MeterItems and ResourceItems ("items").
var ErrPatchConflict = errors.New("lk: patch sets two fields that share one wire field")

// BlockPatch is the body of Blocks.Update: the block's own fields. Only the
// fields that are set are sent and changed; the block's children stay as
// they are. Set only fields the block's kind has, or the server answers
// ErrValidation.
type BlockPatch struct {
	Content       Optional[blocks.Runs]            `json:"content,omitzero"`
	Level         Optional[int]                    `json:"level,omitzero"`
	Align         Optional[blocks.Align]           `json:"align,omitzero"`
	Background    Optional[string]                 `json:"background,omitzero"`
	Text          Optional[string]                 `json:"text,omitzero"`
	Language      Optional[string]                 `json:"language,omitzero"`
	Checked       Optional[bool]                   `json:"checked,omitzero"`
	Title         Optional[string]                 `json:"title,omitzero"`
	IsTitleHidden Optional[bool]                   `json:"isTitleHidden,omitzero"`
	Fill          Optional[bool]                   `json:"fill,omitzero"`
	Tone          Optional[blocks.Tone]            `json:"tone,omitzero"`
	Icon          Optional[string]                 `json:"icon,omitzero"`
	Color         Optional[string]                 `json:"color,omitzero"`
	Header        Optional[bool]                   `json:"header,omitzero"`
	Colspan       Optional[int]                    `json:"colspan,omitzero"`
	Rowspan       Optional[int]                    `json:"rowspan,omitzero"`
	Start         Optional[int]                    `json:"start,omitzero"`
	Width         Optional[float64]                `json:"width,omitzero"`
	URL           Optional[string]                 `json:"url,omitzero"`
	Alt           Optional[string]                 `json:"alt,omitzero"`
	Provider      Optional[blocks.EmbedProvider]   `json:"provider,omitzero"`
	Layout        Optional[blocks.BlockLayout]     `json:"layout,omitzero"`
	Display       Optional[blocks.KeyValueDisplay] `json:"display,omitzero"`
	// Rows (a keyValue block's rows) and InfoboxRows (an infobox's
	// properties) are both "rows" on the wire; set at most one.
	Rows        Optional[blocks.KVRows]     `json:"-"`
	InfoboxRows Optional[blocks.Properties] `json:"-"`
	Mode        Optional[blocks.MeterMode]  `json:"mode,omitzero"`
	// MeterItems and ResourceItems are both "items" on the wire; set at most
	// one.
	MeterItems    Optional[blocks.MeterItems]    `json:"-"`
	ResourceItems Optional[blocks.ResourceItems] `json:"-"`
	ShowText      Optional[bool]                 `json:"showText,omitzero"`
	ShowIcon      Optional[bool]                 `json:"showIcon,omitzero"`
	ShowMax       Optional[bool]                 `json:"showMax,omitzero"`
	Filter        Optional[blocks.Filter]        `json:"filter,omitzero"`
	ResourceID    Optional[string]               `json:"resourceId,omitzero"`
	DocumentID    Optional[string]               `json:"documentId,omitzero"`
	Viewport      Optional[blocks.Viewport]      `json:"viewport,omitzero"`
	DisplayHeight Optional[float64]              `json:"displayHeight,omitzero"`
}

// MarshalJSON encodes the fields that are set.
func (p BlockPatch) MarshalJSON() ([]byte, error) {
	type plain BlockPatch
	rows, err := either(p.Rows, p.InfoboxRows)
	if err != nil {
		return nil, err
	}
	items, err := either(p.MeterItems, p.ResourceItems)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		plain
		Rows  any `json:"rows,omitempty"`
		Items any `json:"items,omitempty"`
	}{plain(p), rows, items})
}

// PropertyPatch is the body of the UpdateProperty operations: envelope
// fields and the fields of the property's kind. Only the fields that are set
// are sent; setting Rank moves the property. Kind and id cannot be patched;
// on an unsupported property only the envelope can.
type PropertyPatch struct {
	Title         Optional[string]                 `json:"title,omitzero"`
	IsTitleHidden Optional[bool]                   `json:"isTitleHidden,omitzero"`
	Rank          Optional[string]                 `json:"rank,omitzero"`
	IsHidden      Optional[bool]                   `json:"isHidden,omitzero"`
	Children      Optional[blocks.List]            `json:"children,omitzero"`
	Display       Optional[blocks.KeyValueDisplay] `json:"display,omitzero"`
	Rows          Optional[blocks.KVRows]          `json:"rows,omitzero"`
	URL           Optional[string]                 `json:"url,omitzero"`
	Alt           Optional[string]                 `json:"alt,omitzero"`
	Fill          Optional[bool]                   `json:"fill,omitzero"`
	Mode          Optional[blocks.MeterMode]       `json:"mode,omitzero"`
	// MeterItems and ResourceItems are both "items" on the wire; set at most
	// one.
	MeterItems    Optional[blocks.MeterItems]    `json:"-"`
	ResourceItems Optional[blocks.ResourceItems] `json:"-"`
	ShowText      Optional[bool]                 `json:"showText,omitzero"`
	ShowIcon      Optional[bool]                 `json:"showIcon,omitzero"`
	ShowMax       Optional[bool]                 `json:"showMax,omitzero"`
	Filter        Optional[blocks.Filter]        `json:"filter,omitzero"`
	Provider      Optional[blocks.EmbedProvider] `json:"provider,omitzero"`
	ResourceID    Optional[string]               `json:"resourceId,omitzero"`
	DocumentID    Optional[string]               `json:"documentId,omitzero"`
	Viewport      Optional[blocks.Viewport]      `json:"viewport,omitzero"`
	DisplayHeight Optional[float64]              `json:"displayHeight,omitzero"`
}

// MarshalJSON encodes the fields that are set.
func (p PropertyPatch) MarshalJSON() ([]byte, error) {
	type plain PropertyPatch
	items, err := either(p.MeterItems, p.ResourceItems)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		plain
		Items any `json:"items,omitempty"`
	}{plain(p), items})
}

// either returns the value of whichever of a and b is set, nil when
// neither is, and ErrPatchConflict when both are.
func either[A, B any](a Optional[A], b Optional[B]) (any, error) {
	av, aok := a.Get()
	bv, bok := b.Get()
	switch {
	case aok && bok:
		return nil, ErrPatchConflict
	case aok:
		return av, nil
	case bok:
		return bv, nil
	}
	return nil, nil
}
