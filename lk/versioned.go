package lk

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Versioned is the result of an operation that reports the document's
// version: Value is what the operation returns, and Version is the document
// version after it. Pass Version to IfMatch on the next write. Versions are
// opaque; compare them only for equality.
type Versioned[T any] struct {
	Value   T
	Version string
}

// VersionedList is a single-page list that also reports the document's
// version, such as a keyValue block's rows.
type VersionedList[T any] struct {
	Items   []T
	Version string
}

// versionedBody receives a versioned response. It keeps the body without
// its top-level "version" (so the value decodes as if it were never there),
// and takes the version from the body or, failing that, the ETag.
type versionedBody struct {
	raw     json.RawMessage
	version string
}

func (v *versionedBody) UnmarshalJSON(b []byte) error {
	if !bytes.HasPrefix(bytes.TrimSpace(b), []byte("{")) {
		v.raw = append(json.RawMessage(nil), b...)
		return nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	if raw, ok := fields["version"]; ok {
		if err := json.Unmarshal(raw, &v.version); err != nil {
			return fmt.Errorf("version: %w", err)
		}
		delete(fields, "version")
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	v.raw = raw
	return nil
}

func (v *versionedBody) setVersionIfEmpty(s string) {
	if v.version == "" {
		v.version = s
	}
}

// singlePage wraps a list the server returns whole. A list that claims more
// items has no cursor to follow, so it reports ErrPaginationInconsistent
// rather than being silently truncated (DECISIONS.md D-008, A-22).
func singlePage[T any](items []T, hasMore bool) *Page[T] {
	return &Page[T]{Items: items, inconsistent: hasMore}
}
