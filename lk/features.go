package lk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/url"
	"time"

	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// FeaturesService manages a map document's features: its pins, labels,
// regions, paths and folders. Feature writes are not versioned and do not
// accept IfMatch (DECISIONS.md D-014, A-33).
type FeaturesService struct{ c *Client }

// FeatureType is the kind of a map feature.
type FeatureType string

// Feature types.
const (
	FeaturePin    FeatureType = "pin"
	FeatureLabel  FeatureType = "label"
	FeatureRegion FeatureType = "region"
	FeaturePath   FeatureType = "path"
	FeatureFolder FeatureType = "folder"
)

// BorderStyle is a region's border line style.
type BorderStyle string

// Border styles.
const (
	BorderSolid  BorderStyle = "solid"
	BorderDashed BorderStyle = "dashed"
	BorderDotted BorderStyle = "dotted"
	BorderNone   BorderStyle = "none"
)

// FillStyle is a region's fill style.
type FillStyle string

// Fill styles.
const (
	FillSolid   FillStyle = "solid"
	FillStriped FillStyle = "striped"
	FillDotted  FillStyle = "dotted"
	FillNone    FillStyle = "none"
)

// StrokeStyle is a path's line style.
type StrokeStyle string

// Stroke styles.
const (
	StrokeSolid  StrokeStyle = "solid"
	StrokeDashed StrokeStyle = "dashed"
	StrokeDotted StrokeStyle = "dotted"
	StrokeNone   StrokeStyle = "none"
)

// LabelSize is the size of a feature's name text. Writes take xs to 3xl;
// features drawn in the app may read back small, medium or large.
type LabelSize string

// Label sizes accepted on writes.
const (
	LabelXS  LabelSize = "xs"
	LabelSM  LabelSize = "sm"
	LabelMD  LabelSize = "md"
	LabelLG  LabelSize = "lg"
	LabelXL  LabelSize = "xl"
	Label2XL LabelSize = "2xl"
	Label3XL LabelSize = "3xl"
)

// LabelVisibility is when a feature's name is drawn.
type LabelVisibility string

// Label visibilities.
const (
	LabelAlways LabelVisibility = "always"
	LabelHover  LabelVisibility = "hover"
	LabelNever  LabelVisibility = "never"
)

// TextEffect is how TextEffectColor is drawn behind a label, region or
// path name: an outline around the letters, or a box behind the text.
type TextEffect string

// Text effects.
const (
	TextEffectOutline    TextEffect = "outline"
	TextEffectBackground TextEffect = "background"
)

// PinImage is a pin's image. URL must be an image asset URL of the same
// project.
type PinImage struct {
	URL    string
	Width  float64
	Height float64
}

// Feature is a feature of a map document. Strings and enums are empty when
// absent; values the SDK does not know are kept as read. Pos is [lat, lng]
// in the map's coordinate frame (see PixelToPos); polygons and polylines
// are lists of [lat, lng] points.
type Feature struct {
	ID   string
	Type FeatureType
	Name string
	// ParentID is the folder the feature is nested in. Deleting a folder
	// leaves its features naming it.
	ParentID string
	// Rank orders the feature among its siblings.
	Rank     string
	IsHidden bool
	// URI links the feature to a resource or document of the project:
	// lk://resources/<id>, lk://documents/<id> or
	// lk://resources/<id>/docs/<id>.
	URI string
	// Pos is absent on folders.
	Pos      Optional[[2]float64]
	Polygon  [][2]float64
	Polygons [][][2]float64
	Polyline [][2]float64

	// Pin style.
	IconGlyph string
	IconColor string
	IconShape IconShape
	IconSize  IconSize
	PinImage  *PinImage

	// Region and path style.
	Color         string
	FillOpacity   Optional[float64]
	FillStyle     FillStyle
	BorderStyle   BorderStyle
	StrokeOpacity Optional[float64]
	StrokeStyle   StrokeStyle
	StrokeWidth   Optional[float64]

	// Name text style.
	LabelSize       LabelSize
	LabelVisibility LabelVisibility
	TextColor       string
	TextEffect      TextEffect
	TextEffectColor string

	UpdatedAt time.Time
}

// FeatureCreate is the body of Features.Create. Every field is optional and
// only the fields set are sent. A color field on a type it does not apply to
// fails with a validation error.
type FeatureCreate struct {
	// ID is an optional client-chosen cuid2 id (see NewID).
	ID       Optional[string]
	Type     Optional[FeatureType]
	Name     Optional[string]
	ParentID Optional[string]
	Rank     Optional[string]
	IsHidden Optional[bool]
	// URI links the feature; see Feature.URI. Any other scheme fails with
	// ErrValidation.
	URI             Optional[string]
	Pos             Optional[[2]float64]
	Polygon         Optional[[][2]float64]
	Polygons        Optional[[][][2]float64]
	Polyline        Optional[[][2]float64]
	VisibilityRange Optional[[2]float64]

	IconGlyph Optional[string]
	IconColor Optional[string]
	IconShape Optional[IconShape]
	IconSize  Optional[IconSize]
	PinImage  Optional[PinImage]

	Color         Optional[string]
	FillOpacity   Optional[float64]
	FillStyle     Optional[FillStyle]
	BorderStyle   Optional[BorderStyle]
	StrokeOpacity Optional[float64]
	StrokeStyle   Optional[StrokeStyle]
	StrokeWidth   Optional[float64]

	LabelSize       Optional[LabelSize]
	LabelVisibility Optional[LabelVisibility]
	TextColor       Optional[string]
	TextEffect      Optional[TextEffect]
	TextEffectColor Optional[string]
}

// FeatureUpdate is the body of Features.Update. Only the fields set are sent
// and changed. URI: Some("") clears the link.
type FeatureUpdate struct {
	Type     Optional[FeatureType]
	Name     Optional[string]
	ParentID Optional[string]
	Rank     Optional[string]
	IsHidden Optional[bool]
	URI      Optional[string]

	Pos             Optional[[2]float64]
	Polygon         Optional[[][2]float64]
	Polygons        Optional[[][][2]float64]
	Polyline        Optional[[][2]float64]
	VisibilityRange Optional[[2]float64]

	IconGlyph Optional[string]
	IconColor Optional[string]
	IconShape Optional[IconShape]
	IconSize  Optional[IconSize]
	PinImage  Optional[PinImage]

	Color         Optional[string]
	FillOpacity   Optional[float64]
	FillStyle     Optional[FillStyle]
	BorderStyle   Optional[BorderStyle]
	StrokeOpacity Optional[float64]
	StrokeStyle   Optional[StrokeStyle]
	StrokeWidth   Optional[float64]

	LabelSize       Optional[LabelSize]
	LabelVisibility Optional[LabelVisibility]
	TextColor       Optional[string]
	TextEffect      Optional[TextEffect]
	TextEffectColor Optional[string]
}

// FeatureListParams filters Features.List.
type FeatureListParams struct {
	// Type lists only features of this type; empty lists every feature.
	Type FeatureType
}

func featureParams(doc DocRef, featureID string) map[string]string {
	p := doc.params()
	if featureID != "" {
		p["featureId"] = featureID
	}
	return p
}

// List returns the map's features, or only those of p.Type, on one page.
func (s *FeaturesService) List(ctx context.Context, doc DocRef, p FeatureListParams, opts ...CallOption) (*Page[Feature], error) {
	var w gen.FeatureList
	req := request{out: &w}
	if p.Type != "" {
		req.query = url.Values{"type": {string(p.Type)}}
	}
	if err := s.c.call(ctx, gen.OpListFeatures.Method, gen.OpListFeatures.Path, doc.params(), req, opts); err != nil {
		return nil, err
	}
	out := make([]Feature, 0, len(w.Data))
	for _, f := range w.Data {
		pf, err := featureFromWire(f)
		if err != nil {
			return nil, err
		}
		out = append(out, pf)
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// All iterates over the map's features, or only those of p.Type.
func (s *FeaturesService) All(ctx context.Context, doc DocRef, p FeatureListParams, opts ...CallOption) iter.Seq2[Feature, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Feature], error) { return s.List(ctx, doc, p, opts...) })
}

// Get returns one feature.
func (s *FeaturesService) Get(ctx context.Context, doc DocRef, featureID string, opts ...CallOption) (*Feature, error) {
	return s.feature(ctx, gen.OpGetFeature, featureParams(doc, featureID), nil, opts)
}

// Create adds a feature to the map and returns it. The first feature
// written to a blank document makes it a map.
func (s *FeaturesService) Create(ctx context.Context, doc DocRef, f FeatureCreate, opts ...CallOption) (*Feature, error) {
	body := gen.FeatureCreateParams{
		ID:              f.ID,
		Type:            convOpt[gen.FeatureCreateParamsType](f.Type),
		Name:            f.Name,
		ParentID:        f.ParentID,
		Rank:            f.Rank,
		IsHidden:        f.IsHidden,
		URI:             f.URI,
		Pos:             optPoint(f.Pos),
		Polygon:         optPoints(f.Polygon),
		Polygons:        optPolygons(f.Polygons),
		Polyline:        optPoints(f.Polyline),
		VisibilityRange: optPoint(f.VisibilityRange),
		IconGlyph:       f.IconGlyph,
		IconColor:       f.IconColor,
		IconShape:       convOpt[gen.FeatureCreateParamsIconShape](f.IconShape),
		IconSize:        convOpt[gen.FeatureCreateParamsIconSize](f.IconSize),
		PinImage:        optPinImage(f.PinImage),
		Color:           f.Color,
		FillOpacity:     f.FillOpacity,
		FillStyle:       convOpt[gen.FeatureCreateParamsFillStyle](f.FillStyle),
		BorderStyle:     convOpt[gen.FeatureCreateParamsBorderStyle](f.BorderStyle),
		StrokeOpacity:   f.StrokeOpacity,
		StrokeStyle:     convOpt[gen.FeatureCreateParamsStrokeStyle](f.StrokeStyle),
		StrokeWidth:     f.StrokeWidth,
		LabelSize:       convOpt[gen.FeatureCreateParamsLabelSize](f.LabelSize),
		LabelVisibility: convOpt[gen.FeatureCreateParamsLabelVisibility](f.LabelVisibility),
		TextColor:       f.TextColor,
		TextEffect:      convOpt[gen.FeatureCreateParamsTextEffect](f.TextEffect),
		TextEffectColor: f.TextEffectColor,
	}
	return s.feature(ctx, gen.OpCreateFeature, featureParams(doc, ""), body, opts)
}

// Update changes the fields set in u and returns the feature.
func (s *FeaturesService) Update(ctx context.Context, doc DocRef, featureID string, u FeatureUpdate, opts ...CallOption) (*Feature, error) {
	body := gen.FeatureUpdateParams{
		Type:            convOpt[gen.FeatureUpdateParamsType](u.Type),
		Name:            u.Name,
		ParentID:        u.ParentID,
		Rank:            u.Rank,
		IsHidden:        u.IsHidden,
		URI:             u.URI,
		Pos:             optPoint(u.Pos),
		Polygon:         optPoints(u.Polygon),
		Polygons:        optPolygons(u.Polygons),
		Polyline:        optPoints(u.Polyline),
		VisibilityRange: optPoint(u.VisibilityRange),
		IconGlyph:       u.IconGlyph,
		IconColor:       u.IconColor,
		IconShape:       convOpt[gen.FeatureUpdateParamsIconShape](u.IconShape),
		IconSize:        convOpt[gen.FeatureUpdateParamsIconSize](u.IconSize),
		PinImage:        optPinImage(u.PinImage),
		Color:           u.Color,
		FillOpacity:     u.FillOpacity,
		FillStyle:       convOpt[gen.FeatureUpdateParamsFillStyle](u.FillStyle),
		BorderStyle:     convOpt[gen.FeatureUpdateParamsBorderStyle](u.BorderStyle),
		StrokeOpacity:   u.StrokeOpacity,
		StrokeStyle:     convOpt[gen.FeatureUpdateParamsStrokeStyle](u.StrokeStyle),
		StrokeWidth:     u.StrokeWidth,
		LabelSize:       convOpt[gen.FeatureUpdateParamsLabelSize](u.LabelSize),
		LabelVisibility: convOpt[gen.FeatureUpdateParamsLabelVisibility](u.LabelVisibility),
		TextColor:       u.TextColor,
		TextEffect:      convOpt[gen.FeatureUpdateParamsTextEffect](u.TextEffect),
		TextEffectColor: u.TextEffectColor,
	}
	return s.feature(ctx, gen.OpUpdateFeature, featureParams(doc, featureID), body, opts)
}

// Delete deletes the feature. Deleting a folder leaves the features in it in
// place, still naming the deleted folder as their ParentID.
func (s *FeaturesService) Delete(ctx context.Context, doc DocRef, featureID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteFeature.Method, gen.OpDeleteFeature.Path, featureParams(doc, featureID), request{}, opts)
}

func (s *FeaturesService) feature(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (*Feature, error) {
	var w gen.Feature
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	f, err := featureFromWire(w)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func featureFromWire(w gen.Feature) (Feature, error) {
	f := Feature{
		ID:              w.ID,
		Type:            FeatureType(w.Type.Or("")),
		Name:            w.Name.Or(""),
		ParentID:        w.ParentID.Or(""),
		Rank:            w.Rank.Or(""),
		IsHidden:        w.IsHidden.Or(false),
		URI:             w.URI.Or(""),
		IconGlyph:       w.IconGlyph.Or(""),
		IconColor:       w.IconColor.Or(""),
		IconShape:       IconShape(w.IconShape.Or("")),
		IconSize:        IconSize(w.IconSize.Or("")),
		Color:           w.Color.Or(""),
		FillOpacity:     w.FillOpacity,
		FillStyle:       FillStyle(w.FillStyle.Or("")),
		BorderStyle:     BorderStyle(w.BorderStyle.Or("")),
		StrokeOpacity:   w.StrokeOpacity,
		StrokeStyle:     StrokeStyle(w.StrokeStyle.Or("")),
		StrokeWidth:     w.StrokeWidth,
		LabelSize:       LabelSize(w.LabelSize.Or("")),
		LabelVisibility: LabelVisibility(w.LabelVisibility.Or("")),
		TextColor:       w.TextColor.Or(""),
		TextEffect:      TextEffect(w.TextEffect.Or("")),
		TextEffectColor: w.TextEffectColor.Or(""),
		UpdatedAt:       utc(w.UpdatedAt.Or(time.Time{})),
	}
	if img, ok := w.PinImage.Get(); ok {
		f.PinImage = &PinImage{URL: img.URL, Width: img.Width, Height: img.Height}
	}
	var err error
	if raw, ok := w.Pos.Get(); ok {
		var p [2]float64
		if p, err = rawPoint(raw); err != nil {
			return Feature{}, fmt.Errorf("lk: decoding feature %s pos: %w", w.ID, err)
		}
		f.Pos = opt.Some(p)
	}
	if raw, ok := w.Polygon.Get(); ok {
		if f.Polygon, err = rawPoints(raw); err != nil {
			return Feature{}, fmt.Errorf("lk: decoding feature %s polygon: %w", w.ID, err)
		}
	}
	if raw, ok := w.Polyline.Get(); ok {
		if f.Polyline, err = rawPoints(raw); err != nil {
			return Feature{}, fmt.Errorf("lk: decoding feature %s polyline: %w", w.ID, err)
		}
	}
	if raw, ok := w.Polygons.Get(); ok {
		f.Polygons = make([][][2]float64, len(raw))
		for i, ring := range raw {
			if f.Polygons[i], err = rawPoints(ring); err != nil {
				return Feature{}, fmt.Errorf("lk: decoding feature %s polygons: %w", w.ID, err)
			}
		}
	}
	return f, nil
}

// The generated wire types carry [lat, lng] pairs as []json.RawMessage
// (the spec's prefixItems); these convert them to and from [2]float64.

func rawPoint(raw []json.RawMessage) ([2]float64, error) {
	var p [2]float64
	if len(raw) != 2 {
		return p, fmt.Errorf("want 2 numbers, got %d", len(raw))
	}
	for i, r := range raw {
		if err := json.Unmarshal(r, &p[i]); err != nil {
			return p, err
		}
	}
	return p, nil
}

func rawPoints(raw [][]json.RawMessage) ([][2]float64, error) {
	out := make([][2]float64, len(raw))
	for i, r := range raw {
		p, err := rawPoint(r)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

func pointRaw(p [2]float64) []json.RawMessage {
	out := make([]json.RawMessage, 2)
	for i, v := range p {
		// Marshalling a finite float64 cannot fail; a NaN or infinity
		// becomes null and the server rejects it.
		b, err := json.Marshal(v)
		if err != nil {
			b = []byte("null")
		}
		out[i] = b
	}
	return out
}

func pointsRaw(ps [][2]float64) [][]json.RawMessage {
	out := make([][]json.RawMessage, len(ps))
	for i, p := range ps {
		out[i] = pointRaw(p)
	}
	return out
}

func optPoint(o Optional[[2]float64]) opt.Optional[[]json.RawMessage] {
	if p, ok := o.Get(); ok {
		return opt.Some(pointRaw(p))
	}
	return opt.Optional[[]json.RawMessage]{}
}

func optPoints(o Optional[[][2]float64]) opt.Optional[[][]json.RawMessage] {
	if ps, ok := o.Get(); ok {
		return opt.Some(pointsRaw(ps))
	}
	return opt.Optional[[][]json.RawMessage]{}
}

func optPolygons(o Optional[[][][2]float64]) opt.Optional[[][][]json.RawMessage] {
	if rings, ok := o.Get(); ok {
		out := make([][][]json.RawMessage, len(rings))
		for i, r := range rings {
			out[i] = pointsRaw(r)
		}
		return opt.Some(out)
	}
	return opt.Optional[[][][]json.RawMessage]{}
}

func optPinImage(o Optional[PinImage]) opt.Optional[gen.FeatureImage] {
	if p, ok := o.Get(); ok {
		return opt.Some(gen.FeatureImage{URL: p.URL, Width: p.Width, Height: p.Height})
	}
	return opt.Optional[gen.FeatureImage]{}
}
