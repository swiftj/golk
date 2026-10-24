package markdown

import (
	"bytes"
	"encoding/json"
)

// Fields Markdown does not show (LK_MARKDOWN.md §3): block ids, a
// property's envelope id and rank, and the ids and ranks of key-value rows,
// meter items and resource items. An unsupported block or row keeps its id,
// which ::unsupported writes.

// hiddenStripped returns the JSON of v without the fields Markdown does not
// show, with object keys sorted, so two encodings compare equal exactly when
// v and the other value agree on everything Markdown shows.
func hiddenStripped(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	dropHidden(tree)
	return json.Marshal(tree)
}

// sameShown reports whether a and b agree on every field Markdown shows.
func sameShown(a, b any) bool {
	x, err := hiddenStripped(a)
	if err != nil {
		return false
	}
	y, err := hiddenStripped(b)
	return err == nil && bytes.Equal(x, y)
}

func dropHidden(v any) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			dropHidden(e)
		}
	case map[string]any:
		kind, hasKind := t["kind"].(string)
		if hasKind {
			if kind != "unsupported" {
				delete(t, "id")
			}
			delete(t, "rank")
		}
		for k, e := range t {
			if hasKind && rowKey(kind, k) {
				if rows, ok := e.([]any); ok {
					for _, r := range rows {
						if m, ok := r.(map[string]any); ok {
							delete(m, "id")
							delete(m, "rank")
						}
					}
				}
			}
			dropHidden(e)
		}
	}
}

// rowKey reports whether field k of a kind holds rows or items whose ids
// and ranks are hidden.
func rowKey(kind, k string) bool {
	switch k {
	case "rows":
		return kind == "keyValue"
	case "items":
		return kind == "meter" || kind == "resourceCollection" || kind == "resourceLink"
	}
	return false
}
