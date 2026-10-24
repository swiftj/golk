package blocks

import (
	"encoding/json"
	"fmt"

	"github.com/swiftj/golk/lk/internal/opt"
)

// Property is a sidebar property or an infobox row: a property-capable
// block plus an Envelope. It is one of the *XxxProperty types,
// *UnsupportedProperty, or *RawProperty for a kind this package does not
// know.
type Property interface {
	Node
	json.Marshaler
	// PropertyEnvelope returns the property's envelope, for reading or
	// changing it in place. It is nil for a *RawProperty.
	PropertyEnvelope() *Envelope
	isProperty()
}

// Envelope holds the fields every property has. Title and IsTitleHidden
// live here rather than on the block. Rank orders properties; it is set by
// the server and ignored on create (place a new property with before/after).
type Envelope struct {
	ID            string               `json:"id,omitempty"`
	Rank          string               `json:"rank,omitempty"`
	Title         opt.Optional[string] `json:"title,omitzero"`
	IsTitleHidden opt.Optional[bool]   `json:"isTitleHidden,omitzero"`
	IsHidden      opt.Optional[bool]   `json:"isHidden,omitzero"`
}

// NodeID returns the property id.
func (e *Envelope) NodeID() string { return e.ID }

// PropertyEnvelope returns e.
func (e *Envelope) PropertyEnvelope() *Envelope { return e }

// Properties is a list of properties. It encodes nil as [].
type Properties []Property

// MarshalJSON encodes ps, with nil as [].
func (ps Properties) MarshalJSON() ([]byte, error) {
	if ps == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Property(ps))
}

// UnmarshalJSON decodes a JSON array of properties by kind.
func (ps *Properties) UnmarshalJSON(data []byte) error {
	items, err := arrayItems(data)
	if err != nil {
		return err
	}
	out := make(Properties, 0, len(items))
	for i, raw := range items {
		p, err := UnmarshalProperty(raw)
		if err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
		out = append(out, p)
	}
	*ps = out
	return nil
}

// UnmarshalProperty decodes one property by its kind. An unknown kind
// decodes to *RawProperty.
func UnmarshalProperty(data []byte) (Property, error) {
	kind, err := kindOf(data)
	if err != nil {
		return nil, err
	}
	newProp, ok := propertyKinds[Kind(kind)]
	if !ok {
		return &RawProperty{KindName: Kind(kind), JSON: append(json.RawMessage(nil), data...)}, nil
	}
	p := newProp()
	if err := json.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("%s: %w", kind, err)
	}
	return p, nil
}

var propertyKinds = map[Kind]func() Property{
	KindTextField:          func() Property { return new(TextFieldProperty) },
	KindKeyValue:           func() Property { return new(KeyValueProperty) },
	KindImage:              func() Property { return new(ImageProperty) },
	KindMeter:              func() Property { return new(MeterProperty) },
	KindResourceCollection: func() Property { return new(ResourceCollectionProperty) },
	KindResourceLink:       func() Property { return new(ResourceLinkProperty) },
	KindEmbed:              func() Property { return new(EmbedProperty) },
	KindMap:                func() Property { return new(MapProperty) },
	KindUnsupported:        func() Property { return new(UnsupportedProperty) },
}

// RawProperty is a property of a kind this package does not know. It keeps
// the JSON it was read from and writes it back unchanged.
type RawProperty struct {
	KindName Kind
	JSON     json.RawMessage
}

func (p *RawProperty) Kind() Kind                   { return p.KindName }
func (*RawProperty) PropertyEnvelope() *Envelope    { return nil }
func (*RawProperty) isProperty()                    {}
func (p *RawProperty) MarshalJSON() ([]byte, error) { return p.JSON, nil }

// NodeID returns the raw property's "id", if it has one.
func (p *RawProperty) NodeID() string {
	var v struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(p.JSON, &v)
	return v.ID
}

// TextFieldProperty is a rich-text property.
type TextFieldProperty struct {
	Envelope
	Children List `json:"children"`
	extras
}

// KeyValueProperty is a property of key-value rows.
type KeyValueProperty struct {
	Envelope
	Display opt.Optional[KeyValueDisplay] `json:"display,omitzero"`
	Rows    KVRows                        `json:"rows"`
	extras
}

// ImageProperty is an image property, such as a portrait.
type ImageProperty struct {
	Envelope
	URL  string               `json:"url"`
	Alt  opt.Optional[string] `json:"alt,omitzero"`
	Fill opt.Optional[bool]   `json:"fill,omitzero"`
	extras
}

// MeterProperty is a meter property.
type MeterProperty struct {
	Envelope
	Mode     MeterMode          `json:"mode"`
	Items    MeterItems         `json:"items"`
	ShowText opt.Optional[bool] `json:"showText,omitzero"`
	ShowIcon opt.Optional[bool] `json:"showIcon,omitzero"`
	ShowMax  opt.Optional[bool] `json:"showMax,omitzero"`
	extras
}

// ResourceCollectionProperty is a collection property.
type ResourceCollectionProperty struct {
	Envelope
	Items  ResourceItems `json:"items,omitzero"`
	Filter *Filter       `json:"filter,omitempty"`
	extras
}

// ResourceLinkProperty is a link-list property.
type ResourceLinkProperty struct {
	Envelope
	Items ResourceItems `json:"items"`
	extras
}

// EmbedProperty is an embedded-player property.
type EmbedProperty struct {
	Envelope
	Provider EmbedProvider `json:"provider"`
	URL      string        `json:"url"`
	extras
}

// MapProperty is a map property.
type MapProperty struct {
	Envelope
	ResourceID    opt.Optional[string]  `json:"resourceId,omitzero"`
	DocumentID    opt.Optional[string]  `json:"documentId,omitzero"`
	Viewport      *Viewport             `json:"viewport,omitempty"`
	DisplayHeight opt.Optional[float64] `json:"displayHeight,omitzero"`
	Fill          opt.Optional[bool]    `json:"fill,omitzero"`
	extras
}

// UnsupportedProperty is a property the public model cannot express. Echo
// it unchanged to keep it; only its envelope can be patched.
type UnsupportedProperty struct {
	Envelope
	extras
}

func (p *TextFieldProperty) Kind() Kind { return KindTextField }
func (*TextFieldProperty) isProperty()  {}

func (p *TextFieldProperty) MarshalJSON() ([]byte, error) {
	type plain TextFieldProperty
	return encodeObject(string(KindTextField), (*plain)(p), p.m)
}

func (p *TextFieldProperty) UnmarshalJSON(data []byte) error {
	type plain TextFieldProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}
func (p *TextFieldProperty) childList() *List { return &p.Children }

func (p *KeyValueProperty) Kind() Kind { return KindKeyValue }
func (*KeyValueProperty) isProperty()  {}

func (p *KeyValueProperty) MarshalJSON() ([]byte, error) {
	type plain KeyValueProperty
	return encodeObject(string(KindKeyValue), (*plain)(p), p.m)
}

func (p *KeyValueProperty) UnmarshalJSON(data []byte) error {
	type plain KeyValueProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *ImageProperty) Kind() Kind { return KindImage }
func (*ImageProperty) isProperty()  {}

func (p *ImageProperty) MarshalJSON() ([]byte, error) {
	type plain ImageProperty
	return encodeObject(string(KindImage), (*plain)(p), p.m)
}

func (p *ImageProperty) UnmarshalJSON(data []byte) error {
	type plain ImageProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *MeterProperty) Kind() Kind { return KindMeter }
func (*MeterProperty) isProperty()  {}

func (p *MeterProperty) MarshalJSON() ([]byte, error) {
	type plain MeterProperty
	return encodeObject(string(KindMeter), (*plain)(p), p.m)
}

func (p *MeterProperty) UnmarshalJSON(data []byte) error {
	type plain MeterProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *ResourceCollectionProperty) Kind() Kind { return KindResourceCollection }
func (*ResourceCollectionProperty) isProperty()  {}

func (p *ResourceCollectionProperty) MarshalJSON() ([]byte, error) {
	type plain ResourceCollectionProperty
	return encodeObject(string(KindResourceCollection), (*plain)(p), p.m)
}

func (p *ResourceCollectionProperty) UnmarshalJSON(data []byte) error {
	type plain ResourceCollectionProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *ResourceLinkProperty) Kind() Kind { return KindResourceLink }
func (*ResourceLinkProperty) isProperty()  {}

func (p *ResourceLinkProperty) MarshalJSON() ([]byte, error) {
	type plain ResourceLinkProperty
	return encodeObject(string(KindResourceLink), (*plain)(p), p.m)
}

func (p *ResourceLinkProperty) UnmarshalJSON(data []byte) error {
	type plain ResourceLinkProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *EmbedProperty) Kind() Kind { return KindEmbed }
func (*EmbedProperty) isProperty()  {}

func (p *EmbedProperty) MarshalJSON() ([]byte, error) {
	type plain EmbedProperty
	return encodeObject(string(KindEmbed), (*plain)(p), p.m)
}

func (p *EmbedProperty) UnmarshalJSON(data []byte) error {
	type plain EmbedProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *MapProperty) Kind() Kind { return KindMap }
func (*MapProperty) isProperty()  {}

func (p *MapProperty) MarshalJSON() ([]byte, error) {
	type plain MapProperty
	return encodeObject(string(KindMap), (*plain)(p), p.m)
}

func (p *MapProperty) UnmarshalJSON(data []byte) error {
	type plain MapProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}

func (p *UnsupportedProperty) Kind() Kind { return KindUnsupported }
func (*UnsupportedProperty) isProperty()  {}

func (p *UnsupportedProperty) MarshalJSON() ([]byte, error) {
	type plain UnsupportedProperty
	return encodeObject(string(KindUnsupported), (*plain)(p), p.m)
}

func (p *UnsupportedProperty) UnmarshalJSON(data []byte) error {
	type plain UnsupportedProperty
	return decodeObject(data, (*plain)(p), &p.extras, "kind")
}
