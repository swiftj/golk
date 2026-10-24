package blocks

import (
	"encoding/json"
	"fmt"

	"github.com/swiftj/golk/lk/internal/opt"
)

// Run is one piece of inline text: *Text, *Mention, *Break, or *RawRun for
// a shape this package does not know.
//
// The server normalizes runs on write: a run whose text is empty is dropped,
// and adjacent runs with identical marks are merged. A page read back is
// therefore not always run-for-run what was written.
type Run interface {
	json.Marshaler
	isRun()
}

// Text is a run of text with optional marks. A Code (mono) run carries no
// other mark, color or highlight, and a run has at most one of Sub and Sup.
type Text struct {
	Text      string               `json:"text"`
	Bold      bool                 `json:"bold,omitempty"`
	Italic    bool                 `json:"italic,omitempty"`
	Underline bool                 `json:"underline,omitempty"`
	Strike    bool                 `json:"strike,omitempty"`
	Code      bool                 `json:"code,omitempty"`
	Link      opt.Optional[string] `json:"link,omitzero"`
	Sub       bool                 `json:"sub,omitempty"`
	Sup       bool                 `json:"sup,omitempty"`
	// Color and Highlight are #RRGGBB.
	Color     opt.Optional[string] `json:"color,omitzero"`
	Highlight opt.Optional[string] `json:"highlight,omitzero"`
	extras
}

func (*Text) isRun() {}

func (r *Text) MarshalJSON() ([]byte, error) {
	type plain Text
	return encodeObject("", (*plain)(r), r.m)
}

func (r *Text) UnmarshalJSON(data []byte) error {
	type plain Text
	return decodeObject(data, (*plain)(r), &r.extras)
}

// Mention is a link to a resource (and optionally one of its documents)
// that the app draws with the resource's name or Label.
type Mention struct {
	ResourceID string
	DocumentID opt.Optional[string]
	// Label is the mention's text label. Without one the app shows Name.
	Label opt.Optional[string]
	// Name is the resource's current name. It is read-only: the server
	// fills it in, and echoing it back is accepted and ignored (D-009, A-24).
	Name      opt.Optional[string]
	Color     opt.Optional[string]
	Highlight opt.Optional[string]
	extras
	refExtra map[string]json.RawMessage
}

type mentionRef struct {
	ResourceID string               `json:"resourceId"`
	DocumentID opt.Optional[string] `json:"documentId,omitzero"`
	Text       opt.Optional[string] `json:"text,omitzero"`
	Name       opt.Optional[string] `json:"name,omitzero"`
	extras
}

func (r *mentionRef) MarshalJSON() ([]byte, error) {
	type plain mentionRef
	return encodeObject("", (*plain)(r), r.m)
}

func (r *mentionRef) UnmarshalJSON(data []byte) error {
	type plain mentionRef
	return decodeObject(data, (*plain)(r), &r.extras)
}

type mentionWire struct {
	Mention   *mentionRef          `json:"mention"`
	Color     opt.Optional[string] `json:"color,omitzero"`
	Highlight opt.Optional[string] `json:"highlight,omitzero"`
}

func (*Mention) isRun() {}

func (r *Mention) MarshalJSON() ([]byte, error) {
	ref := &mentionRef{ResourceID: r.ResourceID, DocumentID: r.DocumentID, Text: r.Label, Name: r.Name}
	ref.m = r.refExtra
	return encodeObject("", &mentionWire{Mention: ref, Color: r.Color, Highlight: r.Highlight}, r.m)
}

func (r *Mention) UnmarshalJSON(data []byte) error {
	var w mentionWire
	var e extras
	if err := decodeObject(data, &w, &e); err != nil {
		return err
	}
	if w.Mention == nil {
		return fmt.Errorf("blocks: mention run without a mention")
	}
	*r = Mention{
		ResourceID: w.Mention.ResourceID, DocumentID: w.Mention.DocumentID,
		Label: w.Mention.Text, Name: w.Mention.Name,
		Color: w.Color, Highlight: w.Highlight,
		extras: e, refExtra: w.Mention.m,
	}
	return nil
}

// Break is a line break inside a block's text.
type Break struct{}

func (*Break) isRun()                       {}
func (*Break) MarshalJSON() ([]byte, error) { return []byte(`{"break":true}`), nil }

// RawRun is a run of a shape this package does not know. It keeps its JSON
// and writes it back unchanged.
type RawRun struct {
	JSON json.RawMessage
}

func (*RawRun) isRun()                         {}
func (r *RawRun) MarshalJSON() ([]byte, error) { return r.JSON, nil }

// Runs is a block's inline content. It encodes a nil Runs as [].
type Runs []Run

// MarshalJSON encodes rs, with nil as [].
func (rs Runs) MarshalJSON() ([]byte, error) {
	if rs == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]Run(rs))
}

// UnmarshalJSON decodes each run by the key it carries: "mention", "break"
// or "text". Anything else is kept as a *RawRun.
func (rs *Runs) UnmarshalJSON(data []byte) error {
	items, err := arrayItems(data)
	if err != nil {
		return err
	}
	out := make(Runs, 0, len(items))
	for i, raw := range items {
		r, err := unmarshalRun(raw)
		if err != nil {
			return fmt.Errorf("[%d]: %w", i, err)
		}
		out = append(out, r)
	}
	*rs = out
	return nil
}

func unmarshalRun(data []byte) (Run, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return nil, err
	}
	switch {
	case keys["mention"] != nil:
		r := new(Mention)
		return r, json.Unmarshal(data, r)
	case keys["break"] != nil:
		if len(keys) == 1 && string(keys["break"]) == "true" {
			return &Break{}, nil
		}
	case keys["text"] != nil:
		r := new(Text)
		if err := json.Unmarshal(data, r); err == nil {
			return r, nil
		}
	}
	return &RawRun{JSON: append(json.RawMessage(nil), data...)}, nil
}
