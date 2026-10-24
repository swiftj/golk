//go:build integration

package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// cleanupMCPCalendar deletes calendar id when the test ends, with the
// version it has then: the sandbox sweep does not know calendars.
func cleanupMCPCalendar(t *testing.T, c *lk.Client, id string) {
	t.Helper()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		for cal, err := range c.Calendars.All(ctx) {
			if err != nil {
				t.Errorf("cleanup: listing calendars: %v", err)
				return
			}
			if cal.ID == id {
				if err := c.Calendars.Delete(ctx, id, lk.IfMatch(cal.Version)); err != nil && !errors.Is(err, lk.ErrNotFound) {
					t.Errorf("cleanup: deleting calendar %s: %v", id, err)
				}
				return
			}
		}
	})
}

// mustRefuseCalendar asserts that create_calendar refuses spec; should it
// create one after all, it is deleted when the test ends.
func mustRefuseCalendar(t *testing.T, cs *mcp.ClientSession, c *lk.Client, spec map[string]any) {
	t.Helper()
	r := call(t, cs, "create_calendar", map[string]any{"spec": spec})
	if id, _ := r.out["id"].(string); id != "" {
		cleanupMCPCalendar(t, c, id)
	}
	if !r.err || !strings.Contains(r.text, "spoiler guard") {
		t.Fatalf("create_calendar: want a guard refusal, got %v: %s", r.err, r.text)
	}
}

func outString(t *testing.T, r result, key string) string {
	t.Helper()
	v, _ := r.out[key].(string)
	if v == "" {
		t.Fatalf("no %s in %v (%s)", key, r.out, r.text)
	}
	return v
}

// firstDocID is the id of the first document create_resource reported.
func firstDocID(t *testing.T, r result) string {
	t.Helper()
	docs, _ := r.out["documents"].([]any)
	if len(docs) == 0 {
		t.Fatalf("no documents in %v", r.out)
	}
	d, _ := docs[0].(map[string]any)
	id, _ := d["document_id"].(string)
	return id
}

// TestIntegration_MCPTimelinesAndCalendars drives the timeline and
// calendar tools against the sandbox through MCP, with the real guard
// (X-TL-01/02, X-CAL-01, X-GUARD-08/09/11).
func TestIntegration_MCPTimelinesAndCalendars(t *testing.T) {
	cs, c := session(t), sandbox.Client(t)

	// A custom calendar: a copy of the Gregorian one under a new name.
	g := mustOK(t, cs, "get_calendar", map[string]any{"calendar_id": "gregorian"})
	_, specJSON, ok := strings.Cut(g.text, "Spec:\n")
	if !ok {
		t.Fatalf("get_calendar text: %s", g.text)
	}
	var spec map[string]any
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		t.Fatal(err)
	}
	delete(spec, "id")
	spec["name"] = sandbox.Name("Ooo Reckoning")

	// X-GUARD-09: a protected term in a month name is refused.
	var bad map[string]any
	_ = json.Unmarshal([]byte(specJSON), &bad)
	delete(bad, "id")
	bad["name"] = sandbox.Name("Bad Reckoning")
	bad["months"].([]any)[0].(map[string]any)["name"] = "Zorblax"
	mustRefuseCalendar(t, cs, c, bad)
	bad["months"].([]any)[0].(map[string]any)["name"] = "Month of Zorblax"
	mustRefuseCalendar(t, cs, c, bad)

	cc := mustOK(t, cs, "create_calendar", map[string]any{"spec": spec})
	calID, v1 := outString(t, cc, "id"), outString(t, cc, "version")
	cleanupMCPCalendar(t, c, calID)
	lc := mustOK(t, cs, "list_calendars", map[string]any{})
	if !strings.Contains(lc.text, "("+calID+") custom") || !strings.Contains(lc.text, "(gregorian) built-in") {
		t.Errorf("list_calendars:\n%s", lc.text)
	}

	// X-CAL-01: replace with the version, then a stale replace is refused;
	// built-ins are refused.
	spec["name"] = sandbox.Name("Ooo Reckoning II")
	rc := mustOK(t, cs, "replace_calendar", map[string]any{"calendar_id": calID, "version": v1, "spec": spec})
	if v2 := outString(t, rc, "version"); v2 == v1 {
		t.Errorf("replace kept version %s", v1)
	}
	mustRefuse(t, cs, "replace_calendar", map[string]any{"calendar_id": calID, "version": v1, "spec": spec}, "VERSION_MISMATCH")
	mustRefuse(t, cs, "replace_calendar", map[string]any{"calendar_id": "harptos", "version": "x", "spec": spec}, "built-in")
	mustRefuse(t, cs, "delete_calendar", map[string]any{"calendar_id": "gregorian", "version": "x"}, "built-in")

	// A timeline on that calendar: the first lane turns the blank document
	// into a timeline.
	cr := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Ooo History")})
	rid := outString(t, cr, "resource_id")
	sandbox.Cleanup(t, rid)
	tid := firstDocID(t, cr)
	ln := mustOK(t, cs, "set_timeline_lane", map[string]any{"resource_id": rid, "document_id": tid, "name": "Finn", "rank": "a0", "size": "lg"})
	laneID := outString(t, ln, "lane_id")
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": tid, "calendar_id": calID})
	mustOK(t, cs, "set_timeline_event", map[string]any{"resource_id": rid, "document_id": tid, "lane_id": laneID, "type": "event",
		"name": "Finn is found", "formatted_start": "1016"})
	mustOK(t, cs, "set_timeline_event", map[string]any{"resource_id": rid, "document_id": tid, "lane_id": laneID, "type": "event",
		"name": "Tree Fort years", "formatted_start": "1028", "formatted_end": "1033"})

	// X-GUARD-08/11: a term in a public event name is refused, and so is
	// the same event hidden.
	ev := map[string]any{"resource_id": rid, "document_id": tid, "lane_id": laneID, "type": "event", "name": "Zorblax rises", "formatted_start": "1030"}
	mustRefuse(t, cs, "set_timeline_event", ev, "spoiler guard")
	ev["is_hidden"] = true
	mustRefuse(t, cs, "set_timeline_event", ev, "spoiler guard")
	mustRefuse(t, cs, "set_timeline_lane", map[string]any{"resource_id": rid, "document_id": tid, "name": "Zorblax", "rank": "a1", "size": "sm"}, "spoiler guard")

	rt := mustOK(t, cs, "read_timeline", map[string]any{"resource_id": rid})
	if rt.out["calendar_id"] != calID || !strings.Contains(rt.text, `"Finn is found"`) || !strings.Contains(rt.text, `"Tree Fort years"`) ||
		strings.Contains(rt.text, "Zorblax") {
		t.Errorf("read_timeline %v:\n%s", rt.out["calendar_id"], rt.text)
	}
	if evs, _ := rt.out["events"].([]any); len(evs) != 2 {
		t.Errorf("events %v", rt.out["events"])
	}
	t.Log(rt.text)

	// X-GUARD-08: chaining a timeline whose lane holds a term is refused,
	// even though that source is hidden (what the chain shows is checked).
	src := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Lich lore"), "is_hidden": true})
	srcID := outString(t, src, "resource_id")
	sandbox.Cleanup(t, srcID)
	srcDoc := firstDocID(t, src)
	mustOK(t, cs, "set_timeline_lane", map[string]any{"resource_id": srcID, "document_id": srcDoc, "name": "Zorblax lane", "rank": "a0", "size": "sm"})
	mustRefuse(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": tid,
		"sources": []map[string]string{{"resource_id": srcID, "document_id": srcDoc}}}, "spoiler guard")

	// A clean chain is set, and read_timeline marks its lane as chained.
	clean := mustOK(t, cs, "create_resource", map[string]any{"name": sandbox.Name("Marceline lore")})
	cleanID := outString(t, clean, "resource_id")
	sandbox.Cleanup(t, cleanID)
	cleanDoc := firstDocID(t, clean)
	mustOK(t, cs, "set_timeline_lane", map[string]any{"resource_id": cleanID, "document_id": cleanDoc, "name": "Marceline", "rank": "a0", "size": "sm"})
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": tid,
		"sources": []map[string]string{{"resource_id": cleanID, "document_id": cleanDoc}}})
	rt = mustOK(t, cs, "read_timeline", map[string]any{"resource_id": rid})
	if !strings.Contains(rt.text, `"Marceline"`) || !strings.Contains(rt.text, "chained from "+cleanID) {
		t.Errorf("chained read_timeline:\n%s", rt.text)
	}
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": tid, "sources": []any{}})

	// Dry-run lane delete counts its events; the real one deletes them.
	dl := mustOK(t, cs, "delete_timeline_lane", map[string]any{"resource_id": rid, "document_id": tid, "lane_id": laneID, "dry_run": true})
	if dl.out["events"] != float64(2) {
		t.Errorf("dry-run lane delete %v", dl.out)
	}
	mustOK(t, cs, "delete_timeline_lane", map[string]any{"resource_id": rid, "document_id": tid, "lane_id": laneID})

	// The calendar can be deleted once with its current version.
	gc := mustOK(t, cs, "get_calendar", map[string]any{"calendar_id": calID})
	mustOK(t, cs, "update_document", map[string]any{"resource_id": rid, "document_id": tid, "calendar_id": "gregorian"})
	mustOK(t, cs, "delete_calendar", map[string]any{"calendar_id": calID, "version": outString(t, gc, "version")})
}
