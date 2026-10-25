package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

var wtPNG = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 24)...)

func wtFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestUploadImage_X_TOOL_07_Uploads(t *testing.T) {
	h := newHarness(t)
	up, _ := json.Marshal(map[string]any{"uploadKey": "k1", "uploadUrl": h.lk.URL + "/bucket/k1?sig=abc", "method": "PUT",
		"headers": map[string]string{"content-type": "image/png"}, "expiresAt": "2026-01-02T03:14:05Z"})
	h.lk.Enqueue(wtOK(string(up)), wtOK(""),
		testutil.JSON(http.StatusCreated, `{"id":"a1","name":"Map","type":"IMAGE","isHidden":false,"url":"https://assets.example/p.png","width":4,"height":3,"createdAt":"2026-01-02T03:04:05Z"}`))
	r := h.call("upload_image", map[string]any{"path": wtFile(t, "m.png", wtPNG), "name": "Map", "folder_id": "f1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	var out UploadImageOut
	structured(t, r, &out)
	if out != (UploadImageOut{AssetID: "a1", Name: "Map", URL: "https://assets.example/p.png", Width: 4, Height: 3, Bytes: 32, ContentType: "image/png"}) {
		t.Errorf("structured %+v", out)
	}
	w := h.writes()
	if len(w) != 3 {
		t.Fatalf("writes %+v", w)
	}
	sum := sha256.Sum256(wtPNG)
	wtReq(t, w[0], "POST", "/api/v3/projects/p1/assets/uploads", "", `{"contentType":"image/png","contentLength":32,"contentHash":"`+hex.EncodeToString(sum[:])+`"}`)
	if w[1].Method != "PUT" || w[1].Path != "/bucket/k1" || string(w[1].Body) != string(wtPNG) || w[1].Header.Get("Authorization") != "" {
		t.Errorf("storage request %+v", w[1])
	}
	wtReq(t, w[2], "POST", "/api/v3/projects/p1/assets/images", "", `{"uploadKey":"k1","name":"Map","folderId":"f1"}`)
	if len(h.guard.checked) != 1 || h.guard.checked[0] != `{"description":"","name":"Map"}` {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestUploadImage_X_TOOL_07_RefusedBeforeAnyRequest(t *testing.T) {
	h := newHarness(t)
	wtMustFail(t, h, "upload_image", map[string]any{"path": wtFile(t, "a.txt", []byte("plain text, not an image")), "name": "x"}, "image type must be")
	wtMustFail(t, h, "upload_image", map[string]any{"path": filepath.Join(t.TempDir(), "missing.png"), "name": "x"}, "can't read")
	big := filepath.Join(t.TempDir(), "big.png")
	if err := os.WriteFile(big, wtPNG, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, maxImageBytes+1); err != nil {
		t.Fatal(err)
	}
	wtMustFail(t, h, "upload_image", map[string]any{"path": big, "name": "x"}, "memory safety cap")
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}

func TestUploadImage_X_TOOL_04_X_GUARD_07(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	p := wtFile(t, "m.png", wtPNG)
	got := wtMustOK(t, h, "upload_image", map[string]any{"path": p, "name": "Map", "dry_run": true})
	if !strings.HasPrefix(got, "Dry run: would upload") || !strings.Contains(got, "image/png, 32 bytes") {
		t.Errorf("text %q", got)
	}
	r := h.call("upload_image", map[string]any{"path": p, "name": "Map", "description": "Zorblax lair"})
	wtRefused(t, h, text(r))
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}

const (
	daImage  = `{"id":"a1","name":"Ascore - Factions","type":"IMAGE","url":"https://assets.example/old.png","width":1124,"height":1999,"isHidden":false}`
	daFolder = `{"id":"f1","name":"Old art","type":"FOLDER","isHidden":false}`
	daPath   = "/api/v3/projects/p1/assets"
)

// X-ASSET-01: delete_asset reads the asset, then deletes it; the text names
// it and says what happens to pages that show it.
func TestDeleteAsset_X_ASSET_01(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(daImage), testutil.Reply{Status: http.StatusNoContent})
	got := wtMustOK(t, h, "delete_asset", map[string]any{"asset_id": "a1"})
	if !strings.HasPrefix(got, `Deleted image "Ascore - Factions" (a1) from the asset library. Pages that already show it keep it`) {
		t.Errorf("text %q", got)
	}
	reqs := h.lk.Requests()
	wtReq(t, reqs[0], http.MethodGet, daPath+"/a1", "", "")
	wtReq(t, reqs[1], http.MethodDelete, daPath+"/a1", "", "")
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v; deleting shows players nothing", h.guard.checked)
	}

	// A dry run reads only.
	h = newHarness(t)
	h.lk.Enqueue(wtOK(daImage))
	if got := wtMustOK(t, h, "delete_asset", map[string]any{"asset_id": "a1", "dry_run": true}); !strings.HasPrefix(got, `Dry run: would delete image "Ascore - Factions" (a1)`) ||
		!strings.HasSuffix(got, "Nothing was written.") || len(h.writes()) != 0 {
		t.Errorf("text %q, writes %+v", got, h.writes())
	}
}

// X-ASSET-01: a folder's dry run counts what is inside it at any depth.
func TestDeleteAsset_X_ASSET_01_Folder(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(daFolder),
		wtOK(`{"data":[`+daImage+`,{"id":"f2","name":"Sub","type":"FOLDER"}],"hasMore":false}`),
		wtOK(`{"data":[{"id":"a3","name":"Deep","type":"IMAGE","url":"https://assets.example/d.png"}],"hasMore":false}`))
	got := wtMustOK(t, h, "delete_asset", map[string]any{"asset_id": "f1", "dry_run": true})
	if !strings.Contains(got, `would delete folder "Old art" (f1) from the asset library. Everything inside it goes too: 3 asset(s).`) {
		t.Errorf("text %q", got)
	}
	reqs := h.lk.Requests()
	wtReq(t, reqs[1], http.MethodGet, daPath, "folderId=f1&limit=100", "")
	wtReq(t, reqs[2], http.MethodGet, daPath, "folderId=f2&limit=100", "")
}

// X-ASSET-01: a key without the ADMIN role gets the API's error with a hint.
func TestDeleteAsset_X_ASSET_01_Forbidden(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(daImage), testutil.JSON(http.StatusForbidden, `{"message":"Admin role required","code":"FORBIDDEN","requestId":"rq9"}`))
	got := wtMustFail(t, h, "delete_asset", map[string]any{"asset_id": "a1"}, "an admin key may be needed")
	if !strings.Contains(got, "rq9") {
		t.Errorf("text %q lacks the request id", got)
	}
}
