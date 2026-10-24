package lk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// pngBytes is the 8-byte PNG signature plus padding: enough for
// http.DetectContentType to report image/png.
var pngBytes = append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 24)...)

type storage struct {
	*httptest.Server
	mu     sync.Mutex
	method string
	header http.Header
	body   []byte
	status int
}

func newStorage(t *testing.T, status int) *storage {
	s := &storage{status: status}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.method, s.header, s.body = r.Method, r.Header.Clone(), b
		s.mu.Unlock()
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte("<Error><Code>BadDigest</Code></Error>"))
	}))
	t.Cleanup(s.Close)
	return s
}

func uploadReply(t *testing.T, storageURL string, headers map[string]string) testutil.Reply {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"uploadKey": "k1", "uploadUrl": storageURL + "/bucket/k1?sig=abc", "method": "PUT",
		"headers": headers, "expiresAt": "2026-01-02T03:14:05Z"})
	return testutil.JSON(200, string(b))
}

func TestAssets_C_ASSET_08_C_ASSET_09_UploadImage(t *testing.T) {
	st := newStorage(t, 200)
	sum := sha256.Sum256(pngBytes)
	hash := hex.EncodeToString(sum[:])
	srv := testutil.NewFakeServer(t,
		uploadReply(t, st.URL, map[string]string{"content-type": "image/png", "x-amz-checksum-sha256": "c2lnbmVk"}),
		testutil.JSON(201, assetJSON))
	c, _ := newTestClient(t, srv)
	a, err := c.Assets.UploadImage(context.Background(), UploadImageParams{Data: pngBytes, Name: "Portrait", FolderID: Some("f1")})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != "a1" {
		t.Errorf("asset = %+v", a)
	}
	reqs := srv.Requests()
	assertJSON(t, reqs[0].Body, `{"contentHash":"`+hash+`","contentLength":32,"contentType":"image/png"}`)
	assertJSON(t, reqs[1].Body, `{"folderId":"f1","name":"Portrait","uploadKey":"k1"}`)

	// Storage got the bytes with exactly the returned headers and nothing
	// of the client's: no key, no SDK user agent, no idempotency key.
	if st.method != "PUT" || string(st.body) != string(pngBytes) {
		t.Errorf("storage got %s %d bytes", st.method, len(st.body))
	}
	if st.header.Get("Content-Type") != "image/png" || st.header.Get("X-Amz-Checksum-Sha256") != "c2lnbmVk" {
		t.Errorf("storage headers = %v", st.header)
	}
	for _, h := range []string{"Authorization", "Idempotency-Key", "If-Match", "User-Agent", "Accept"} {
		if v := st.header.Get(h); v != "" {
			t.Errorf("storage got %s: %q", h, v)
		}
	}
}

func TestAssets_C_ASSET_09_UploadAddsMissingContentTypeAndReportsRefusal(t *testing.T) {
	st := newStorage(t, 400)
	srv := testutil.NewFakeServer(t, uploadReply(t, st.URL, map[string]string{}))
	c, _ := newTestClient(t, srv)
	_, err := c.Assets.UploadImage(context.Background(), UploadImageParams{Data: pngBytes, ContentType: "image/png", Name: "x"})
	var ue *UploadError
	if !errors.As(err, &ue) || ue.Status != 400 || ue.Body == "" {
		t.Fatalf("err = %#v", err)
	}
	if st.header.Get("Content-Type") != "image/png" {
		t.Errorf("Content-Type not added: %v", st.header)
	}
	// The storage refusal is not retried, and createImage is never called.
	if n := len(srv.Requests()); n != 1 {
		t.Errorf("%d API requests", n)
	}
}

func TestAssets_C_ASSET_08_ContentTypeChecked(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	for _, p := range []UploadImageParams{
		{Data: []byte("plain text, not an image"), Name: "x"},
		{Data: pngBytes, ContentType: "image/svg+xml", Name: "x"},
	} {
		if _, err := c.Assets.UploadImage(context.Background(), p); !errors.Is(err, ErrUnsupportedImage) {
			t.Errorf("%q: err = %v", p.ContentType, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}
