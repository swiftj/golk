package lk

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// AssetType is the kind of an asset. A type this SDK does not know is kept
// as is.
type AssetType string

// Asset types.
const (
	AssetImage  AssetType = "IMAGE"
	AssetMap    AssetType = "MAP"
	AssetFolder AssetType = "FOLDER"
	AssetAudio  AssetType = "AUDIO"
)

// Asset is an entry in the project's asset library: an image, a map image,
// a folder or an audio file.
type Asset struct {
	ID   string
	Name string
	Type AssetType
	// FolderID is empty for an asset at the root.
	FolderID    string
	IsHidden    bool
	Description string
	// URL is an image's file URL. Write it into image fields.
	URL string
	// Width and Height are an image's pixel size.
	Width     int
	Height    int
	CreatedAt time.Time
}

// AssetsService manages the project's asset library. Any member can create
// folders and upload images; every other write needs the admin role.
type AssetsService struct{ c *Client }

// AssetListParams filters Assets.List.
type AssetListParams struct {
	// FolderID lists one folder's direct contents; empty lists the root.
	FolderID string
	// Limit is the page size (1–100). Zero uses the server default (50).
	Limit int
}

// List returns the first page of a folder's contents.
func (s *AssetsService) List(ctx context.Context, p AssetListParams, opts ...CallOption) (*Page[Asset], error) {
	q := url.Values{}
	if p.FolderID != "" {
		q.Set("folderId", p.FolderID)
	}
	if p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	pg := &pager[gen.AssetList, Asset]{
		c: s.c, tmpl: gen.OpListAssets.Path, query: q, opts: opts,
		items: func(w gen.AssetList) ([]Asset, opt.Optional[bool], opt.Nullable[string]) {
			out := make([]Asset, len(w.Data))
			for i, a := range w.Data {
				out[i] = assetFromWire(a)
			}
			return out, w.HasMore, w.NextCursor
		},
	}
	return pg.page(ctx, "")
}

// All iterates over a folder's contents, fetching pages as it goes.
func (s *AssetsService) All(ctx context.Context, p AssetListParams, opts ...CallOption) iter.Seq2[Asset, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Asset], error) { return s.List(ctx, p, opts...) })
}

// Get returns one asset.
func (s *AssetsService) Get(ctx context.Context, id string, opts ...CallOption) (*Asset, error) {
	return s.asset(ctx, gen.OpGetAsset, map[string]string{"assetId": id}, nil, opts)
}

// AssetFolderCreate is the body of Assets.CreateFolder.
type AssetFolderCreate struct {
	Name string
	// FolderID nests the folder in another; unset creates it at the root.
	FolderID Optional[string]
}

// CreateFolder creates a folder.
func (s *AssetsService) CreateFolder(ctx context.Context, f AssetFolderCreate, opts ...CallOption) (*Asset, error) {
	return s.asset(ctx, gen.OpCreateAssetFolder, nil, gen.AssetCreateParams{Name: f.Name, FolderID: f.FolderID}, opts)
}

// AssetUpdate is the body of Assets.Update. Only the fields that are set
// are sent and changed.
type AssetUpdate struct {
	Name        Optional[string]
	Description Optional[string]
	// FolderID moves the asset; Null moves it to the root.
	FolderID Nullable[string]
	IsHidden Optional[bool]
}

// Update changes the asset. The server returns no body; Get the asset to
// read it back. It needs the admin role.
func (s *AssetsService) Update(ctx context.Context, id string, u AssetUpdate, opts ...CallOption) error {
	body := gen.AssetUpdateParams{Name: u.Name, Description: u.Description, FolderID: u.FolderID, IsHidden: u.IsHidden}
	return s.c.call(ctx, gen.OpUpdateAsset.Method, gen.OpUpdateAsset.Path, map[string]string{"assetId": id},
		request{body: body}, opts)
}

// Delete deletes the asset; deleting a folder deletes everything inside it.
// It needs the admin role.
func (s *AssetsService) Delete(ctx context.Context, id string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteAsset.Method, gen.OpDeleteAsset.Path, map[string]string{"assetId": id},
		request{}, opts)
}

// ImageUploadCreate is the body of Assets.CreateImageUpload.
type ImageUploadCreate struct {
	// ContentType is image/jpeg, image/jpg, image/png, image/gif or
	// image/webp.
	ContentType string
	// ContentLength is the file's exact size in bytes.
	ContentLength int64
	// ContentHash is the file's lowercase hex SHA-256. When set, storage
	// refuses bytes that do not match.
	ContentHash Optional[string]
}

// ImageUpload is where to send an image's bytes: Method to UploadURL with
// exactly Headers, before ExpiresAt (ten minutes after it was created).
type ImageUpload struct {
	UploadKey string
	UploadURL string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// imageContentTypes are the image types the API accepts.
func imageContentTypes() []string {
	return []string{"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp"}
}

// CreateImageUpload is step one of an upload: it returns a signed storage
// URL for the bytes. UploadImage does all three steps.
func (s *AssetsService) CreateImageUpload(ctx context.Context, u ImageUploadCreate, opts ...CallOption) (*ImageUpload, error) {
	body := gen.ImageUploadCreateParams{
		ContentType: gen.ImageUploadCreateParamsContentType(u.ContentType), ContentLength: u.ContentLength,
		ContentHash: u.ContentHash,
	}
	var w gen.ImageUpload
	if err := s.c.call(ctx, gen.OpCreateImageUpload.Method, gen.OpCreateImageUpload.Path, nil,
		request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	exp, _ := time.Parse(time.RFC3339Nano, w.ExpiresAt)
	return &ImageUpload{
		UploadKey: w.UploadKey, UploadURL: w.UploadURL, Method: string(w.Method),
		Headers: w.Headers, ExpiresAt: utc(exp),
	}, nil
}

// ImageCreate is the body of Assets.CreateImage.
type ImageCreate struct {
	UploadKey   string
	Name        string
	Description Optional[string]
	// FolderID places the image in a folder; unset places it at the root.
	FolderID Optional[string]
}

// CreateImage registers the file stored under UploadKey as an image asset.
// The server records its real size, type and pixel dimensions; a file that
// is not an accepted image fails with ErrValidation.
func (s *AssetsService) CreateImage(ctx context.Context, c ImageCreate, opts ...CallOption) (*Asset, error) {
	body := gen.ImageAssetCreateParams{UploadKey: c.UploadKey, Name: c.Name, Description: c.Description, FolderID: c.FolderID}
	return s.asset(ctx, gen.OpCreateImageAsset, nil, body, opts)
}

func (s *AssetsService) asset(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (*Asset, error) {
	var w gen.Asset
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	a := assetFromWire(w)
	return &a, nil
}

// UploadImageParams is the input of Assets.UploadImage.
type UploadImageParams struct {
	Data []byte
	// ContentType is image/jpeg, image/jpg, image/png, image/gif or
	// image/webp. Empty detects it from Data.
	ContentType string
	Name        string
	Description Optional[string]
	FolderID    Optional[string]
}

// ErrUnsupportedImage reports image data whose type is not one the API
// accepts, or cannot be detected.
var ErrUnsupportedImage = errors.New("lk: image type must be one of image/jpeg, image/jpg, image/png, image/gif or image/webp")

// UploadError reports that storage refused an upload's bytes.
type UploadError struct {
	// Status is the storage's HTTP status.
	Status int
	// Body is a short excerpt of the storage's answer.
	Body string
}

func (e *UploadError) Error() string {
	msg := fmt.Sprintf("lk: storage refused the image upload: %d %s", e.Status, http.StatusText(e.Status))
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// UploadImage uploads one image and returns the created asset: it calls
// CreateImageUpload with the data's length and SHA-256, sends the bytes to
// storage with only the returned headers (never the API key), and calls
// CreateImage. The whole file is held in memory. The storage request gets
// the client's timeout but no retries; on failure call UploadImage again,
// which starts a new upload (DECISIONS.md D-008, A-31).
func (s *AssetsService) UploadImage(ctx context.Context, p UploadImageParams, opts ...CallOption) (*Asset, error) {
	ct := p.ContentType
	if ct == "" {
		ct = http.DetectContentType(p.Data)
	}
	if !slices.Contains(imageContentTypes(), ct) {
		return nil, ErrUnsupportedImage
	}
	sum := sha256.Sum256(p.Data)
	up, err := s.CreateImageUpload(ctx, ImageUploadCreate{
		ContentType: ct, ContentLength: int64(len(p.Data)), ContentHash: Some(hex.EncodeToString(sum[:])),
	}, opts...)
	if err != nil {
		return nil, err
	}
	if err := s.c.putStorage(ctx, up, ct, p.Data); err != nil {
		return nil, err
	}
	return s.CreateImage(ctx, ImageCreate{UploadKey: up.UploadKey, Name: p.Name, Description: p.Description, FolderID: p.FolderID}, opts...)
}

// putStorage sends data to a signed storage URL. It sends the returned
// headers and nothing else of the client's: no Authorization, User-Agent
// or idempotency key. Content-Type is added only when the headers lack it
// (DECISIONS.md D-009, A-23).
func (c *Client) putStorage(ctx context.Context, up *ImageUpload, contentType string, data []byte) error {
	actx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if c.timeout > 0 {
		stop := c.clock.AfterFunc(c.timeout, func() { cancel(ErrTimeout) })
		defer stop()
	}
	method := up.Method
	if method == "" {
		method = http.MethodPut
	}
	req, err := http.NewRequestWithContext(actx, method, up.UploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("lk: building storage upload: %w", err)
	}
	req.Header = http.Header{}
	for k, v := range up.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", contentType)
	}
	// Go adds its own User-Agent unless one is set; send none.
	req.Header["User-Agent"] = nil
	resp, err := c.hc.Do(req)
	if err != nil {
		if errors.Is(context.Cause(actx), ErrTimeout) && ctx.Err() == nil {
			err = fmt.Errorf("%w after %s", ErrTimeout, c.timeout)
		}
		return &ConnectionError{Method: method, Path: "(storage upload)", Err: err}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &UploadError{Status: resp.StatusCode, Body: errorText(body)}
	}
	return nil
}

func assetFromWire(w gen.Asset) Asset {
	return Asset{
		ID: w.ID, Name: w.Name, Type: AssetType(w.Type), FolderID: w.FolderID.Or(""),
		IsHidden: w.IsHidden.Or(false), Description: w.Description.Or(""), URL: w.URL.Or(""),
		Width: int(w.Width.Or(0)), Height: int(w.Height.Or(0)),
		CreatedAt: utc(w.CreatedAt.Or(time.Time{})),
	}
}
