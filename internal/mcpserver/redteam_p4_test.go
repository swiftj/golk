package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// Phase 4 red-team regressions. All names and terms are made up.

const (
	// rtP4ResList lists r1 (tlRes: visible t1, hidden t9) and r5, a visible
	// resource with a visible timeline v1.
	rtP4ResList = `{"data":[
{"id":"r1","name":"Ooo","isHidden":false,"documents":[{"id":"t1","name":"History","type":"time"},{"id":"t9","name":"DM History","type":"time"}]},
{"id":"r5","name":"Candy Kingdom","isHidden":false,"documents":[{"id":"v1","name":"Public History","type":"time"}]},
{"id":"r6","name":"Nothing here","isHidden":false,"documents":[{"id":"p6","name":"Main","type":"page"}]}],"hasMore":false,"nextCursor":null}`
	rtP4R1Docs = `{"data":[
{"id":"t1","name":"History","type":"time","rank":"a","isHidden":false},
{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true}]}`
	// v1 chains t9 directly.
	rtP4R5ChainsT9 = `{"data":[{"id":"v1","name":"Public History","type":"time","rank":"a","isHidden":false,"sources":[{"resourceId":"r1","documentId":"t9"}]}]}`
	// v1 chains t1, which chains t9: a transitive chain.
	rtP4R1DocsT1ChainsT9 = `{"data":[
{"id":"t1","name":"History","type":"time","rank":"a","isHidden":true,"sources":[{"resourceId":"r1","documentId":"t9"}]},
{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true}]}`
	rtP4R5ChainsT1 = `{"data":[{"id":"v1","name":"Public History","type":"time","rank":"a","isHidden":false,"sources":[{"resourceId":"r1","documentId":"t1"}]}]}`
	rtP4R5NoChain  = `{"data":[{"id":"v1","name":"Public History","type":"time","rank":"a","isHidden":false}]}`
)

// X-GUARD-08: a hidden timeline that a visible timeline chains shows its
// lanes and events to players through that chain (D-014 checks a chain's
// hidden sources for exactly this reason), so writing to it is checked.
// Before the fix, t.hidden() skipped the guard and the term went out.
func TestRedteamP4_X_GUARD_08_HiddenChainedSourceIsChecked(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		args    map[string]any
		replies []string
	}{
		{"event on directly chained source", "set_timeline_event",
			map[string]any{"resource_id": "r1", "document_id": "t9", "lane_id": "l1", "type": "event", "name": "Zorblax rises", "start": 1},
			[]string{tlRes, rtP4ResList, rtP4R1Docs, rtP4R5ChainsT9}},
		{"lane on directly chained source", "set_timeline_lane",
			map[string]any{"resource_id": "r1", "document_id": "t9", "name": "Zorblax", "rank": "a1", "size": "sm"},
			[]string{tlRes, rtP4ResList, rtP4R1Docs, rtP4R5ChainsT9}},
		{"event on transitively chained source", "set_timeline_event",
			map[string]any{"resource_id": "r1", "document_id": "t9", "lane_id": "l1", "type": "event", "name": "Zorblax rises", "start": 1},
			[]string{tlRes, rtP4ResList, rtP4R1DocsT1ChainsT9, rtP4R5ChainsT1}},
		{"sources of a chained hidden timeline", "update_document",
			map[string]any{"resource_id": "r1", "document_id": "t9", "sources": []map[string]string{{"resource_id": "r2", "document_id": "t2"}}},
			[]string{tlRes, `{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true}`, rtP4ResList, rtP4R1Docs, rtP4R5ChainsT9,
				`{"data":[{"id":"x1","name":"Zorblax lane","rank":"a2","size":"sm"}]}`, tlNoEvents}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, dry := range []bool{false, true} {
				h := rgHarness(t, "Zorblax")
				for _, b := range tc.replies {
					h.lk.Enqueue(wtOK(b))
				}
				args := map[string]any{"dry_run": dry}
				for k, v := range tc.args {
					args[k] = v
				}
				r := h.call(tc.tool, args)
				if !r.IsError {
					t.Fatalf("dry_run=%v accepted: %s", dry, text(r))
				}
				wtRefused(t, h, text(r))
			}
		})
	}
	t.Run("unchained hidden source still skips", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(rtP4ResList), wtOK(rtP4R1Docs), wtOK(rtP4R5NoChain))
		out := wtMustOK(t, h, "set_timeline_event", map[string]any{"resource_id": "r1", "document_id": "t9", "lane_id": "l1",
			"type": "event", "name": "Zorblax rises", "start": 1, "dry_run": true})
		if !strings.Contains(out, "Dry run") || len(h.writes()) != 0 {
			t.Errorf("got %s, writes %+v", out, h.writes())
		}
	})
	t.Run("a failed scan refuses", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), testutil.JSON(http.StatusInternalServerError, `{"message":"boom","code":"INTERNAL","requestId":"rq1"}`))
		r := h.call("set_timeline_event", map[string]any{"resource_id": "r1", "document_id": "t9", "lane_id": "l1",
			"type": "event", "name": "Zorblax rises", "start": 1})
		if !r.IsError || len(h.writes()) != 0 {
			t.Fatalf("got %s, writes %+v", text(r), h.writes())
		}
	})
}

// X-GUARD-08: showing a timeline shows its custom calendar's month,
// weekday and era names. Before the fix the unhide checked lanes and
// events only, and a term in the calendar went out unchecked.
func TestRedteamP4_X_GUARD_08_UnhideTimelineChecksItsCalendar(t *testing.T) {
	res := `{"id":"r1","name":"Ooo","isHidden":false,"documents":[{"id":"t9","name":"DM History","type":"time","rank":"d","isHidden":true,"calendarId":"cal9"}]}`
	cals := `{"data":[{"id":"cal9","version":"W/\"c9\"","name":"Ooo Reckoning","hasZeroYear":false,"maxMinutes":1,` +
		`"months":[{"id":"m1","name":"Zorblaxtide","isIntercalary":false,"length":30},{"id":"m2","name":"Month of Zorblax","isIntercalary":false,"length":30}],` +
		`"leapDays":[],"weekdays":[],"epochWeekday":0,"hoursInDay":24,"minutesInHour":60,"positiveEras":[]}]}`
	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{"update_document", "update_document", map[string]any{"resource_id": "r1", "document_id": "t9", "is_hidden": false}},
		{"update_document dry run", "update_document", map[string]any{"resource_id": "r1", "document_id": "t9", "is_hidden": false, "dry_run": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			h.lk.Enqueue(wtOK(res), wtOK(`{"data":[]}`), wtOK(tlNoEvents), wtOK(cals))
			r := h.call(tc.tool, tc.args)
			if !r.IsError {
				t.Fatalf("accepted: %s", text(r))
			}
			wtRefused(t, h, text(r))
			if !strings.Contains(text(r), "calendar") {
				t.Errorf("violation should point at the calendar: %s", text(r))
			}
		})
	}
	t.Run("built-in calendar reads nothing more", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(strings.Replace(res, "cal9", "harptos", 1)), wtOK(`{"data":[]}`), wtOK(tlNoEvents))
		wtMustOK(t, h, "update_document", map[string]any{"resource_id": "r1", "document_id": "t9", "is_hidden": false, "dry_run": true})
		if n := len(h.lk.Requests()); n != 3 {
			t.Errorf("requests %d", n)
		}
	})
}

// X-GUARD-09: a calendar spec is user JSON whose unmodelled fields are
// kept. Checked as the guard's root value, a top-level "kind":"secret" (or
// a property kind with "isHidden":true) made the whole spec DM-only, so a
// dry run passed a spec full of terms. The spec is now checked under a
// key, where no exemption applies.
func TestRedteamP4_X_GUARD_09_SpecRootCantExemptItself(t *testing.T) {
	for name, extra := range map[string]map[string]any{
		"secret":          {"kind": "secret"},
		"hidden property": {"kind": "textField", "isHidden": true},
	} {
		t.Run(name, func(t *testing.T) {
			spec := map[string]any{"id": "zz", "name": "Zorblax Reckoning", "hasZeroYear": false, "maxMinutes": 1, "months": []any{},
				"leapDays": []any{}, "weekdays": []any{}, "epochWeekday": 0, "hoursInDay": 24, "minutesInHour": 60, "positiveEras": []any{}}
			for k, v := range extra {
				spec[k] = v
			}
			h := rgHarness(t, "Zorblax")
			r := h.call("create_calendar", map[string]any{"spec": spec, "dry_run": true})
			if !r.IsError {
				t.Fatalf("accepted: %s", text(r))
			}
			wtRefused(t, h, text(r))
		})
	}
}

// X-GUARD-10: without a name, a copy is named after the template less the
// word "Template". If the server drops it inside a word, the copy of
// "ZorblaxTemplate" is "Zorblax", which whole-word matching on the
// template's name missed.
func TestRedteamP4_X_GUARD_10_DefaultCopyNameIsChecked(t *testing.T) {
	for _, name := range []string{"ZorblaxTemplate", "Zorblaxtemplate Keep", "ZorTEMPLATEblax", "TemplateZorblax"} {
		t.Run(name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			h.lk.Enqueue(wtOK(`{"id":"tp1","name":"` + name + `","parentId":"templates","isHidden":false,"documents":[]}`))
			r := h.call("create_from_template", map[string]any{"template_id": "tp1", "dry_run": true})
			if !r.IsError {
				t.Fatalf("accepted: %s", text(r))
			}
			wtRefused(t, h, text(r))
		})
	}
	t.Run("a given name is what is checked", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(`{"id":"tp1","name":"ZorblaxTemplate","parentId":"templates","isHidden":true,"documents":[]}`))
		wtMustOK(t, h, "create_from_template", map[string]any{"template_id": "tp1", "name": "Realm", "dry_run": true})
	})
}

// Attacks that held, kept as regressions.

// X-RO-01: in read-only mode the Phase 4 read tools send only GETs.
func TestRedteamP4_X_RO_01_Phase4ReadToolsOnlyGET(t *testing.T) {
	for _, tc := range []struct {
		tool    string
		args    map[string]any
		replies []string
	}{
		{"read_map", map[string]any{"resource_id": "r1"}, []string{mtRes, mtDoc, mtFeatures}},
		{"read_timeline", map[string]any{"resource_id": "r1"}, []string{tlRes, tlDoc, `{"data":[]}`, tlNoEvents}},
		{"list_calendars", map[string]any{}, []string{calList}},
		{"get_calendar", map[string]any{"calendar_id": "cal1"}, []string{calList}},
		{"list_templates", map[string]any{}, []string{`{"data":[],"hasMore":false,"nextCursor":null}`}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			h := newHarness(t, readOnlyMode())
			for _, b := range tc.replies {
				h.lk.Enqueue(wtOK(b))
			}
			if r := h.call(tc.tool, tc.args); r.IsError {
				t.Fatal(text(r))
			}
			for _, rq := range h.lk.Requests() {
				if rq.Method != http.MethodGet {
					t.Errorf("%s sent %s %s", tc.tool, rq.Method, rq.Path)
				}
			}
		})
	}
}

// X-GUARD-08: every player-visible field set_map_feature takes is checked,
// style fields and the pin image URL included.
func TestRedteamP4_X_GUARD_08_EveryFeatureFieldIsChecked(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"pin image url":     {"style": map[string]any{"pin_image": map[string]any{"url": "https://assets.example/zorblax.png", "width": 1, "height": 1}}},
		"text effect color": {"style": map[string]any{"text_effect_color": "Zorblax"}},
		"icon glyph":        {"style": map[string]any{"icon_glyph": "zorblax"}},
		"uri":               {"uri": "lk://resources/zorblax"},
		"label text":        {"name": "the zorblax"},
	} {
		t.Run(name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			h.lk.Enqueue(wtOK(mtRes))
			a := map[string]any{"resource_id": "r1", "type": "pin", "pos": []float64{1, 2}}
			for k, v := range args {
				a[k] = v
			}
			wtRefused(t, h, text(h.call("set_map_feature", a)))
		})
	}
}

// X-GUARD-08, X-GUARD-11: showing a map shows its hidden features too
// (until confirmed DM-only), whether the document or its resource is
// unhidden; and showing a timeline reads every page of its events.
func TestRedteamP4_X_GUARD_08_ShowingChecksEverything(t *testing.T) {
	if hiddenFeaturesConfirmed {
		t.Skip("hidden features are DM-only")
	}
	hiddenTerm := `{"data":[{"id":"f2","type":"pin","name":"Zorblax Cave","pos":[-1,2],"isHidden":true}],"hasMore":false}`
	t.Run("unhide map document", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(mtRes), wtOK(strings.Replace(mtDoc, `"m1"`, `"m2"`, 1)), wtOK(hiddenTerm))
		wtRefused(t, h, text(h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "m2", "is_hidden": false})))
	})
	t.Run("unhide resource holding a map", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(mtResHidden), wtOK(mtDoc), wtOK(hiddenTerm), wtOK(`{"data":[]}`))
		wtRefused(t, h, text(h.call("update_resource", map[string]any{"id": "r1", "is_hidden": false})))
	})
	t.Run("unhide timeline, term on the second events page", func(t *testing.T) {
		h := rgHarness(t, "Zorblax")
		h.lk.Enqueue(wtOK(tlRes), wtOK(`{"data":[]}`),
			wtOK(`{"data":[`+tlEvent+`],"hasMore":true,"nextCursor":"c2"}`),
			wtOK(`{"data":[{"id":"e5","laneId":"l1","type":"event","name":"Zorblax wakes","start":900,"isHidden":true,"startDate":{"era":{}}}],"hasMore":false,"nextCursor":null}`))
		wtRefused(t, h, text(h.call("update_document", map[string]any{"resource_id": "r1", "document_id": "t9", "is_hidden": false})))
		if reqs := h.lk.Requests(); len(reqs) != 4 || !strings.Contains(reqs[3].RawQuery, "cursor=c2") {
			t.Errorf("second page not read: %+v", reqs)
		}
	})
}
