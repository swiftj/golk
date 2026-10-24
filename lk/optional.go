package lk

import "github.com/swiftj/golk/lk/internal/opt"

// Optional is a request field that is either omitted or set. The zero value is
// omitted, so a field the caller never touches is never sent.
//
//	lk.ResourceUpdate{Name: lk.Some("The Ice Kingdom")} // sends {"name":"The Ice Kingdom"}
type Optional[T any] = opt.Optional[T]

// Nullable is a request field that is omitted, explicitly null, or set.
//
//	lk.ResourceMove{ParentID: lk.Null[string]()} // sends {"parentId":null}: move to the root
type Nullable[T any] = opt.Nullable[T]

// Some returns an Optional set to v.
func Some[T any](v T) Optional[T] { return opt.Some(v) }

// Value returns a Nullable set to v.
func Value[T any](v T) Nullable[T] { return opt.Value(v) }

// Null returns a Nullable that is sent as JSON null.
func Null[T any]() Nullable[T] { return opt.Null[T]() }
