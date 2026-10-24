package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"

	"github.com/swiftj/golk/lk"
)

// maxImageBytes caps the file upload_image reads. It is a client-side
// memory safety cap (the whole file is held in memory to hash and send),
// not a LegendKeeper limit.
const maxImageBytes = 50 << 20

// imageTypes are the content types the API accepts for images (the list in
// lk's ErrUnsupportedImage).
var imageTypes = []string{"image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp"}

// UploadImageIn is upload_image's input.
type UploadImageIn struct {
	ProjectArg
	DryRunArg
	Path        string `json:"path" jsonschema:"the local image file (JPEG, PNG, GIF or WebP)"`
	Name        string `json:"name" jsonschema:"the asset's name in the library"`
	Description string `json:"description,omitempty" jsonschema:"the asset's description"`
	FolderID    string `json:"folder_id,omitempty" jsonschema:"an asset folder id; omit for the library root"`
}

// UploadImageOut is upload_image's output.
type UploadImageOut struct {
	AssetID     string `json:"asset_id,omitempty"`
	Name        string `json:"name"`
	URL         string `json:"url,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Bytes       int    `json:"bytes"`
	ContentType string `json:"content_type"`
	DryRun      bool   `json:"dry_run,omitempty"`
}

func (s *server) uploadImage(ctx context.Context, c *lk.Client, in UploadImageIn) (toolResult[UploadImageOut], error) {
	var zero toolResult[UploadImageOut]
	if strings.TrimSpace(in.Name) == "" {
		return zero, errors.New("name is required")
	}
	fi, err := os.Stat(in.Path)
	if err != nil {
		return zero, fmt.Errorf("can't read %s: %w", in.Path, err)
	}
	if !fi.Mode().IsRegular() {
		return zero, fmt.Errorf("%s is not a regular file", in.Path)
	}
	if fi.Size() > maxImageBytes {
		return zero, fmt.Errorf("%s is %d bytes; upload_image reads at most %d (a memory safety cap of this server, not a LegendKeeper limit)", in.Path, fi.Size(), maxImageBytes)
	}
	data, err := os.ReadFile(in.Path)
	if err != nil {
		return zero, fmt.Errorf("can't read %s: %w", in.Path, err)
	}
	if len(data) > maxImageBytes {
		return zero, fmt.Errorf("%s is over %d bytes (a memory safety cap of this server)", in.Path, maxImageBytes)
	}
	// The same detection lk does, done here so a dry run refuses too and
	// nothing is sent for a file the API won't take.
	ct := http.DetectContentType(data)
	if !slices.Contains(imageTypes, ct) {
		return zero, fmt.Errorf("%s is %s: %w", in.Path, ct, lk.ErrUnsupportedImage)
	}
	// Asset names and descriptions can be shown to players.
	if err := s.check(map[string]any{"name": in.Name, "description": in.Description}); err != nil {
		return zero, err
	}
	out := UploadImageOut{Name: in.Name, Bytes: len(data), ContentType: ct, DryRun: in.DryRun}
	if in.DryRun {
		return toolResult[UploadImageOut]{out: out, text: fmt.Sprintf("Dry run: would upload %s (%s, %d bytes) as image %q. Nothing was written.", in.Path, ct, len(data), in.Name)}, nil
	}
	p := lk.UploadImageParams{Data: data, ContentType: ct, Name: in.Name}
	if in.Description != "" {
		p.Description = lk.Some(in.Description)
	}
	if in.FolderID != "" {
		p.FolderID = lk.Some(in.FolderID)
	}
	a, err := c.Assets.UploadImage(ctx, p)
	if err != nil {
		return zero, err
	}
	out.AssetID, out.Name, out.URL, out.Width, out.Height = a.ID, a.Name, a.URL, a.Width, a.Height
	return toolResult[UploadImageOut]{out: out, text: fmt.Sprintf("Uploaded image %q (asset %s, %dx%d): %s", a.Name, a.ID, a.Width, a.Height, a.URL)}, nil
}
