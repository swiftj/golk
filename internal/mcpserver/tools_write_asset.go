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

// maxFolderScan caps how many assets a folder delete's description counts.
const maxFolderScan = 1000

// DeleteAssetIn is delete_asset's input.
type DeleteAssetIn struct {
	ProjectArg
	DryRunArg
	AssetID string `json:"asset_id" jsonschema:"the asset (image, map image, audio or folder) to delete, from list_assets or upload_image"`
}

// DeleteAssetOut is delete_asset's output.
type DeleteAssetOut struct {
	AssetID string `json:"asset_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url,omitempty"`
	// Contents is, for a folder, how many assets inside it (at any depth)
	// go with it; ContentsCapped means there are at least that many.
	Contents       int  `json:"contents,omitempty"`
	ContentsCapped bool `json:"contents_capped,omitempty"`
	DryRun         bool `json:"dry_run,omitempty"`
}

// folderContents counts the assets inside folder id at any depth, up to
// maxFolderScan.
func folderContents(ctx context.Context, c *lk.Client, id string) (n int, capped bool, err error) {
	queue := []string{id}
	for len(queue) > 0 {
		f := queue[0]
		queue = queue[1:]
		for a, err := range c.Assets.All(ctx, lk.AssetListParams{FolderID: f, Limit: 100}) {
			if err != nil {
				return n, false, err
			}
			if n++; n >= maxFolderScan {
				return n, true, nil
			}
			if a.Type == lk.AssetFolder {
				queue = append(queue, a.ID)
			}
		}
	}
	return n, false, nil
}

// assetInUseNote is what deleting an image means for pages that show it
// (DECISIONS.md D-020, A-41).
const assetInUseNote = "Pages that already show it keep it (its URL still loaded right after a delete in testing, though LegendKeeper doesn't document for how long), " +
	"but its URL can't be added anywhere new. Replace it on those pages first if you want them to stay correct."

func (s *server) deleteAsset(ctx context.Context, c *lk.Client, in DeleteAssetIn) (toolResult[DeleteAssetOut], error) {
	var zero toolResult[DeleteAssetOut]
	if strings.TrimSpace(in.AssetID) == "" {
		return zero, errors.New("asset_id is required")
	}
	// No guard: deleting takes content away from players, it can't show
	// them anything.
	a, err := c.Assets.Get(ctx, in.AssetID)
	if err != nil {
		return zero, err
	}
	out := DeleteAssetOut{AssetID: a.ID, Name: a.Name, Type: string(a.Type), URL: a.URL, DryRun: in.DryRun}
	what := fmt.Sprintf("%s %q (%s)", strings.ToLower(string(a.Type)), a.Name, a.ID)
	note := ""
	if a.Type == lk.AssetFolder {
		if out.Contents, out.ContentsCapped, err = folderContents(ctx, c, a.ID); err != nil {
			return zero, err
		}
		more := ""
		if out.ContentsCapped {
			more = " at least"
		}
		note = fmt.Sprintf(" Everything inside it goes too:%s %d asset(s).", more, out.Contents)
	} else if a.URL != "" {
		note = " " + assetInUseNote
	}
	if in.DryRun {
		return toolResult[DeleteAssetOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s from the asset library.%s Nothing was written.", what, note)}, nil
	}
	if err := c.Assets.Delete(ctx, a.ID); err != nil {
		return zero, err
	}
	return toolResult[DeleteAssetOut]{out: out, text: fmt.Sprintf("Deleted %s from the asset library.%s", what, note)}, nil
}
