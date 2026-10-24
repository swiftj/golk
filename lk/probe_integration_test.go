//go:build integration

package lk_test

// Probes record server behavior the docs leave open (SDK_CONTRACT.md
// A-02, A-05, A-06, A-16 to A-19, A-23, A-24). They go through the escape
// hatch so they depend on nothing but the transport. Run them with
//
//	make test-integration ARGS='-run TestProbe -v'
//
// and copy the observations into DECISIONS.md. Everything they write is
// made up and deleted when they finish.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

type prober struct {
	t   *testing.T
	c   *lk.Client
	pid string
}

func newProber(t *testing.T) *prober {
	return &prober{t: t, c: sandbox.Client(t), pid: sandbox.ProjectID(t)}
}

// do sends a raw request under the project and returns the decoded body,
// the response metadata and the error.
func (p *prober) do(method, path string, body any, opts ...lk.CallOption) (map[string]any, lk.ResponseMeta, error) {
	p.t.Helper()
	var raw json.RawMessage
	var meta lk.ResponseMeta
	h := http.Header{}
	if method == http.MethodPost {
		h.Set("Idempotency-Key", sandbox.RunPrefix+"-"+lk.NewID())
	}
	err := p.c.Do(sandbox.Ctx(p.t), lk.Request{Method: method, Path: "/api/v3/projects/" + p.pid + path, Body: body, Header: h},
		&raw, append(opts, lk.CaptureResponse(&meta))...)
	var out map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
	}
	return out, meta, err
}

func (p *prober) must(method, path string, body any, opts ...lk.CallOption) (map[string]any, lk.ResponseMeta) {
	p.t.Helper()
	out, meta, err := p.do(method, path, body, opts...)
	if err != nil {
		p.t.Fatalf("%s %s: %v", method, path, err)
	}
	return out, meta
}

// page creates a resource and returns the path of its first document.
func (p *prober) page(name string) (resourceID, docPath string) {
	p.t.Helper()
	r := create(p.t, p.c, lk.ResourceCreate{Name: name})
	return r.ID, "/resources/" + r.ID + "/docs/" + r.Documents[0].ID
}

func show(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func apiErr(t *testing.T, err error) *lk.APIError {
	t.Helper()
	var e *lk.APIError
	if !errors.As(err, &e) {
		t.Fatalf("want *APIError, got %v", err)
	}
	return e
}

// TestProbe_Versions covers A-06 (ETag form), A-05 (412 message), A-02
// (errors[] on DUPLICATE_ID) and A-19 (row write versions).
func TestProbe_Versions(t *testing.T) {
	p := newProber(t)
	_, doc := p.page("Probe versions")

	read, meta := p.must("GET", doc+"/blocks", nil)
	t.Logf("A-06 blank getBlocks: body version=%q ETag=%q type of schema=%T", read["version"], meta.ETag, read["schema"])

	blocks := []any{
		map[string]any{"kind": "paragraph", "id": "probe-p1", "content": []any{map[string]any{"text": "Glob"}}},
		map[string]any{"kind": "keyValue", "rows": []any{map[string]any{"key": "Lumps", "text": "Many"}}},
	}
	written, meta := p.must("PUT", doc+"/blocks", map[string]any{"blocks": blocks}, lk.IfMatch(fmt.Sprint(read["version"])))
	t.Logf("A-06 replaceBlocks: body version=%q ETag=%q", written["version"], meta.ETag)

	// Which If-Match forms does the server accept? The current version as
	// the body gives it, quoted, and as the ETag gives it.
	cur := fmt.Sprint(written["version"])
	for _, wrap := range []func(string) string{
		func(v string) string { return v },
		func(v string) string { return `"` + v + `"` },
		func(v string) string { return `W/"` + v + `"` },
	} {
		form := wrap(cur)
		again, meta, err := p.do("PUT", doc+"/blocks", map[string]any{"blocks": blocks}, lk.IfMatch(form))
		t.Logf("A-06 If-Match %q: err=%v status=%d", form, err, meta.Status)
		if err == nil {
			cur = fmt.Sprint(again["version"])
		}
	}
	written["version"] = cur

	_, _, err := p.do("PUT", doc+"/blocks", map[string]any{"blocks": blocks}, lk.IfMatch(fmt.Sprint(read["version"])))
	e := apiErr(t, err)
	t.Logf("A-05 stale If-Match: status=%d code=%s message=%q current=%q (written version %q)", e.Status, e.Code, e.Message, e.CurrentVersion, written["version"])

	_, _, err = p.do("POST", doc+"/blocks", map[string]any{"block": map[string]any{"kind": "paragraph", "id": "probe-p1", "content": []any{}}})
	e = apiErr(t, err)
	t.Logf("A-02 duplicate block id: status=%d code=%s errors=%s", e.Status, e.Code, show(e.Errors))

	got, _ := p.must("GET", doc+"/blocks", nil)
	kv := got["blocks"].([]any)[1].(map[string]any)
	kvPath := doc + "/blocks/" + fmt.Sprint(kv["id"])
	row, meta := p.must("POST", kvPath+"/rows", map[string]any{"key": "Princess", "text": "Yes"}, lk.IfMatch(fmt.Sprint(got["version"])))
	t.Logf("A-19 createRow: body=%s ETag=%q status=%d", show(row), meta.ETag, meta.Status)
	rows, meta := p.must("GET", kvPath+"/rows", nil)
	t.Logf("A-19 listRows: version=%q ETag=%q", rows["version"], meta.ETag)
	upd, meta := p.must("PATCH", kvPath+"/rows/"+fmt.Sprint(row["id"]), map[string]any{"content": nil}, lk.IfMatch(fmt.Sprint(rows["version"])))
	t.Logf("A-19 updateRow content:null: body=%s ETag=%q", show(upd), meta.ETag)
	blk, meta := p.must("GET", kvPath, nil)
	t.Logf("A-06 getBlock: body version=%q ETag=%q", blk["version"], meta.ETag)
	_, meta, err = p.do("DELETE", kvPath+"/rows/"+fmt.Sprint(row["id"]), nil, lk.IfMatch(fmt.Sprint(blk["version"])))
	t.Logf("A-20 deleteRow: status=%d ETag=%q err=%v", meta.Status, meta.ETag, err)
}

// textFixture holds one of every text-bearing block, each marked with a
// distinct word so the server's text can be read back line by line.
func textFixture(mentionID string) []any {
	run := func(s string) []any { return []any{map[string]any{"text": s}} }
	para := func(s string) map[string]any { return map[string]any{"kind": "paragraph", "content": run(s)} }
	return []any{
		map[string]any{"kind": "heading", "level": 1, "content": run("Heading")},
		map[string]any{"kind": "paragraph", "content": []any{
			map[string]any{"text": "Bold", "bold": true}, map[string]any{"text": " plain "},
			map[string]any{"break": true},
			map[string]any{"text": "after break"},
			map[string]any{"text": " "},
			map[string]any{"mention": map[string]any{"resourceId": mentionID, "text": "Label"}},
			map[string]any{"text": " "},
			map[string]any{"mention": map[string]any{"resourceId": mentionID}},
		}},
		para(""),
		map[string]any{"kind": "quote", "children": []any{para("Quote1"), para("Quote2")}},
		map[string]any{"kind": "code", "text": "code line 1\ncode line 2", "language": "js"},
		map[string]any{"kind": "rule"},
		map[string]any{"kind": "bulletList", "children": []any{
			map[string]any{"kind": "listItem", "children": []any{para("Bullet1"),
				map[string]any{"kind": "orderedList", "children": []any{map[string]any{"kind": "listItem", "children": []any{para("Nested1")}}}}}},
			map[string]any{"kind": "listItem", "children": []any{para("Bullet2")}},
		}},
		map[string]any{"kind": "taskList", "children": []any{map[string]any{"kind": "taskItem", "checked": true, "content": run("Task1")}}},
		map[string]any{"kind": "table", "children": []any{
			map[string]any{"kind": "tableRow", "children": []any{
				map[string]any{"kind": "tableCell", "header": true, "children": []any{para("H1")}},
				map[string]any{"kind": "tableCell", "header": true, "children": []any{para("H2")}},
			}},
			map[string]any{"kind": "tableRow", "children": []any{
				map[string]any{"kind": "tableCell", "children": []any{para("C1")}},
				map[string]any{"kind": "tableCell", "children": []any{para("C2")}},
			}},
		}},
		map[string]any{"kind": "callout", "tone": "info", "children": []any{para("Callout")}},
		map[string]any{"kind": "expand", "title": "ExpandTitle", "children": []any{para("ExpandBody")}},
		map[string]any{"kind": "layout", "children": []any{
			map[string]any{"kind": "layoutColumn", "children": []any{para("Col1")}},
			map[string]any{"kind": "layoutColumn", "children": []any{para("Col2")}},
		}},
		map[string]any{"kind": "keyValue", "title": "KVTitle", "rows": []any{
			map[string]any{"key": "Key1", "text": "Value1"},
			map[string]any{"key": "Key2", "content": []any{map[string]any{"text": "V2a"}, map[string]any{"break": true}, map[string]any{"text": "V2b"}}},
		}},
		map[string]any{"kind": "secret", "children": []any{para("Secret")}},
		map[string]any{"kind": "textField", "title": "TFTitle", "children": []any{para("TextField")}},
		map[string]any{"kind": "infobox", "rows": []any{
			map[string]any{"kind": "keyValue", "title": "IBTitle", "rows": []any{map[string]any{"key": "IBKey", "text": "IBValue"}}},
			map[string]any{"kind": "textField", "children": []any{para("IBText")}},
		}},
		map[string]any{"kind": "meter", "mode": "bar", "title": "MeterTitle", "items": []any{map[string]any{"name": "HP", "icon": "heart", "current": 3, "max": 5}}},
		map[string]any{"kind": "resourceLink", "items": []any{map[string]any{"resourceId": mentionID}}},
		para("Last"),
	}
}

// TestProbe_PlainText covers A-17 (DocumentBlocks.text) and A-24 (mention
// name echo).
func TestProbe_PlainText(t *testing.T) {
	p := newProber(t)
	target := create(t, p.c, lk.ResourceCreate{Name: "Probe mention target"})
	_, doc := p.page("Probe text")

	written, _ := p.must("PUT", doc+"/blocks", map[string]any{"blocks": textFixture(target.ID)})
	t.Logf("A-17 text=%q", written["text"])
	t.Logf("A-17 blocks=%s", show(written["blocks"]))

	// Echo the read blocks, including the mention's read-only name.
	echoed, _, err := p.do("PUT", doc+"/blocks", map[string]any{"blocks": written["blocks"]})
	t.Logf("A-24 echo with mention name: err=%v", err)
	if err == nil {
		t.Logf("A-24 echoed text equal: %v", echoed["text"] == written["text"])
	}
}

// TestProbe_ErrorPaths covers A-18: the dotted path the server reports for
// a bad nested block and a bad infobox row.
func TestProbe_ErrorPaths(t *testing.T) {
	p := newProber(t)
	_, doc := p.page("Probe paths")
	nested := []any{
		map[string]any{"kind": "paragraph", "content": []any{}},
		map[string]any{"kind": "quote", "children": []any{
			map[string]any{"kind": "callout", "tone": "info", "children": []any{
				map[string]any{"kind": "paragraph", "content": []any{}, "bogus": 1}}}}},
	}
	_, _, err := p.do("PUT", doc+"/blocks", map[string]any{"blocks": nested})
	e := apiErr(t, err)
	t.Logf("A-18 nested unknown field: code=%s errors=%s", e.Code, show(e.Errors))

	infobox := []any{map[string]any{"kind": "infobox", "rows": []any{
		map[string]any{"kind": "keyValue", "rows": []any{}},
		map[string]any{"kind": "textField", "children": []any{map[string]any{"kind": "paragraph", "content": []any{}, "bogus": 1}}},
	}}}
	_, _, err = p.do("PUT", doc+"/blocks", map[string]any{"blocks": infobox})
	e = apiErr(t, err)
	t.Logf("A-18 infobox row: code=%s errors=%s", e.Code, show(e.Errors))

	_, _, err = p.do("PUT", doc+"/blocks", map[string]any{"blocks": []any{map[string]any{"kind": "listItem", "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}}}})
	e = apiErr(t, err)
	t.Logf("A-18 top-level listItem: code=%s errors=%s", e.Code, show(e.Errors))
}

func tinyPNG(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for i := range 16 {
		img.Set(i%4, i/4, color.RGBA{R: 0x4c, G: 0xaf, B: 0x50, A: 0xff})
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestProbe_UploadAndReplace covers A-23 (upload headers) and A-16 (which
// optional fields survive a replace that omits them).
func TestProbe_UploadAndReplace(t *testing.T) {
	p := newProber(t)
	data := tinyPNG(t)
	sum := sha256.Sum256(data)
	up, _ := p.must("POST", "/assets/uploads", map[string]any{"contentType": "image/png", "contentLength": len(data), "contentHash": hex.EncodeToString(sum[:])})
	hdrs, _ := up["headers"].(map[string]any)
	var names []string
	for k := range hdrs {
		names = append(names, k)
	}
	slices.Sort(names)
	u, _ := url.Parse(fmt.Sprint(up["uploadUrl"]))
	t.Logf("A-23 upload: method=%v host=%s header names=%v content-type=%v expiresAt=%v", up["method"], u.Host, names, hdrs["content-type"], up["expiresAt"])

	req, err := http.NewRequestWithContext(sandbox.Ctx(t), fmt.Sprint(up["method"]), u.String(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdrs {
		req.Header.Set(k, fmt.Sprint(v))
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	t.Logf("A-23 storage PUT with only the returned headers: status=%d", resp.StatusCode)

	asset, meta := p.must("POST", "/assets/images", map[string]any{"uploadKey": up["uploadKey"], "name": sandbox.Name("probe image")})
	t.Logf("A-23 createImage: status=%d asset=%s", meta.Status, show(asset))
	assetID := fmt.Sprint(asset["id"])
	t.Cleanup(func() {
		if _, _, err := p.do("DELETE", "/assets/"+assetID, nil); err != nil && !errors.Is(err, lk.ErrNotFound) {
			t.Errorf("deleting probe asset: %v", err)
		}
	})

	_, doc := p.page("Probe replace")
	full := []any{
		map[string]any{"kind": "paragraph", "align": "center", "background": "#112233", "content": []any{map[string]any{"text": "styled"}}},
		map[string]any{"kind": "image", "url": asset["url"], "alt": "a green square", "fill": true, "title": "Square", "isTitleHidden": false, "layout": "wrapLeft", "width": 30},
		map[string]any{"kind": "layout", "children": []any{
			map[string]any{"kind": "layoutColumn", "width": 30, "background": "#445566", "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}},
			map[string]any{"kind": "layoutColumn", "width": 70, "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}},
		}},
		map[string]any{"kind": "callout", "tone": "custom", "icon": "dragon", "color": "#aa0000", "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}},
	}
	written, _ := p.must("PUT", doc+"/blocks", map[string]any{"blocks": full})
	t.Logf("A-16 written=%s", show(written["blocks"]))

	// Replace each block by id with only its required fields.
	bs := written["blocks"].([]any)
	id := func(i int) string { return fmt.Sprint(bs[i].(map[string]any)["id"]) }
	cols := bs[2].(map[string]any)["children"].([]any)
	bare := []any{
		map[string]any{"kind": "paragraph", "id": id(0), "content": []any{map[string]any{"text": "styled"}}},
		map[string]any{"kind": "image", "id": id(1), "url": asset["url"]},
		map[string]any{"kind": "layout", "id": id(2), "children": []any{
			map[string]any{"kind": "layoutColumn", "id": cols[0].(map[string]any)["id"], "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}},
			map[string]any{"kind": "layoutColumn", "id": cols[1].(map[string]any)["id"], "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}},
		}},
		map[string]any{"kind": "callout", "id": id(3), "tone": "custom", "children": []any{map[string]any{"kind": "paragraph", "content": []any{}}}},
	}
	after, _ := p.must("PUT", doc+"/blocks", map[string]any{"blocks": bare})
	t.Logf("A-16 after replaceBlocks without optional fields=%s", show(after["blocks"]))

	// The same through blocks.replace on the image alone.
	_, _ = p.must("PUT", doc+"/blocks", map[string]any{"blocks": full[:2]})
	again, _ := p.must("GET", doc+"/blocks", nil)
	img := again["blocks"].([]any)[1].(map[string]any)
	one, _ := p.must("PUT", doc+"/blocks/"+fmt.Sprint(img["id"]), map[string]any{"kind": "image", "url": asset["url"]})
	t.Logf("A-16 blocks.replace image without optional fields=%s", show(one))

	// A data: URI is refused with a path (C-CONTENT-03).
	_, _, err = p.do("PUT", doc+"/blocks", map[string]any{"blocks": []any{map[string]any{"kind": "image", "url": "data:image/png;base64,AAAA"}}})
	e := apiErr(t, err)
	t.Logf("C-CONTENT-03 data URI: code=%s errors=%s", e.Code, show(e.Errors))
	if !strings.Contains(show(e.Errors), "url") {
		t.Errorf("data: URI error has no url path: %s", show(e.Errors))
	}
}
