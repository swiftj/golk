// Package opt defines the three-state field types shared by the generated wire
// types and the public lk package, which re-exports them as aliases.
//
// Both types are meant for struct fields tagged `json:",omitzero"`: an unset
// value reports IsZero and is left out of the encoded object.
package opt

import (
	"bytes"
	"encoding/json"
	"reflect"
)

var null = []byte("null")

// Optional is a value that is either omitted or set.
type Optional[T any] struct {
	v   T
	set bool
}

// Some returns an Optional set to v.
func Some[T any](v T) Optional[T] { return Optional[T]{v: v, set: true} }

// IsZero reports whether o is unset. encoding/json uses it for omitzero.
func (o Optional[T]) IsZero() bool { return !o.set }

// IsSet reports whether o holds a value.
func (o Optional[T]) IsSet() bool { return o.set }

// Get returns the value and whether it is set.
func (o Optional[T]) Get() (T, bool) { return o.v, o.set }

// Or returns the value, or def when o is unset.
func (o Optional[T]) Or(def T) T {
	if !o.set {
		return def
	}
	return o.v
}

// MarshalJSON encodes the value. An unset Optional must be omitted by the
// enclosing struct with omitzero; encoded on its own it is the zero value.
//
// An Optional never encodes null, because null is a separate state that only
// Nullable carries: a set nil slice encodes as [] and a set nil map as {}.
func (o Optional[T]) MarshalJSON() ([]byte, error) {
	switch rv := reflect.ValueOf(&o.v).Elem(); rv.Kind() {
	case reflect.Slice:
		if rv.IsNil() {
			return []byte("[]"), nil
		}
	case reflect.Map:
		if rv.IsNil() {
			return []byte("{}"), nil
		}
	}
	return json.Marshal(o.v)
}

// UnmarshalJSON marks o set and decodes into it. A JSON null leaves o unset,
// because an Optional field has no null state.
func (o *Optional[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), null) {
		*o = Optional[T]{}
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*o = Optional[T]{v: v, set: true}
	return nil
}

// Nullable is a value that is omitted, explicitly null, or set.
type Nullable[T any] struct {
	v    T
	set  bool
	null bool
}

// Value returns a Nullable set to v.
func Value[T any](v T) Nullable[T] { return Nullable[T]{v: v, set: true} }

// Null returns a Nullable that encodes as JSON null.
func Null[T any]() Nullable[T] { return Nullable[T]{set: true, null: true} }

// IsZero reports whether n is omitted. encoding/json uses it for omitzero.
func (n Nullable[T]) IsZero() bool { return !n.set }

// IsNull reports whether n is explicitly null.
func (n Nullable[T]) IsNull() bool { return n.set && n.null }

// IsSet reports whether n holds a non-null value.
func (n Nullable[T]) IsSet() bool { return n.set && !n.null }

// Get returns the value and whether n holds a non-null value.
func (n Nullable[T]) Get() (T, bool) { return n.v, n.set && !n.null }

// Or returns the value, or def when n is omitted or null.
func (n Nullable[T]) Or(def T) T {
	if !n.IsSet() {
		return def
	}
	return n.v
}

// MarshalJSON encodes the value, or null.
func (n Nullable[T]) MarshalJSON() ([]byte, error) {
	if n.null {
		return null, nil
	}
	return json.Marshal(n.v)
}

// UnmarshalJSON marks n set, and null when the input is JSON null.
func (n *Nullable[T]) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), null) {
		*n = Nullable[T]{set: true, null: true}
		return nil
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*n = Nullable[T]{v: v, set: true}
	return nil
}
