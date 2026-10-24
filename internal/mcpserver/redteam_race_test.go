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

// A map, board or timeline document's content can't be read by the guard,
// so lk-mcp refuses to show one: unhiding the document, or unhiding a
// resource with such a document visible.
func TestRedteam_X_GUARD_05_UncheckableDocumentsStayHidden(t *testing.T) {
	hiddenMap := `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"m1","name":"Map","type":"map","rank":"c","isHidden":true}]}`
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(hiddenMap))
	wtMustFail(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "m1", "is_hidden": false}, "can't read")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}

	resHidden := `{"id":"r1","name":"Ooo","isHidden":true,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"m1","name":"Map","type":"map","rank":"c","isHidden":false}]}`
	h = rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(resHidden), wtOK(rgBlocks))
	wtMustFail(t, h, "update_resource", map[string]any{"id": "r1", "is_hidden": false, "dry_run": true}, "can't read")
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
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
