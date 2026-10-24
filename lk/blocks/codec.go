package blocks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"
)

// extras holds fields the server sent that this package does not model.
// They are written back unchanged, so a read-modify-write never drops them
// (DECISIONS.md D-008, A-32). Callers cannot set them.
type extras struct {
	m map[string]json.RawMessage
}

// knownFields caches the JSON names each struct type declares.
var knownFields sync.Map // reflect.Type -> map[string]bool

func fieldsOf(t reflect.Type) map[string]bool {
	if v, ok := knownFields.Load(t); ok {
		return v.(map[string]bool)
	}
	m := map[string]bool{}
	var walk func(t reflect.Type)
	walk = func(t reflect.Type) {
		for i := range t.NumField() {
			f := t.Field(i)
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, _, _ := strings.Cut(tag, ",")
			if f.Anonymous && name == "" && f.Type.Kind() == reflect.Struct {
				walk(f.Type)
				continue
			}
			if !f.IsExported() {
				continue
			}
			if name == "" {
				name = f.Name
			}
			m[name] = true
		}
	}
	walk(t)
	knownFields.Store(t, m)
	return m
}

// decodeObject decodes data into v (a pointer to a method-free copy of a
// block or property struct) and records every field v does not declare,
// other than those in skip, in e.
func decodeObject(data []byte, v any, e *extras, skip ...string) error {
	if err := json.Unmarshal(data, v); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	known := fieldsOf(reflect.TypeOf(v).Elem())
	e.m = nil
	for k, raw := range all {
		if known[k] || slices.Contains(skip, k) {
			continue
		}
		if e.m == nil {
			e.m = map[string]json.RawMessage{}
		}
		e.m[k] = raw
	}
	return nil
}

// encodeObject encodes v (a method-free copy of a block or property
// struct), adding "kind" first when kind is not empty and the extra fields
// last, in name order.
func encodeObject(kind string, v any, extra map[string]json.RawMessage) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("blocks: %T does not encode as an object", v)
	}
	var b bytes.Buffer
	b.WriteByte('{')
	n := 0
	if kind != "" {
		b.WriteString(`"kind":`)
		k, _ := json.Marshal(kind)
		b.Write(k)
		n++
	}
	if inner := body[1 : len(body)-1]; len(inner) > 0 {
		if n > 0 {
			b.WriteByte(',')
		}
		b.Write(inner)
		n++
	}
	for _, k := range slices.Sorted(maps.Keys(extra)) {
		if n > 0 {
			b.WriteByte(',')
		}
		name, _ := json.Marshal(k)
		b.Write(name)
		b.WriteByte(':')
		b.Write(extra[k])
		n++
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// kindOf reads the "kind" field of a JSON object.
func kindOf(data []byte) (string, error) {
	var k struct {
		Kind *string `json:"kind"`
	}
	if err := json.Unmarshal(data, &k); err != nil {
		return "", err
	}
	if k.Kind == nil {
		return "", fmt.Errorf("blocks: object has no kind")
	}
	return *k.Kind, nil
}

// arrayItems splits a JSON array into its elements. null decodes as no
// elements.
func arrayItems(data []byte) ([]json.RawMessage, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}
