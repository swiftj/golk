package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

func (s *server) addMapTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name: "read_map",
		Description: "Read a map document: its image (url, size, bounds) and every feature (pins, labels, regions, paths, folders) " +
			"with ids, names, positions [lat, lng], folders, links and hidden flags. Omit document_id for the resource's first map.",
		Annotations: readOnly("Read map"),
	}, func(in ReadMapIn) string { return in.Project }, s.readMap)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name: "set_map_feature",
		Description: "Create a map feature (pin, label, region, path or folder), or update the one named by feature_id. Only the fields given are sent. " +
			"Place it with pos [lat, lng], or with pixel [x, y] counted from the map image's top-left corner (not both). " +
			"Its name, link, image and label text are checked by the spoiler guard unless the map document or its resource is hidden; " +
			"hidden features are checked too, since players may see them.",
		Annotations: write("Set map feature", false, false),
	}, func(in SetMapFeatureIn) string { return in.Project }, serial(s, s.setMapFeature))
	addTool(s, ms, &mcp.Tool{
		Name: "delete_map_feature",
		Description: "Delete one map feature. Deleting a folder leaves the features in it in place, still naming the folder as their parent. " +
			"Use dry_run to see what would go.",
		Annotations: write("Delete map feature", true, true),
	}, func(in DeleteMapFeatureIn) string { return in.Project }, serial(s, s.deleteMapFeature))
	addTool(s, ms, &mcp.Tool{
		Name: "set_map_image",
		Description: "Set or replace a map document's image from an image asset (asset_id from upload_image). Features keep their positions. " +
			"A blank document becomes a map. The asset's URL and name are checked by the spoiler guard unless the document or resource is hidden.",
		Annotations: write("Set map image", true, true),
	}, func(in SetMapImageIn) string { return in.Project }, serial(s, s.setMapImage))
}

// mapDoc finds the map document id of r, or its first map document when id
// is empty. A blank document is accepted only when named and blankOK is set
// (a write makes it a map).
func mapDoc(r *lk.Resource, id string, blankOK bool) (*lk.Document, error) {
	if id == "" {
		for i := range r.Documents {
			if r.Documents[i].Type == lk.DocumentMap {
				return &r.Documents[i], nil
			}
		}
		return nil, fmt.Errorf("resource %q (%s) has no map document. Hint: pass document_id; documents: %s", r.Name, r.ID, docList(r.Documents))
	}
	d, err := findDoc(r, id)
	if err != nil {
		return nil, err
	}
	if d.Type == lk.DocumentMap || blankOK && d.Type == lk.DocumentBlank {
		return d, nil
	}
	return nil, fmt.Errorf("document %q (%s) of %q is a %s document, not a map", d.Name, d.ID, r.Name, d.Type)
}

// ReadMapIn is read_map's input.
type ReadMapIn struct {
	ProjectArg
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the map"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"the map document; omit for the resource's first map document"`
	Type       string `json:"type,omitempty" jsonschema:"list only features of this type: pin, label, region, path or folder"`
}

// MapImageOut is a map's image and coordinate frame.
type MapImageOut struct {
	AssetID string        `json:"asset_id,omitempty"`
	URL     string        `json:"url,omitempty"`
	Width   int           `json:"width"`
	Height  int           `json:"height"`
	Bounds  [2][2]float64 `json:"bounds" jsonschema:"[[south, west], [north, east]]"`
}

// ReadMapOut is read_map's structured output; the features are in the text.
type ReadMapOut struct {
	ResourceID string       `json:"resource_id"`
	DocumentID string       `json:"document_id"`
	Name       string       `json:"name"`
	Hidden     bool         `json:"hidden"`
	Image      *MapImageOut `json:"image,omitempty"`
	Features   int          `json:"features"`
}

func mapImageOut(m *lk.MapImage) *MapImageOut {
	if m == nil {
		return nil
	}
	return &MapImageOut{AssetID: m.AssetID, URL: m.URL, Width: m.Width, Height: m.Height, Bounds: m.Bounds}
}

func describeImage(m *lk.MapImage) string {
	if m == nil {
		return "no image"
	}
	url := m.URL
	if url == "" {
		url = "(legacy tiled map, no url)"
	}
	return fmt.Sprintf("image %s, %dx%d, bounds [[%g, %g], [%g, %g]]", url, m.Width, m.Height, m.Bounds[0][0], m.Bounds[0][1], m.Bounds[1][0], m.Bounds[1][1])
}

func (s *server) readMap(ctx context.Context, c *lk.Client, in ReadMapIn) (toolResult[ReadMapOut], error) {
	var zero toolResult[ReadMapOut]
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := mapDoc(r, in.DocumentID, false)
	if err != nil {
		return zero, err
	}
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	full, err := c.Documents.Get(ctx, ref)
	if err != nil {
		return zero, err
	}
	fs, err := c.Features.List(ctx, ref, lk.FeatureListParams{Type: lk.FeatureType(in.Type)})
	if err != nil {
		return zero, err
	}
	out := ReadMapOut{ResourceID: r.ID, DocumentID: full.ID, Name: full.Name, Hidden: full.IsHidden,
		Image: mapImageOut(full.MapImage), Features: len(fs.Items)}
	var b strings.Builder
	fmt.Fprintf(&b, "Map %q (%s) of %q (%s)%s: %s.\n", full.Name, full.ID, r.Name, r.ID,
		listFlags(flagIf(full.IsHidden, "hidden document"), flagIf(r.IsHidden, "hidden resource")), describeImage(full.MapImage))
	for _, f := range fs.Items {
		fmt.Fprintf(&b, "- %s %q (%s)", orDash(string(f.Type)), f.Name, f.ID)
		if p, ok := f.Pos.Get(); ok {
			fmt.Fprintf(&b, " pos [%g, %g]", p[0], p[1])
		}
		if len(f.Polygon) > 0 {
			fmt.Fprintf(&b, " polygon of %d points", len(f.Polygon))
		}
		if len(f.Polyline) > 0 {
			fmt.Fprintf(&b, " polyline of %d points", len(f.Polyline))
		}
		if f.ParentID != "" {
			fmt.Fprintf(&b, " folder %s", f.ParentID)
		}
		if f.URI != "" {
			fmt.Fprintf(&b, " link %s", f.URI)
		}
		b.WriteString(listFlags(flagIf(f.IsHidden, "hidden")))
		b.WriteString("\n")
	}
	noun := "feature(s)"
	if in.Type != "" {
		noun = in.Type + " feature(s)"
	}
	fmt.Fprintf(&b, "%d %s.", len(fs.Items), noun)
	return toolResult[ReadMapOut]{out: out, text: b.String()}, nil
}

// FeatureStyle is a feature's look. Which fields apply depends on its type
// (pins: icon_*, pin_image; regions and paths: color, fill_*, border_style,
// stroke_*); a field on a type it doesn't apply to fails validation.
type FeatureStyle struct {
	IconGlyph       *string      `json:"icon_glyph,omitempty" jsonschema:"a pin's icon glyph name"`
	IconColor       *string      `json:"icon_color,omitempty" jsonschema:"a pin's icon colour, such as #aa3344"`
	IconShape       *string      `json:"icon_shape,omitempty" jsonschema:"a pin's shape: pin, diamond, flag, marker, shield, circle, banner, arch or none"`
	IconSize        *string      `json:"icon_size,omitempty" jsonschema:"a pin's size: medium or small"`
	PinImage        *PinImageArg `json:"pin_image,omitempty" jsonschema:"an image shown as the pin (an image asset URL of this project)"`
	Color           *string      `json:"color,omitempty" jsonschema:"a region's or path's colour"`
	FillOpacity     *float64     `json:"fill_opacity,omitempty" jsonschema:"a region's fill opacity, 0-1"`
	FillStyle       *string      `json:"fill_style,omitempty" jsonschema:"a region's fill: solid, striped, dotted or none"`
	BorderStyle     *string      `json:"border_style,omitempty" jsonschema:"a region's border: solid, dashed, dotted or none"`
	StrokeOpacity   *float64     `json:"stroke_opacity,omitempty" jsonschema:"a path's or border's opacity, 0-1"`
	StrokeStyle     *string      `json:"stroke_style,omitempty" jsonschema:"a path's line: solid, dashed, dotted or none"`
	StrokeWidth     *float64     `json:"stroke_width,omitempty" jsonschema:"a path's or border's line width"`
	LabelSize       *string      `json:"label_size,omitempty" jsonschema:"name text size: xs, sm, md, lg, xl, 2xl or 3xl"`
	LabelVisibility *string      `json:"label_visibility,omitempty" jsonschema:"when the name is drawn: always, hover or never"`
	TextColor       *string      `json:"text_color,omitempty" jsonschema:"name text colour"`
	TextEffect      *string      `json:"text_effect,omitempty" jsonschema:"outline or background, drawn in text_effect_color"`
	TextEffectColor *string      `json:"text_effect_color,omitempty" jsonschema:"the colour of text_effect"`
}

// PinImageArg is a pin's image.
type PinImageArg struct {
	URL    string  `json:"url"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// SetMapFeatureIn is set_map_feature's input.
type SetMapFeatureIn struct {
	ProjectArg
	DryRunArg
	ResourceID string        `json:"resource_id" jsonschema:"the resource holding the map"`
	DocumentID string        `json:"document_id,omitempty" jsonschema:"the map document (a blank one becomes a map); omit for the resource's first map document"`
	FeatureID  string        `json:"feature_id,omitempty" jsonschema:"the feature to update; omit to create one"`
	Type       string        `json:"type,omitempty" jsonschema:"pin, label, region, path or folder"`
	Name       *string       `json:"name,omitempty" jsonschema:"the name players see"`
	Pos        *[2]float64   `json:"pos,omitempty" jsonschema:"position [lat, lng] in the map's frame (not with pixel)"`
	Pixel      *[2]float64   `json:"pixel,omitempty" jsonschema:"position [x, y] in pixels from the map image's top-left corner (not with pos; needs a map image)"`
	ParentID   *string       `json:"parent_id,omitempty" jsonschema:"the folder feature to nest it in"`
	Rank       *string       `json:"rank,omitempty" jsonschema:"order among its siblings"`
	URI        *string       `json:"uri,omitempty" jsonschema:"link: lk://resources/<id>, lk://documents/<id> or lk://resources/<id>/docs/<id>; empty clears it on update"`
	IsHidden   *bool         `json:"is_hidden,omitempty" jsonschema:"true hides it from players (it is still checked by the spoiler guard)"`
	Polygon    *[][2]float64 `json:"polygon,omitempty" jsonschema:"a region's outline, [lat, lng] points"`
	Polyline   *[][2]float64 `json:"polyline,omitempty" jsonschema:"a path's points, [lat, lng]"`
	Style      *FeatureStyle `json:"style,omitempty" jsonschema:"colour, icon, opacity, stroke, border, fill, label and text style"`
}

// FeatureOut describes a feature after a write (or, with dry_run, as it
// would be).
type FeatureOut struct {
	ResourceID string      `json:"resource_id"`
	DocumentID string      `json:"document_id"`
	FeatureID  string      `json:"feature_id,omitempty"`
	Type       string      `json:"type,omitempty"`
	Name       string      `json:"name"`
	Pos        *[2]float64 `json:"pos,omitempty"`
	Hidden     bool        `json:"hidden"`
	Created    bool        `json:"created,omitempty"`
	DryRun     bool        `json:"dry_run,omitempty"`
}

func setOpt[T any](dst *lk.Optional[T], v *T) {
	if v != nil {
		*dst = lk.Some(*v)
	}
}

func setOptAs[T ~string](dst *lk.Optional[T], v *string) {
	if v != nil {
		*dst = lk.Some(T(*v))
	}
}

// featureChange is set_map_feature's input as the fields it sends; it
// fills a FeatureCreate or a FeatureUpdate (which have the same fields)
// and overlays a Feature to give what players will see.
type featureChange struct {
	in  SetMapFeatureIn
	pos *[2]float64
}

func (fc featureChange) count() int {
	in := fc.in
	n := 0
	for _, set := range []bool{in.Type != "", in.Name != nil, fc.pos != nil, in.ParentID != nil, in.Rank != nil, in.URI != nil,
		in.IsHidden != nil, in.Polygon != nil, in.Polyline != nil} {
		if set {
			n++
		}
	}
	if st := in.Style; st != nil {
		for _, set := range []bool{st.IconGlyph != nil, st.IconColor != nil, st.IconShape != nil, st.IconSize != nil, st.PinImage != nil,
			st.Color != nil, st.FillOpacity != nil, st.FillStyle != nil, st.BorderStyle != nil, st.StrokeOpacity != nil, st.StrokeStyle != nil,
			st.StrokeWidth != nil, st.LabelSize != nil, st.LabelVisibility != nil, st.TextColor != nil, st.TextEffect != nil, st.TextEffectColor != nil} {
			if set {
				n++
			}
		}
	}
	return n
}

func (fc featureChange) update() lk.FeatureUpdate {
	in := fc.in
	var u lk.FeatureUpdate
	if in.Type != "" {
		u.Type = lk.Some(lk.FeatureType(in.Type))
	}
	setOpt(&u.Name, in.Name)
	setOpt(&u.Pos, fc.pos)
	setOpt(&u.ParentID, in.ParentID)
	setOpt(&u.Rank, in.Rank)
	setOpt(&u.URI, in.URI)
	setOpt(&u.IsHidden, in.IsHidden)
	setOpt(&u.Polygon, in.Polygon)
	setOpt(&u.Polyline, in.Polyline)
	if st := in.Style; st != nil {
		setOpt(&u.IconGlyph, st.IconGlyph)
		setOpt(&u.IconColor, st.IconColor)
		setOptAs(&u.IconShape, st.IconShape)
		setOptAs(&u.IconSize, st.IconSize)
		if st.PinImage != nil {
			u.PinImage = lk.Some(lk.PinImage{URL: st.PinImage.URL, Width: st.PinImage.Width, Height: st.PinImage.Height})
		}
		setOpt(&u.Color, st.Color)
		setOpt(&u.FillOpacity, st.FillOpacity)
		setOptAs(&u.FillStyle, st.FillStyle)
		setOptAs(&u.BorderStyle, st.BorderStyle)
		setOpt(&u.StrokeOpacity, st.StrokeOpacity)
		setOptAs(&u.StrokeStyle, st.StrokeStyle)
		setOpt(&u.StrokeWidth, st.StrokeWidth)
		setOptAs(&u.LabelSize, st.LabelSize)
		setOptAs(&u.LabelVisibility, st.LabelVisibility)
		setOpt(&u.TextColor, st.TextColor)
		setOptAs(&u.TextEffect, st.TextEffect)
		setOpt(&u.TextEffectColor, st.TextEffectColor)
	}
	return u
}

// create is update as a FeatureCreate: the two have the same fields, and
// FeatureCreate's extras (ID, Polygons, VisibilityRange) stay unset.
func (fc featureChange) create() lk.FeatureCreate {
	u := fc.update()
	return lk.FeatureCreate{
		Type: u.Type, Name: u.Name, ParentID: u.ParentID, Rank: u.Rank, IsHidden: u.IsHidden, URI: u.URI,
		Pos: u.Pos, Polygon: u.Polygon, Polyline: u.Polyline,
		IconGlyph: u.IconGlyph, IconColor: u.IconColor, IconShape: u.IconShape, IconSize: u.IconSize, PinImage: u.PinImage,
		Color: u.Color, FillOpacity: u.FillOpacity, FillStyle: u.FillStyle, BorderStyle: u.BorderStyle,
		StrokeOpacity: u.StrokeOpacity, StrokeStyle: u.StrokeStyle, StrokeWidth: u.StrokeWidth,
		LabelSize: u.LabelSize, LabelVisibility: u.LabelVisibility, TextColor: u.TextColor, TextEffect: u.TextEffect, TextEffectColor: u.TextEffectColor,
	}
}

// overlay returns f with the change applied: the feature as players will
// see it.
func (fc featureChange) overlay(f lk.Feature) lk.Feature {
	u := fc.update()
	str := func(dst *string, o lk.Optional[string]) {
		if v, ok := o.Get(); ok {
			*dst = v
		}
	}
	if v, ok := u.Type.Get(); ok {
		f.Type = v
	}
	str(&f.Name, u.Name)
	str(&f.ParentID, u.ParentID)
	str(&f.Rank, u.Rank)
	str(&f.URI, u.URI)
	if v, ok := u.IsHidden.Get(); ok {
		f.IsHidden = v
	}
	if u.Pos.IsSet() {
		f.Pos = u.Pos
	}
	if v, ok := u.Polygon.Get(); ok {
		f.Polygon = v
	}
	if v, ok := u.Polyline.Get(); ok {
		f.Polyline = v
	}
	str(&f.IconGlyph, u.IconGlyph)
	str(&f.IconColor, u.IconColor)
	if v, ok := u.IconShape.Get(); ok {
		f.IconShape = v
	}
	if v, ok := u.IconSize.Get(); ok {
		f.IconSize = v
	}
	if v, ok := u.PinImage.Get(); ok {
		f.PinImage = &v
	}
	str(&f.Color, u.Color)
	if u.FillOpacity.IsSet() {
		f.FillOpacity = u.FillOpacity
	}
	if v, ok := u.FillStyle.Get(); ok {
		f.FillStyle = v
	}
	if v, ok := u.BorderStyle.Get(); ok {
		f.BorderStyle = v
	}
	if u.StrokeOpacity.IsSet() {
		f.StrokeOpacity = u.StrokeOpacity
	}
	if v, ok := u.StrokeStyle.Get(); ok {
		f.StrokeStyle = v
	}
	if u.StrokeWidth.IsSet() {
		f.StrokeWidth = u.StrokeWidth
	}
	if v, ok := u.LabelSize.Get(); ok {
		f.LabelSize = v
	}
	if v, ok := u.LabelVisibility.Get(); ok {
		f.LabelVisibility = v
	}
	str(&f.TextColor, u.TextColor)
	if v, ok := u.TextEffect.Get(); ok {
		f.TextEffect = v
	}
	str(&f.TextEffectColor, u.TextEffectColor)
	return f
}

func featureOut(r *lk.Resource, d *lk.Document, f lk.Feature) FeatureOut {
	out := FeatureOut{ResourceID: r.ID, DocumentID: d.ID, FeatureID: f.ID, Type: string(f.Type), Name: f.Name, Hidden: f.IsHidden}
	if p, ok := f.Pos.Get(); ok {
		out.Pos = &p
	}
	return out
}

func (s *server) setMapFeature(ctx context.Context, c *lk.Client, in SetMapFeatureIn) (toolResult[FeatureOut], error) {
	var zero toolResult[FeatureOut]
	if in.Pos != nil && in.Pixel != nil {
		return zero, errors.New("give pos or pixel, not both")
	}
	fc := featureChange{in: in, pos: in.Pos}
	if in.FeatureID == "" && in.Type == "" {
		return zero, errors.New("type is required to create a feature: pin, label, region, path or folder")
	}
	if fc.count() == 0 && in.Pixel == nil {
		return zero, errors.New("nothing to change: give at least one field to set")
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := mapDoc(r, in.DocumentID, true)
	if err != nil {
		return zero, err
	}
	ref := lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}
	if in.Pixel != nil {
		full, err := c.Documents.Get(ctx, ref)
		if err != nil {
			return zero, err
		}
		if full.MapImage == nil || full.MapImage.Width <= 0 || full.MapImage.Height <= 0 {
			return zero, fmt.Errorf("map %q (%s) has no image, so pixel can't be converted. Hint: set one with set_map_image, or give pos [lat, lng]", d.Name, d.ID)
		}
		p := lk.PixelToPos(*full.MapImage, in.Pixel[0], in.Pixel[1])
		fc.pos = &p
	}
	before := lk.Feature{}
	if in.FeatureID != "" {
		cur, err := c.Features.Get(ctx, ref, in.FeatureID)
		if err != nil {
			return zero, err
		}
		before = *cur
	}
	after := fc.overlay(before)
	// The guard checks the feature as it will be, with the map's name,
	// unless players can't see the map at all (hidden document or
	// resource; hidden is not inherited, D-012). A hidden feature is
	// checked too until hidden pins are confirmed invisible (X-GUARD-11).
	dmOnly := hiddenFeaturesConfirmed && after.IsHidden
	if !r.IsHidden && !d.IsHidden && !dmOnly {
		if err := s.check(map[string]any{"map": d.Name, "feature": after}); err != nil {
			return zero, err
		}
	}
	out := featureOut(r, d, after)
	out.DryRun, out.Created = in.DryRun, in.FeatureID == ""
	verb := "update"
	if out.Created {
		verb = "create"
	}
	what := fmt.Sprintf("%s %q", orDash(string(after.Type)), after.Name)
	if in.DryRun {
		return toolResult[FeatureOut]{out: out, text: fmt.Sprintf("Dry run: would %s %s on map %q (%s) of %q, %s, setting %d field(s). Nothing was written.",
			verb, what, d.Name, d.ID, r.Name, posText(fc.pos), fc.count())}, nil
	}
	var f *lk.Feature
	if out.Created {
		f, err = c.Features.Create(ctx, ref, fc.create())
	} else {
		f, err = c.Features.Update(ctx, ref, in.FeatureID, fc.update())
	}
	if err != nil {
		return zero, err
	}
	out = featureOut(r, d, *f)
	out.Created = in.FeatureID == ""
	return toolResult[FeatureOut]{out: out, text: fmt.Sprintf("%sd %s %q (%s) on map %q (%s) of %q%s%s.",
		capitalize(verb), orDash(string(f.Type)), f.Name, f.ID, d.Name, d.ID, r.Name, posSuffix(f.Pos), listFlags(flagIf(f.IsHidden, "hidden")))}, nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func posText(p *[2]float64) string {
	if p == nil {
		return "position unchanged"
	}
	return fmt.Sprintf("at pos [%g, %g]", p[0], p[1])
}

func posSuffix(p lk.Optional[[2]float64]) string {
	if v, ok := p.Get(); ok {
		return fmt.Sprintf(" at pos [%g, %g]", v[0], v[1])
	}
	return ""
}

// DeleteMapFeatureIn is delete_map_feature's input.
type DeleteMapFeatureIn struct {
	ProjectArg
	DryRunArg
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the map"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"the map document; omit for the resource's first map document"`
	FeatureID  string `json:"feature_id" jsonschema:"the feature to delete"`
}

// DeleteMapFeatureOut is delete_map_feature's output.
type DeleteMapFeatureOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	FeatureID  string `json:"feature_id"`
	Type       string `json:"type,omitempty"`
	Name       string `json:"name"`
	// Orphans is, for a folder, how many features name it as their parent
	// and keep doing so after the delete.
	Orphans int  `json:"orphans,omitempty"`
	DryRun  bool `json:"dry_run,omitempty"`
}

func (s *server) deleteMapFeature(ctx context.Context, c *lk.Client, in DeleteMapFeatureIn) (toolResult[DeleteMapFeatureOut], error) {
	var zero toolResult[DeleteMapFeatureOut]
	if in.FeatureID == "" {
		return zero, errors.New("feature_id is required")
	}
	// No guard: deleting takes content away from players, it can't show
	// them anything.
	ref := lk.DocRef{ResourceID: in.ResourceID, DocumentID: in.DocumentID}
	if in.DocumentID == "" {
		r, err := c.Resources.Get(ctx, in.ResourceID)
		if err != nil {
			return zero, err
		}
		d, err := mapDoc(r, "", false)
		if err != nil {
			return zero, err
		}
		ref.DocumentID = d.ID
	}
	f, err := c.Features.Get(ctx, ref, in.FeatureID)
	if err != nil {
		return zero, err
	}
	out := DeleteMapFeatureOut{ResourceID: ref.ResourceID, DocumentID: ref.DocumentID, FeatureID: f.ID, Type: string(f.Type), Name: f.Name, DryRun: in.DryRun}
	what := fmt.Sprintf("%s %q (%s)", orDash(string(f.Type)), f.Name, f.ID)
	warn := ""
	if f.Type == lk.FeatureFolder {
		fs, err := c.Features.List(ctx, ref, lk.FeatureListParams{})
		if err != nil {
			return zero, err
		}
		for _, x := range fs.Items {
			if x.ParentID == f.ID {
				out.Orphans++
			}
		}
		warn = fmt.Sprintf(" Warning: it is a folder; the %d feature(s) in it are not deleted and keep %s as their parentId.", out.Orphans, f.ID)
	}
	if in.DryRun {
		return toolResult[DeleteMapFeatureOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s from map %s.%s Nothing was written.", what, ref.DocumentID, warn)}, nil
	}
	if err := c.Features.Delete(ctx, ref, f.ID); err != nil {
		return zero, err
	}
	return toolResult[DeleteMapFeatureOut]{out: out, text: fmt.Sprintf("Deleted %s from map %s.%s", what, ref.DocumentID, warn)}, nil
}

// SetMapImageIn is set_map_image's input.
type SetMapImageIn struct {
	ProjectArg
	DryRunArg
	ResourceID string `json:"resource_id" jsonschema:"the resource holding the map"`
	DocumentID string `json:"document_id" jsonschema:"the map document (a blank one becomes a map)"`
	AssetID    string `json:"asset_id" jsonschema:"the image asset (asset_id from upload_image)"`
}

// SetMapImageOut is set_map_image's output.
type SetMapImageOut struct {
	ResourceID string       `json:"resource_id"`
	DocumentID string       `json:"document_id"`
	Name       string       `json:"name"`
	AssetID    string       `json:"asset_id"`
	Image      *MapImageOut `json:"image,omitempty"`
	DryRun     bool         `json:"dry_run,omitempty"`
}

func (s *server) setMapImage(ctx context.Context, c *lk.Client, in SetMapImageIn) (toolResult[SetMapImageOut], error) {
	var zero toolResult[SetMapImageOut]
	if in.DocumentID == "" || in.AssetID == "" {
		return zero, errors.New("document_id and asset_id are required")
	}
	r, err := c.Resources.Get(ctx, in.ResourceID)
	if err != nil {
		return zero, err
	}
	d, err := mapDoc(r, in.DocumentID, true)
	if err != nil {
		return zero, err
	}
	a, err := c.Assets.Get(ctx, in.AssetID)
	if err != nil {
		return zero, err
	}
	// Players see the image (its URL) on the map unless the document or
	// the resource is hidden.
	if !r.IsHidden && !d.IsHidden {
		if err := s.check(map[string]any{"map": d.Name, "mapImage": a.URL, "assetName": a.Name}); err != nil {
			return zero, err
		}
	}
	out := SetMapImageOut{ResourceID: r.ID, DocumentID: d.ID, Name: d.Name, AssetID: a.ID, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[SetMapImageOut]{out: out, text: fmt.Sprintf("Dry run: would set the image of map %q (%s) of %q to asset %q (%s, %dx%d). Nothing was written.",
			d.Name, d.ID, r.Name, a.Name, a.ID, a.Width, a.Height)}, nil
	}
	nd, err := c.Documents.Update(ctx, lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}, lk.DocumentUpdate{MapImage: lk.Some(lk.MapImageRef{AssetID: a.ID})})
	if err != nil {
		return zero, err
	}
	out.Name, out.Image = nd.Name, mapImageOut(nd.MapImage)
	return toolResult[SetMapImageOut]{out: out, text: fmt.Sprintf("Set the image of map %q (%s) of %q to asset %q (%s): %s. Features keep their positions.",
		nd.Name, nd.ID, r.Name, a.Name, a.ID, describeImage(nd.MapImage))}, nil
}
