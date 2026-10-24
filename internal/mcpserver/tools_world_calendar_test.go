package mcpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// A made-up calendar spec, complete so that the body sent equals it.
const calSpec = `{"name":"Ooo Reckoning","description":"Candy months.","hasZeroYear":false,"maxMinutes":1800000000,` +
	`"months":[{"id":"candy","name":"Candy","length":30,"interval":1,"offset":0,"isIntercalary":false}],` +
	`"leapDays":[{"id":"gunter","name":"Gunter Day","month":1,"interval":"4","offset":0,"day":1,"addsWeekDay":false,"intercalary":false,"weekDay":""}],` +
	`"weekdays":[{"id":"finn","name":"Finnsday"}],"epochWeekday":0,"hoursInDay":24,"minutesInHour":60,` +
	`"positiveEras":[{"id":"after","name":"After the War","abbr":"AW","startsAt":0,"resetMode":"none"}],` +
	`"moons":[{"id":"moon","name":"Glob","phase":28,"shift":0}],` +
	`"format":{"id":"f","year":"YYYY","month":"MMMM","day":"D","time":"HH:mm"},"futureField":{"label":"kept"}}`

const calList = `{"data":[` +
	`{"id":"cal1","version":"W/\"cv1\"",` + `"name":"Ooo Reckoning","hasZeroYear":false,"maxMinutes":1,"months":[],"leapDays":[],"weekdays":[],"epochWeekday":0,"hoursInDay":24,"minutesInHour":60,"positiveEras":[]},` +
	`{"id":"gregorian","version":"g1","name":"Gregorian","hasZeroYear":false,"maxMinutes":1,"months":[],"leapDays":[],"weekdays":[],"epochWeekday":0,"hoursInDay":24,"minutesInHour":60,"positiveEras":[]}]}`

const calPath = "/api/v3/projects/p1/calendars"

func calArgs(t *testing.T, spec string, kv ...any) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(spec), &m); err != nil {
		t.Fatal(err)
	}
	args := map[string]any{"spec": m}
	for i := 0; i+1 < len(kv); i += 2 {
		args[kv[i].(string)] = kv[i+1]
	}
	return args
}

func withVersion(r testutil.Reply, etag string) testutil.Reply {
	r.Header["ETag"] = etag
	return r
}

// X-CAL-01: list and get (custom vs built-in, versions, spec as JSON).
func TestCalendarReads_X_CAL_01(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(calList), wtOK(calList))
	r := h.call("list_calendars", map[string]any{})
	if r.IsError {
		t.Fatal(text(r))
	}
	var out ListCalendarsOut
	structured(t, r, &out)
	if len(out.Calendars) != 2 || out.Calendars[0].BuiltIn || !out.Calendars[1].BuiltIn || out.Calendars[0].Version != `W/"cv1"` {
		t.Errorf("out %+v", out)
	}
	if !strings.Contains(text(r), `"Ooo Reckoning" (cal1) custom`) || !strings.Contains(text(r), "(gregorian) built-in") {
		t.Errorf("text %s", text(r))
	}
	g := h.call("get_calendar", map[string]any{"calendar_id": "cal1"})
	if g.IsError || !strings.Contains(text(g), `"minutesInHour": 60`) || !strings.Contains(text(g), `version W/"cv1"`) {
		t.Errorf("get %s", text(g))
	}
	wtReq(t, h.lk.Requests()[0], "GET", calPath, "", "")
}

// X-CAL-01: create sends the spec (lossless) with an Idempotency-Key.
func TestCreateCalendar_X_CAL_01(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(withVersion(testutil.JSON(http.StatusCreated, `{"id":"cal2","version":"W/\"cv1\"",`+calSpec[1:]), `W/"cv1"`))
	txt := wtMustOK(t, h, "create_calendar", calArgs(t, calSpec))
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "POST", calPath, "", calSpec)
	if !strings.Contains(txt, "id cal2") {
		t.Errorf("text %s", txt)
	}
}

// X-CAL-01: replace sends the spec with If-Match; delete sends If-Match.
func TestReplaceDeleteCalendar_X_CAL_01(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(withVersion(wtOK(`{"id":"cal1","version":"W/\"cv2\"",`+calSpec[1:]), `W/"cv2"`), testutil.Reply{Status: http.StatusNoContent})
	txt := wtMustOK(t, h, "replace_calendar", calArgs(t, calSpec, "calendar_id", "cal1", "version", `W/"cv1"`))
	wtMustOK(t, h, "delete_calendar", map[string]any{"calendar_id": "cal1", "version": `W/"cv2"`})
	w := h.writes()
	if len(w) != 2 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], "PUT", calPath+"/cal1", "", calSpec)
	wtReq(t, w[1], "DELETE", calPath+"/cal1", "", "")
	if got := w[0].Header.Get("If-Match"); got != `W/"cv1"` {
		t.Errorf("replace If-Match %q", got)
	}
	if got := w[1].Header.Get("If-Match"); got != `W/"cv2"` {
		t.Errorf("delete If-Match %q", got)
	}
	if !strings.Contains(txt, `new version W/"cv2"`) {
		t.Errorf("text %s", txt)
	}
}

// A stale version comes back as a calendar hint, not a page hint.
func TestReplaceCalendar_Stale(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(testutil.JSON(http.StatusPreconditionFailed, `{"message":"Calendar version is W/\"cv2\", If-Match was W/\"cv1\"","code":"VERSION_MISMATCH","requestId":"rq1"}`))
	r := h.call("replace_calendar", calArgs(t, calSpec, "calendar_id", "cal1", "version", `W/"cv1"`))
	if !r.IsError || !strings.Contains(text(r), "VERSION_MISMATCH") || !strings.Contains(text(r), "get_calendar") || strings.Contains(text(r), "read_page") {
		t.Errorf("got %s", text(r))
	}
}

// X-CAL-01: a built-in calendar id is refused before any request.
func TestCalendarBuiltinRefused_X_CAL_01(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	for tool, args := range map[string]map[string]any{
		"replace_calendar": calArgs(t, calSpec, "calendar_id", "harptos", "version", "v1"),
		"delete_calendar":  {"calendar_id": "gregorian", "version": "v1"},
		"create_calendar":  calArgs(t, `{"id":"eberron",`+calSpec[1:]),
	} {
		r := h.call(tool, args)
		if !r.IsError || !strings.Contains(text(r), "built-in") {
			t.Errorf("%s: %s", tool, text(r))
		}
	}
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}

// X-GUARD-09: every string of a calendar spec is checked before the
// write, unmodelled fields included; a refusal sends nothing.
func TestCalendarGuard_X_GUARD_09(t *testing.T) {
	for name, spec := range map[string]string{
		"name":        strings.Replace(calSpec, "Ooo Reckoning", "Zorblax Reckoning", 1),
		"description": strings.Replace(calSpec, "Candy months.", "Months of Zorblax.", 1),
		"month":       strings.Replace(calSpec, `"name":"Candy"`, `"name":"Zorblax"`, 1),
		"weekday":     strings.Replace(calSpec, "Finnsday", "Zorblaxday zorblax", 1),
		"leap day":    strings.Replace(calSpec, "Gunter Day", "Zorblax Day", 1),
		"era":         strings.Replace(calSpec, "After the War", "Age of Zorblax", 1),
		"era abbr":    strings.Replace(calSpec, `"AW"`, `"Zorblax"`, 1),
		"moon":        strings.Replace(calSpec, "Glob", "Zorblax", 1),
		"format":      strings.Replace(calSpec, `"MMMM"`, `"[Zorblax] MMMM"`, 1),
		"unmodelled":  strings.Replace(calSpec, `"kept"`, `"Zorblax"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			for _, tool := range []string{"create_calendar", "replace_calendar"} {
				h := rgHarness(t, "Zorblax")
				args := calArgs(t, spec)
				if tool == "replace_calendar" {
					args = calArgs(t, spec, "calendar_id", "cal1", "version", "v1")
				}
				r := h.call(tool, args)
				if !r.IsError {
					t.Fatalf("%s accepted: %s", tool, text(r))
				}
				wtRefused(t, h, text(r))
				if len(h.lk.Requests()) != 0 {
					t.Errorf("%s sent %+v", tool, h.lk.Requests())
				}
			}
		})
	}
}

// X-TOOL-04: calendar dry runs send no write.
func TestCalendarDryRuns_X_TOOL_04(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(calList), wtOK(calList))
	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"create_calendar", calArgs(t, calSpec, "dry_run", true), "would create calendar"},
		{"replace_calendar", calArgs(t, calSpec, "dry_run", true, "calendar_id", "cal1", "version", "old"), "now at version"},
		{"delete_calendar", map[string]any{"dry_run": true, "calendar_id": "cal1", "version": `W/"cv1"`}, "would delete calendar"},
	} {
		txt := wtMustOK(t, h, tc.tool, tc.args)
		if !strings.Contains(txt, tc.want) || !strings.Contains(txt, "Nothing was written") {
			t.Errorf("%s: %s", tc.tool, txt)
		}
	}
	if len(h.writes()) != 0 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestCalendarInputErrors(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	for name, tc := range map[string]struct {
		tool string
		args map[string]any
	}{
		"no spec":       {"create_calendar", map[string]any{}},
		"no name":       {"create_calendar", calArgs(t, `{"months":[]}`)},
		"id mismatch":   {"replace_calendar", calArgs(t, `{"id":"other",`+calSpec[1:], "calendar_id", "cal1", "version", "v1")},
		"no version":    {"replace_calendar", calArgs(t, calSpec, "calendar_id", "cal1")},
		"delete no ver": {"delete_calendar", map[string]any{"calendar_id": "cal1"}},
	} {
		if r := h.call(tc.tool, tc.args); !r.IsError {
			t.Errorf("%s: accepted %s", name, text(r))
		}
	}
	if len(h.lk.Requests()) != 0 {
		t.Errorf("requests %+v", h.lk.Requests())
	}
}
