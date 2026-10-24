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
