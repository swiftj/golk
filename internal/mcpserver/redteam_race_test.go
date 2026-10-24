package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/mcpserver/guard"
)

// blockingGuard is a real guard whose first Check waits until released.
type blockingGuard struct {
	g        *guard.Guard
	once     sync.Once
	entered  chan struct{}
	released chan struct{}
}

func (b *blockingGuard) Check(v any) ([]guard.Violation, error) {
	b.once.Do(func() {
		close(b.entered)
		<-b.released
	})
	return b.g.Check(v)
}

// A board document's content can't be read by the guard yet (Phase 5),
// so lk-mcp refuses to show one: unhiding the document, or unhiding a
// resource with such a document visible.
func TestRedteam_X_GUARD_05_UncheckableDocumentsStayHidden(t *testing.T) {
	hiddenMap := `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"m1","name":"Board","type":"board","rank":"c","isHidden":true}]}`
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(hiddenMap))
	wtMustFail(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "m1", "is_hidden": false}, "can't read")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}

	resHidden := `{"id":"r1","name":"Ooo","isHidden":true,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"m1","name":"Board","type":"board","rank":"c","isHidden":false}]}`
	h = rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(resHidden), wtOK(rgBlocks))
	wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "is_hidden": false, "dry_run": true}, "can't read")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

// Without a terms file every write tool refuses before any request, even
// those that can't show players anything (D-011).
func TestRedteam_X_GUARD_06_NoGuardRefusesEveryWrite(t *testing.T) {
	h := newHarness(t, noGuard())
	for tool, args := range map[string]map[string]any{
		"delete_resource": {"id": "r1"},
		"delete_property": {"resource_id": "r1", "title": "T"},
		"update_resource": {"id": "r1", "is_hidden": true},
	} {
		wtMustFail(t, h, tool, args, "LK_MCP_GUARD_FILE")
	}
	if n := len(h.lk.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}

// Unhiding a resource checks its image and banner URLs too.
func TestRedteam_X_GUARD_05_UnhideChecksImageAndBanner(t *testing.T) {
	for name, field := range map[string]string{
		"image":  `"imageUrl":"https://example.com/zorblax.png"`,
		"banner": `"banner":{"enabled":true,"url":"https://example.com/zorblax.png","yPosition":0.5}`,
	} {
		t.Run(name, func(t *testing.T) {
			res := `{"id":"r1","name":"Ooo","isHidden":true,` + field + `,"documents":[{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false}]}`
			h := rgHarness(t, "Zorblax")
			h.lk.Enqueue(wtOK(res), wtOK(rgBlocks), wtOK(wtNoProps))
			wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "is_hidden": false}, "spoiler guard")
			if len(h.writes()) != 0 {
				t.Errorf("writes %+v", h.writes())
			}
		})
	}
}

// Two concurrent write tools must not interleave a guard decision with
// the other's write: update_resource unhides r1 after checking its old
// content while edit_page, which read r1 as hidden, writes a term to it.
// The write lock makes edit_page wait for the unhide, then see r1 visible
// and refuse.
func TestRedteam_X_GUARD_05_ConcurrentEditDuringUnhide(t *testing.T) {
	path := filepath.Join(t.TempDir(), "terms.txt")
	if err := os.WriteFile(path, []byte("Zorblax\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bg := &blockingGuard{g: guard.Open(path), entered: make(chan struct{}), released: make(chan struct{})}
	h := rgHarnessWith(t, bg)
	// update_resource: get r1 (hidden), its visible page, its properties.
	h.lk.Enqueue(wtOK(wtResHidden), wtOK(rgBlocks), wtOK(wtNoProps))
	done := make(chan *mcp.CallToolResult, 2)
	go func() {
		r, _ := h.cs.CallTool(context.Background(), callParams("update_resource", map[string]any{"id": "r1", "is_hidden": false}))
		done <- r
	}()
	<-bg.entered
	go func() {
		r, _ := h.cs.CallTool(context.Background(), callParams("edit_page", map[string]any{"resource_id": "r1", "base_version": "v7", "markdown": "Zorblax\n"}))
		done <- r
	}()
	// Give edit_page the chance to run ahead; with the lock it can't. (A
	// slow scheduler can only make this pass without the lock, never fail
	// with it.)
	time.Sleep(50 * time.Millisecond)
	if n := len(h.lk.Requests()); n != 3 {
		t.Errorf("edit_page ran while update_resource held its guard decision: %d requests", n)
	}
	// The unhide's PATCH, then edit_page's reads (r1 now visible).
	h.lk.Enqueue(wtOK(wtRes), wtOK(wtRes), wtOK(rgBlocks))
	close(bg.released)
	var refused bool
	for range 2 {
		r := <-done
		if r != nil && r.IsError && strings.Contains(text(r), "spoiler guard") {
			refused = true
		}
	}
	if !refused {
		t.Error("edit_page was not refused after the unhide")
	}
	for _, w := range h.writes() {
		if w.Method == "PUT" {
			t.Errorf("the term was written: %s %s", w.Method, w.Path)
		}
	}
}

// Showing a map checks its image URL and every feature; showing a
// timeline checks its lanes and events (X-GUARD-08). Hidden pins and events
// count as public until confirmed (X-GUARD-11).
func TestGuard_X_GUARD_08_ShowingMapsAndTimelinesScansThem(t *testing.T) {
	res := `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"m1","name":"Map","type":"map","rank":"a","isHidden":true},
{"id":"t1","name":"History","type":"time","rank":"b","isHidden":true}]}`
	mapDoc := `{"id":"m1","name":"Map","type":"map","rank":"a","isHidden":true,
"mapImage":{"assetId":"a1","url":"https://assets.legendkeeper.com/p1/ooo.png","width":512,"height":256,"bounds":[[-128,0],[0,256]]}}`
	cases := []struct {
		name, doc string
		replies   []string
		refuse    bool
	}{
		{"public pin", "m1", []string{res, mapDoc, `{"data":[{"id":"f1","type":"pin","name":"Zorblax Keep"}]}`}, true},
		{"X-GUARD-11 hidden pin still checked", "m1", []string{res, mapDoc, `{"data":[{"id":"f1","type":"pin","name":"Zorblax Keep","isHidden":true}]}`}, true},
		{"clean map", "m1", []string{res, mapDoc, `{"data":[{"id":"f1","type":"pin","name":"Candy Castle"}]}`}, false},
		{"lane", "t1", []string{res, `{"data":[{"id":"l1","name":"Zorblax","rank":"a0","size":"lg"}]}`, `{"data":[],"hasMore":false}`}, true},
		{"event", "t1", []string{res, `{"data":[{"id":"l1","name":"Kings","rank":"a0","size":"lg"}]}`,
			`{"data":[{"id":"e1","laneId":"l1","type":"event","start":0,"startDate":{"year":1,"month":1,"monthName":"Jan","day":1,"hour":0,"minute":0,"era":{"id":"ce","name":"CE","abbr":"CE"}},"name":"Zorblax crowned"}],"hasMore":false}`}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			for _, r := range c.replies {
				h.lk.Enqueue(wtOK(r))
			}
			args := map[string]any{"resource_id": "r1", "document_id": c.doc, "is_hidden": false, "dry_run": true}
			if c.refuse {
				wtMustFail(t, h, "update_document", args, "Zorblax")
			} else {
				wtMustOK(t, h, "update_document", args)
			}
			if len(h.writes()) != 0 {
				t.Errorf("writes %+v", h.writes())
			}
		})
	}
}
