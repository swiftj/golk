package blocks

import (
	"encoding/json"

	"github.com/swiftj/golk/lk/internal/opt"
)

// KVRow is one key-value row: in a keyValue block or property, and the row
// type of the row operations. Write the value as Content runs or as plain
// Text; when both are sent Content wins, and reads return both. Rows are
// ordered by Rank.
type KVRow struct {
	ID      string               `json:"id,omitempty"`
	Rank    string               `json:"rank,omitempty"`
	Key     string               `json:"key"`
	Content Runs                 `json:"content,omitzero"`
	Text    opt.Optional[string] `json:"text,omitzero"`
	extras
}

// MeterItem is one tracked value of a meter. Color is #RRGGBB or "none"
// (the default) for the block tint.
type MeterItem struct {
	ID         string               `json:"id,omitempty"`
	Name       string               `json:"name"`
	Icon       string               `json:"icon"`
	RatingIcon opt.Optional[string] `json:"ratingIcon,omitzero"`
	Current    float64              `json:"current"`
	Max        float64              `json:"max"`
	Color      opt.Optional[string] `json:"color,omitzero"`
	extras
}

// ResourceItem is one resource in a collection or link list.
type ResourceItem struct {
	ID         string               `json:"id,omitempty"`
	Rank       string               `json:"rank,omitempty"`
	ResourceID string               `json:"resourceId"`
	DocumentID opt.Optional[string] `json:"documentId,omitzero"`
	extras
}

// Viewport is a map view: Center is [lat, lng] in the map's coordinate
// frame.
type Viewport struct {
	Center []float64 `json:"center"`
	Zoom   float64   `json:"zoom"`
	extras
}

// Filter is a collection filter predicate, which the app evaluates each
// time it draws the collection; the API only stores it. FilterType selects
// which other fields apply (see the guide's Collection filters). Type is
// always "filter" and is filled in when empty.
type Filter struct {
	Type         string                `json:"type"`
	ID           string                `json:"id"`
	FilterType   string                `json:"filterType"`
	Scope        opt.Optional[string]  `json:"scope,omitzero"`
	Field        opt.Optional[string]  `json:"field,omitzero"`
	Operator     opt.Optional[string]  `json:"operator,omitzero"`
	Value        json.RawMessage       `json:"value,omitzero"`
	IDs          []any                 `json:"ids,omitzero"`
	Tags         []string              `json:"tags,omitzero"`
	Predicate    opt.Optional[string]  `json:"predicate,omitzero"`
	Params       json.RawMessage       `json:"params,omitzero"`
	Start        opt.Optional[float64] `json:"start,omitzero"`
	End          opt.Optional[float64] `json:"end,omitzero"`
	PropertyType opt.Optional[string]  `json:"propertyType,omitzero"`
	Key          opt.Optional[string]  `json:"key,omitzero"`
	TargetID     opt.Optional[string]  `json:"targetId,omitzero"`
	Filters      []Filter              `json:"filters,omitzero"`
	extras
}

// KVRows is a list of rows. It encodes nil as [].
type KVRows []KVRow

// MeterItems is a meter's items. It encodes nil as [].
type MeterItems []MeterItem

// ResourceItems is a collection's or link list's items. It encodes nil as [].
type ResourceItems []ResourceItem

func (r KVRows) MarshalJSON() ([]byte, error)        { return encodeSlice([]KVRow(r)) }
func (r MeterItems) MarshalJSON() ([]byte, error)    { return encodeSlice([]MeterItem(r)) }
func (r ResourceItems) MarshalJSON() ([]byte, error) { return encodeSlice([]ResourceItem(r)) }

// encodeSlice encodes s with nil as [].
func encodeSlice[T any](s []T) ([]byte, error) {
	if s == nil {
		return []byte("[]"), nil
	}
	return json.Marshal(s)
}

func (b KVRow) MarshalJSON() ([]byte, error) {
	type plain KVRow
	return encodeObject("", plain(b), b.m)
}

func (b *KVRow) UnmarshalJSON(data []byte) error {
	type plain KVRow
	return decodeObject(data, (*plain)(b), &b.extras)
}

func (b MeterItem) MarshalJSON() ([]byte, error) {
	type plain MeterItem
	return encodeObject("", plain(b), b.m)
}

func (b *MeterItem) UnmarshalJSON(data []byte) error {
	type plain MeterItem
	return decodeObject(data, (*plain)(b), &b.extras)
}

func (b ResourceItem) MarshalJSON() ([]byte, error) {
	type plain ResourceItem
	return encodeObject("", plain(b), b.m)
}

func (b *ResourceItem) UnmarshalJSON(data []byte) error {
	type plain ResourceItem
	return decodeObject(data, (*plain)(b), &b.extras)
}

func (b Viewport) MarshalJSON() ([]byte, error) {
	type plain Viewport
	return encodeObject("", plain(b), b.m)
}

func (b *Viewport) UnmarshalJSON(data []byte) error {
	type plain Viewport
	return decodeObject(data, (*plain)(b), &b.extras)
}

func (b Filter) MarshalJSON() ([]byte, error) {
	if b.Type == "" {
		b.Type = "filter"
	}
	type plain Filter
	return encodeObject("", plain(b), b.m)
}

func (b *Filter) UnmarshalJSON(data []byte) error {
	type plain Filter
	return decodeObject(data, (*plain)(b), &b.extras)
}
