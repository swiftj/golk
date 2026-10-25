package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// Fixtures of the timeline tool tests. All names and terms are made up.
const (
	tlRes = `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"t1","name":"History","type":"time","rank":"a","isHidden":false},
{"id":"b1","name":"Blank","type":"blank","rank":"b","isHidden":false},
{"id":"p1","name":"Main","type":"page","rank":"c","isHidden":false},
{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true}]}`
	tlResHidden = `{"id":"r1","name":"Ooo","isHidden":true,"documents":[
{"id":"t1","name":"History","type":"time","rank":"a","isHidden":false}]}`
	tlDoc        = `{"id":"t1","name":"History","type":"time","rank":"a","isHidden":false,"calendarId":"cal1"}`
	tlDocChained = `{"id":"t1","name":"History","type":"time","rank":"a","isHidden":false,"calendarId":"cal1","sources":[{"resourceId":"r2","documentId":"t2"}]}`
	tlLane       = `{"id":"l1","name":"Jake","rank":"a1","size":"sm"}`
	tlLanes      = `{"data":[{"id":"l1","name":"Jake","rank":"a1","size":"sm"},{"id":"l0","name":"Finn","rank":"a0","size":"lg"},{"id":"x1","name":"Lich lane","rank":"a2","size":"sm"}]}`
	tlSrcLanes   = `{"data":[{"id":"x1","name":"Lich lane","rank":"a2","size":"sm"}]}`
	tlEvent      = `{"id":"e1","laneId":"l1","type":"event","name":"Tree Fort","start":540,"formattedStart":"1028 AD",
"startDate":{"year":1028,"month":1,"monthName":"January","day":1,"hour":0,"minute":0,"era":{"id":"ad","name":"Anno Domini","abbr":"AD"}},"uri":"lk://resources/r2"}`
	tlEvents = `{"data":[` + tlEvent + `,{"id":"e2","laneId":"x1","type":"event","name":"Lich wakes","start":600,"formattedStart":"1030 AD","isHidden":true,
"startDate":{"year":1030,"month":1,"monthName":"January","day":1,"hour":0,"minute":0,"era":{"id":"ad","name":"Anno Domini","abbr":"AD"}}}],"hasMore":true,"nextCursor":"c2"}`
	tlNoEvents = `{"data":[],"hasMore":false,"nextCursor":null}`
	tlDocPath  = "/api/v3/projects/p1/resources/r1/docs/t1"
)

// X-TL-01: lanes by rank, one page of events, chained lanes and events
// marked, next_cursor returned.
func TestReadTimeline_X_TL_01(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(tlRes), wtOK(tlDocChained), wtOK(tlLanes), wtOK(tlSrcLanes), wtOK(tlEvents))
	r := h.call("read_timeline", map[string]any{"resource_id": "r1", "limit": 2, "cursor": "c1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	reqs := h.lk.Requests()
	if len(reqs) != 5 {
		t.Fatalf("requests %+v", reqs)
	}
	wtReq(t, reqs[1], "GET", tlDocPath, "", "")
	wtReq(t, reqs[2], "GET", tlDocPath+"/lanes", "", "")
	wtReq(t, reqs[3], "GET", "/api/v3/projects/p1/resources/r2/docs/t2/lanes", "", "")
	wtReq(t, reqs[4], "GET", tlDocPath+"/events", "cursor=c1&limit=2", "")
	var out ReadTimelineOut
	structured(t, r, &out)
	if out.CalendarID != "cal1" || out.NextCursor != "c2" || !out.HasMore || len(out.Sources) != 1 {
		t.Errorf("out %+v", out)
	}
	if len(out.Lanes) != 3 || out.Lanes[0].ID != "l0" || out.Lanes[1].ID != "l1" || out.Lanes[2].ID != "x1" {
		t.Fatalf("lanes not by rank: %+v", out.Lanes)
	}
	if out.Lanes[1].Chained || !out.Lanes[2].Chained || out.Lanes[2].Source.DocumentID != "t2" {
		t.Errorf("chained marks: %+v", out.Lanes)
	}
	if len(out.Events) != 2 || out.Events[0].Chained || !out.Events[1].Chained || !out.Events[1].Hidden {
		t.Errorf("events %+v", out.Events)
	}
	txt := text(r)
	for _, want := range []string{"calendar cal1", `"Finn" (l0)`, "chained from r2/t2", `1028 AD: "Tree Fort" on "Jake" (e1) → lk://resources/r2`, "hidden, chained", `cursor "c2"`} {
		if !strings.Contains(txt, want) {
			t.Errorf("text lacks %q:\n%s", want, txt)
		}
	}
}

func TestReadTimeline_NotATimeline(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(tlRes))
	r := h.call("read_timeline", map[string]any{"resource_id": "r1", "document_id": "p1"})
	if !r.IsError || !strings.Contains(text(r), "not a timeline") {
		t.Fatalf("got %s", text(r))
	}
}

// X-TL-02: lane create and update send only the fields given; POST has an
// Idempotency-Key. Create on a blank document is allowed.
func TestSetTimelineLane_X_TL_02(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		h := newHarness(t)
		h.lk.Enqueue(wtOK(tlRes), testutil.JSON(http.StatusCreated, tlLane))
		r := h.call("set_timeline_lane", map[string]any{"resource_id": "r1", "document_id": "b1", "name": "Jake", "rank": "a1", "size": "sm"})
		if r.IsError {
			t.Fatal(text(r))
		}
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "POST", "/api/v3/projects/p1/resources/r1/docs/b1/lanes", "", `{"name":"Jake","rank":"a1","size":"sm"}`)
	})
	t.Run("update", func(t *testing.T) {
		h := newHarness(t)
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlLane), wtOK(tlLane))
		r := h.call("set_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1", "name": "Jake the Dog", "style": map[string]any{"color": "#ff0000"}})
		if r.IsError {
			t.Fatal(text(r))
		}
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "PATCH", tlDocPath+"/lanes/l1", "", `{"name":"Jake the Dog","style":{"color":"#ff0000"}}`)
	})
	t.Run("create needs name rank size", func(t *testing.T) {
		h := newHarness(t)
		r := h.call("set_timeline_lane", map[string]any{"resource_id": "r1", "name": "Jake"})
		if !r.IsError || len(h.lk.Requests()) != 0 {
			t.Fatalf("got %s", text(r))
		}
	})
	t.Run("bad size", func(t *testing.T) {
		h := newHarness(t)
		r := h.call("set_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1", "size": "xl"})
		if !r.IsError || len(h.lk.Requests()) != 0 {
			t.Fatalf("got %s", text(r))
		}
	})
	t.Run("create on a page is refused", func(t *testing.T) {
		h := newHarness(t)
		h.lk.Enqueue(wtOK(tlRes))
		r := h.call("set_timeline_lane", map[string]any{"resource_id": "r1", "document_id": "p1", "name": "Jake", "rank": "a1", "size": "sm"})
		if !r.IsError || len(h.writes()) != 0 {
			t.Fatalf("got %s", text(r))
		}
	})
}

// X-TOOL-04: dry runs of every timeline write tool send no write.
func TestTimelineDryRuns_X_TOOL_04(t *testing.T) {
	cases := []struct {
		tool    string
		args    map[string]any
		replies []string
		want    string
	}{
		{"set_timeline_lane", map[string]any{"resource_id": "r1", "name": "Jake", "rank": "a1", "size": "sm"}, []string{tlRes}, "would create lane"},
		{"set_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1", "name": "J"}, []string{tlRes, tlLane}, "would update lane"},
		{"delete_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1"}, []string{tlRes, tlLane, `{"data":[` + tlEvent + `,` + tlEvent + `],"hasMore":false,"nextCursor":null}`}, "its 2 event(s)"},
		{"set_timeline_event", map[string]any{"resource_id": "r1", "lane_id": "l1", "type": "event", "name": "Fort", "formatted_start": "1028"}, []string{tlRes}, "would create event"},
		{"set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "name": "Fort"}, []string{tlRes, tlEvent}, "would update event"},
		{"delete_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1"}, []string{tlRes, tlEvent}, "would delete event"},
	}
	for _, tc := range cases {
		t.Run(tc.tool+" "+tc.want, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			for _, b := range tc.replies {
				h.lk.Enqueue(wtOK(b))
			}
			tc.args["dry_run"] = true
			txt := wtMustOK(t, h, tc.tool, tc.args)
			if !strings.Contains(txt, tc.want) || !strings.Contains(txt, "Nothing was written") {
				t.Errorf("text %s", txt)
			}
			if len(h.writes()) != 0 {
				t.Errorf("dry run wrote %+v", h.writes())
			}
		})
	}
}

// delete_timeline_lane and delete_timeline_event send DELETE on the
// document's own lane or event.
func TestDeleteTimeline_X_TL_02(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(tlRes), wtOK(tlLane), wtOK(tlNoEvents), testutil.Reply{Status: http.StatusNoContent})
	wtMustOK(t, h, "delete_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1"})
	h.lk.Enqueue(wtOK(tlRes), wtOK(tlEvent), testutil.Reply{Status: http.StatusNoContent})
	wtMustOK(t, h, "delete_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1"})
	w := h.writes()
	if len(w) != 2 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "DELETE", tlDocPath+"/lanes/l1", "", "")
	wtReq(t, w[1], "DELETE", tlDocPath+"/events/e1", "", "")
}

// X-TL-02: event create and update send only the fields given.
func TestSetTimelineEvent_X_TL_02(t *testing.T) {
	t.Run("create formatted", func(t *testing.T) {
		h := newHarness(t)
		h.lk.Enqueue(wtOK(tlRes), testutil.JSON(http.StatusCreated, tlEvent))
		wtMustOK(t, h, "set_timeline_event", map[string]any{"resource_id": "r1", "lane_id": "l1", "type": "event", "name": "Tree Fort",
			"formatted_start": "1028", "formatted_end": "1033", "is_hidden": false, "uri": "lk://resources/r2"})
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "POST", tlDocPath+"/events", "",
			`{"laneId":"l1","type":"event","name":"Tree Fort","formattedStart":"1028","formattedEnd":"1033","isHidden":false,"uri":"lk://resources/r2"}`)
	})
	t.Run("update minutes", func(t *testing.T) {
		h := newHarness(t)
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlEvent), wtOK(tlEvent))
		wtMustOK(t, h, "set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "start": 100, "end": 200, "lane_id": "l0"})
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "PATCH", tlDocPath+"/events/e1", "", `{"laneId":"l0","start":100,"end":200}`)
	})
	for name, args := range map[string]map[string]any{
		"both date forms":   {"resource_id": "r1", "lane_id": "l1", "type": "event", "formatted_start": "1028", "end": 5},
		"create needs type": {"resource_id": "r1", "lane_id": "l1", "formatted_start": "1028"},
		"type on update":    {"resource_id": "r1", "event_id": "e1", "type": "note"},
		"bad type":          {"resource_id": "r1", "lane_id": "l1", "type": "party", "start": 1},
		"nothing to change": {"resource_id": "r1", "event_id": "e1"},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			r := h.call("set_timeline_event", args)
			if !r.IsError || len(h.lk.Requests()) != 0 {
				t.Fatalf("got %v %s, requests %d", r.IsError, text(r), len(h.lk.Requests()))
			}
		})
	}
}

// X-GUARD-08: lanes and events are checked as they will be, before the
// write; a refusal sends nothing. A hidden document or resource skips it.
func TestTimelineGuard_X_GUARD_08(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		args    map[string]any
		replies []string
	}{
		{"lane create name", "set_timeline_lane", map[string]any{"resource_id": "r1", "name": "Zorblax", "rank": "a1", "size": "sm"}, []string{tlRes}},
		{"lane style image", "set_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1", "style": map[string]any{"image_url": "https://x/zorblax.png"}}, []string{tlRes, tlLane}},
		{"lane update keeps old name", "set_timeline_lane", map[string]any{"resource_id": "r1", "lane_id": "l1", "rank": "a5"},
			[]string{tlRes, `{"id":"l1","name":"Zorblax lane","rank":"a1","size":"sm"}`}},
		{"event name", "set_timeline_event", map[string]any{"resource_id": "r1", "lane_id": "l1", "type": "event", "name": "Zorblax rises", "start": 1}, []string{tlRes}},
		{"event uri", "set_timeline_event", map[string]any{"resource_id": "r1", "lane_id": "l1", "type": "note", "uri": "https://zorblax.example", "start": 1}, []string{tlRes}},
		{"event image", "set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "image_url": "https://x/Zorblax.png"}, []string{tlRes, tlEvent}},
		{"event update keeps old name", "set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "start": 5},
			[]string{tlRes, strings.Replace(tlEvent, "Tree Fort", "Zorblax Fort", 1)}},
		{"event formatted date", "set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "formatted_start": "Zorblax 3rd, 1000"}, []string{tlRes, tlEvent}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			for _, b := range tc.replies {
				h.lk.Enqueue(wtOK(b))
			}
			r := h.call(tc.tool, tc.args)
			if !r.IsError {
				t.Fatalf("accepted: %s", text(r))
			}
			wtRefused(t, h, text(r))
		})
	}
	t.Run("hidden document skips", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(`{"data":[],"hasMore":false,"nextCursor":null}`), testutil.JSON(http.StatusCreated, `{"id":"e3","laneId":"l1","type":"event","name":"Zorblax","start":1,"startDate":{"era":{}}}`))
		wtMustOK(t, h, "set_timeline_event", map[string]any{"resource_id": "r1", "document_id": "t9", "lane_id": "l1", "type": "event", "name": "Zorblax", "start": 1})
		if len(h.writes()) != 1 {
			t.Errorf("writes %+v", h.writes())
		}
	})
	t.Run("hidden resource skips", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlResHidden), wtOK(`{"data":[],"hasMore":false,"nextCursor":null}`), testutil.JSON(http.StatusCreated, `{"id":"l2","name":"Zorblax","rank":"a1","size":"sm"}`))
		wtMustOK(t, h, "set_timeline_lane", map[string]any{"resource_id": "r1", "name": "Zorblax", "rank": "a1", "size": "sm"})
		if len(h.writes()) != 1 {
			t.Errorf("writes %+v", h.writes())
		}
	})
}

// X-GUARD-11: a hidden event is checked like a public one until hidden
// events are confirmed DM-only.
func TestTimelineGuard_X_GUARD_11_HiddenEventChecked(t *testing.T) {
	if hiddenFeaturesConfirmed {
		t.Skip("hidden events are confirmed DM-only")
	}
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(tlRes))
	r := h.call("set_timeline_event", map[string]any{"resource_id": "r1", "lane_id": "l1", "type": "event", "name": "Zorblax rises", "start": 1, "is_hidden": true})
	if !r.IsError {
		t.Fatalf("a hidden event with a protected term was accepted: %s", text(r))
	}
	wtRefused(t, h, text(r))
	// Hiding an existing event that holds a term doesn't exempt it either.
	h = rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(tlRes), wtOK(strings.Replace(tlEvent, "Tree Fort", "Zorblax Fort", 1)))
	r = h.call("set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "is_hidden": true})
	if !r.IsError {
		t.Fatalf("accepted: %s", text(r))
	}
	wtRefused(t, h, text(r))
}

// Without a terms file, timeline writes are not refused (D-017).
func TestTimelineWrites_NoTermsFile(t *testing.T) {
	h := newHarness(t, noGuard())
	for tool, args := range map[string]map[string]any{
		"set_timeline_lane":     {"resource_id": "r1", "name": "Jake", "rank": "a1", "size": "sm"},
		"delete_timeline_lane":  {"resource_id": "r1", "lane_id": "l1"},
		"set_timeline_event":    {"resource_id": "r1", "lane_id": "l1", "type": "event", "start": 1},
		"delete_timeline_event": {"resource_id": "r1", "event_id": "e1"},
	} {
		args["dry_run"] = true
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlLanes), wtOK(tlEvents), wtOK(tlEvent))
		if r := h.call(tool, args); r.IsError && strings.Contains(text(r), "spoiler guard") {
			t.Errorf("%s: %s", tool, text(r))
		}
	}
}

// update_document: calendar_id and sources (X-TL-02), the chain scan
// (X-GUARD-08), a hidden target skipping it.
func TestUpdateDocumentTimeline_X_TL_02(t *testing.T) {
	cals := `{"data":[{"id":"cal1","name":"Ooo time","version":"v1"},{"id":"gregorian","name":"Gregorian","version":"g1"}]}`
	t.Run("calendar to built-in", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlDoc), wtOK(cals), wtOK(tlDoc))
		wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "t1", "calendar_id": "gregorian"})
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "PATCH", tlDocPath, "", `{"calendarId":"gregorian"}`)
	})
	t.Run("custom calendar with a term is refused", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(`{"id":"t1","name":"History","type":"time","rank":"a","isHidden":false,"calendarId":"gregorian"}`),
			wtOK(`{"data":[{"id":"cal1","name":"Ooo time","version":"v1","months":[{"id":"m","name":"Zorblaxmonth"},{"id":"z","name":"Month of Zorblax"}]}]}`))
		r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "t1", "calendar_id": "cal1"})
		wtRefused(t, h, text(r))
	})
	t.Run("unknown calendar", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlDoc), wtOK(cals))
		r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "t1", "calendar_id": "nope"})
		if !r.IsError || len(h.writes()) != 0 {
			t.Fatalf("got %s", text(r))
		}
	})
	t.Run("sources scanned then set", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlDoc), wtOK(tlSrcLanes), wtOK(tlNoEvents), wtOK(tlDocChained))
		wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "t1",
			"sources": []map[string]string{{"resource_id": "r2", "document_id": "t2"}}})
		reqs := h.lk.Requests()
		wtReq(t, reqs[2], "GET", "/api/v3/projects/p1/resources/r2/docs/t2/lanes", "", "")
		wtReq(t, reqs[3], "GET", "/api/v3/projects/p1/resources/r2/docs/t2/events", "limit=100", "")
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "PATCH", tlDocPath, "", `{"sources":[{"resourceId":"r2","documentId":"t2"}]}`)
	})
	t.Run("sources with a term are refused", func(t *testing.T) {
		for name, replies := range map[string][]string{
			"lane":         {strings.Replace(tlSrcLanes, "Lich lane", "Zorblax lane", 1), tlNoEvents},
			"hidden event": {tlSrcLanes, `{"data":[` + strings.Replace(tlEvent, `"Tree Fort"`, `"Zorblax","isHidden":true`, 1) + `],"hasMore":false}`},
		} {
			t.Run(name, func(t *testing.T) {
				h := rgHarness(t, "Zorblax")
				h.lk.Enqueue(wtOK(tlRes), wtOK(tlDoc))
				for _, b := range replies {
					h.lk.Enqueue(wtOK(b))
				}
				r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "t1",
					"sources": []map[string]string{{"resource_id": "r2", "document_id": "t2"}}})
				wtRefused(t, h, text(r))
			})
		}
	})
	t.Run("hidden unchained target skips the scan", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(`{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true}`), wtOK(`{"data":[],"hasMore":false,"nextCursor":null}`), wtOK(`{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true}`))
		wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "t9",
			"sources": []map[string]string{{"resource_id": "r2", "document_id": "t2"}}})
		if n := len(h.lk.Requests()); n != 4 {
			t.Errorf("want resource, document, chain scan, update; got %d requests", n)
		}
	})
	t.Run("clear sources", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlDocChained), wtOK(tlDoc))
		wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "t1", "sources": []any{}})
		w := h.writes()
		if len(w) != 1 {
			t.Fatalf("writes %+v", w)
		}
		wtReq(t, w[0], "PATCH", tlDocPath, "", `{"sources":[]}`)
	})
	t.Run("dry run", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(tlDoc), wtOK(tlSrcLanes), wtOK(tlNoEvents))
		txt := wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "t1", "dry_run": true,
			"sources": []map[string]string{{"resource_id": "r2", "document_id": "t2"}}})
		if len(h.writes()) != 0 || !strings.Contains(txt, "chain 1 timeline") {
			t.Errorf("%s / %+v", txt, h.writes())
		}
	})
	t.Run("needs a time document", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes))
		r := h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "p1", "calendar_id": "gregorian"})
		if !r.IsError || len(h.lk.Requests()) != 1 {
			t.Fatalf("got %s", text(r))
		}
	})
}
