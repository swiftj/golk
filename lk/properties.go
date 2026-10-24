package lk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/swiftj/golk/lk/internal/gen"
)

// PropertiesService manages a resource's sidebar properties. The property
// writes are not versioned; their row writes are, and take IfMatch.
type PropertiesService struct{ c *Client }

// sidebarParams names a resource and, for paths that have one, a property;
// an empty id fails in expandPath.
func sidebarParams(resourceID, propertyID string) map[string]string {
	return map[string]string{"resourceId": resourceID, "propertyId": propertyID}
}

// List returns the resource's sidebar properties, sorted by rank, on one
// page. Hidden properties are listed only to admins. An unsupported property
// is listed as *blocks.UnsupportedProperty; echo it unchanged to keep it.
func (s *PropertiesService) List(ctx context.Context, resourceID string, opts ...CallOption) (*Page[blocks.Property], error) {
	var w struct {
		Data    blocks.Properties `json:"data"`
		HasMore bool              `json:"hasMore"`
	}
	if err := s.c.call(ctx, gen.OpListProperties.Method, gen.OpListProperties.Path,
		sidebarParams(resourceID, ""), request{out: &w}, opts); err != nil {
		return nil, err
	}
	return singlePage([]blocks.Property(w.Data), w.HasMore), nil
}

// All iterates over the resource's sidebar properties.
func (s *PropertiesService) All(ctx context.Context, resourceID string, opts ...CallOption) iter.Seq2[blocks.Property, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[blocks.Property], error) { return s.List(ctx, resourceID, opts...) })
}

// Get returns one sidebar property.
func (s *PropertiesService) Get(ctx context.Context, resourceID, propertyID string, opts ...CallOption) (blocks.Property, error) {
	return s.property(ctx, gen.OpGetProperty, sidebarParams(resourceID, propertyID), nil, opts)
}

// Create adds a sidebar property at place. Its Rank, if set, is not sent:
// place it with place instead. An id already used fails with
// ErrDuplicateID.
func (s *PropertiesService) Create(ctx context.Context, resourceID string, p blocks.Property, place Place, opts ...CallOption) (blocks.Property, error) {
	body, err := propertyCreateBody(p, place)
	if err != nil {
		return nil, err
	}
	return s.property(ctx, gen.OpCreateProperty, sidebarParams(resourceID, ""), body, opts)
}

// Update merges the fields set in p into the property and returns it.
func (s *PropertiesService) Update(ctx context.Context, resourceID, propertyID string, p PropertyPatch, opts ...CallOption) (blocks.Property, error) {
	return s.property(ctx, gen.OpUpdateProperty, sidebarParams(resourceID, propertyID), p, opts)
}

// Delete removes the property and its rows.
func (s *PropertiesService) Delete(ctx context.Context, resourceID, propertyID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteProperty.Method, gen.OpDeleteProperty.Path,
		sidebarParams(resourceID, propertyID), request{}, opts)
}

func (s *PropertiesService) property(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (blocks.Property, error) {
	var raw json.RawMessage
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &raw}, opts); err != nil {
		return nil, err
	}
	p, err := blocks.UnmarshalProperty(raw)
	if err != nil {
		return nil, fmt.Errorf("lk: decoding property: %w", err)
	}
	return p, nil
}

// ListRows returns the rows of a keyValue sidebar property and the version.
func (s *PropertiesService) ListRows(ctx context.Context, resourceID, propertyID string, opts ...CallOption) (*VersionedList[blocks.KVRow], error) {
	return listRows(ctx, s.c, gen.OpListPropertyRows, sidebarParams(resourceID, propertyID), opts)
}

// CreateRow adds a row to a keyValue sidebar property. It takes IfMatch.
func (s *PropertiesService) CreateRow(ctx context.Context, resourceID, propertyID string, r RowCreate, opts ...CallOption) (*Versioned[blocks.KVRow], error) {
	w, err := s.c.versioned(ctx, gen.OpCreatePropertyRow, sidebarParams(resourceID, propertyID), r, false, opts)
	return decodeVersioned(w, err, decodeRow)
}

// UpdateRow changes a row of a keyValue sidebar property. It takes IfMatch.
func (s *PropertiesService) UpdateRow(ctx context.Context, resourceID, propertyID, rowID string, p RowPatch, opts ...CallOption) (*Versioned[blocks.KVRow], error) {
	params := sidebarParams(resourceID, propertyID)
	params["rowId"] = rowID
	w, err := s.c.versioned(ctx, gen.OpUpdatePropertyRow, params, p, false, opts)
	return decodeVersioned(w, err, decodeRow)
}

// DeleteRow removes a row from a keyValue sidebar property. It takes
// IfMatch.
func (s *PropertiesService) DeleteRow(ctx context.Context, resourceID, propertyID, rowID string, opts ...CallOption) error {
	params := sidebarParams(resourceID, propertyID)
	params["rowId"] = rowID
	return s.c.call(ctx, gen.OpDeletePropertyRow.Method, gen.OpDeletePropertyRow.Path, params, request{versioned: true}, opts)
}
