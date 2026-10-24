package lk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// oooSpec is the guide's Calendar of Ooo, exactly as the SDK encodes it.
const oooSpec = `{"name":"The Calendar of Ooo","description":"Four kingdom months.","hasZeroYear":false,"maxMinutes":1800000000,` +
	`"months":[{"id":"candy","name":"Candy","length":30,"interval":1,"offset":0,"isIntercalary":false},` +
	`{"id":"mathematical","name":"Mathematical","length":5,"interval":1,"offset":0,"isIntercalary":true}],` +
	`"leapDays":[{"id":"gunter-day","name":"Gunter Day","month":1,"interval":"4","offset":0,"day":1,"addsWeekDay":false,"intercalary":false,"weekDay":""}],` +
	`"weekdays":[{"id":"finn","name":"Finnsday"},{"id":"jake","name":"Jakesday"}],"epochWeekday":0,"weekResetsEachMonth":false,` +
	`"hoursInDay":24,"minutesInHour":60,` +
	`"negativeEra":{"id":"before-war","name":"Before the Mushroom War","abbr":"BMW","hideAbbr":false,"startsAt":-180000000,"resetMode":"none"},` +
	`"positiveEras":[{"id":"after-war","name":"After the Mushroom War","abbr":"AMW","hideAbbr":false,"startsAt":0,"resetMode":"none"}],` +
	`"moons":[{"id":"the-moon","name":"The Moon","phase":40320,"shift":0,"color":"#E8E4D8"}],` +
	`"format":{"id":"ooo","year":"YYYY E","month":"MMMM YYYY E","day":"MMMM D^, YYYY E","time":"MMMM D^, YYYY E, HH:mm"},"halfClock":false}`

func ooo() CalendarSpec {
	month := func(id, name string, n float64) CalendarMonth {
		return CalendarMonth{ID: id, Name: name, Length: n, Interval: 1}
	}
	math := month("mathematical", "Mathematical", 5)
	math.IsIntercalary = true
	return CalendarSpec{
		Name: "The Calendar of Ooo", Description: Some("Four kingdom months."), MaxMinutes: 1_800_000_000,
		Months:              []CalendarMonth{month("candy", "Candy", 30), math},
		LeapDays:            []CalendarLeapDay{{ID: "gunter-day", Name: "Gunter Day", Month: 1, Interval: "4", Day: 1}},
		Weekdays:            []CalendarWeekday{{ID: "finn", Name: "Finnsday"}, {ID: "jake", Name: "Jakesday"}},
		WeekResetsEachMonth: Some(false), HoursInDay: 24, MinutesInHour: 60,
		NegativeEra: Some(CalendarEra{ID: "before-war", Name: "Before the Mushroom War", Abbr: Some("BMW"), HideAbbr: Some(false),
			StartsAt: -180_000_000, ResetMode: EraResetNone}),
		PositiveEras: []CalendarEra{{ID: "after-war", Name: "After the Mushroom War", Abbr: Some("AMW"), HideAbbr: Some(false), ResetMode: EraResetNone}},
		Moons:        Some([]CalendarMoon{{ID: "the-moon", Name: "The Moon", Phase: 40_320, Color: Some("#E8E4D8")}}),
		Format:       Some(CalendarFormat{ID: "ooo", Year: "YYYY E", Month: "MMMM YYYY E", Day: "MMMM D^, YYYY E", Time: "MMMM D^, YYYY E, HH:mm"}),
		HalfClock:    Some(false),
	}
}

// withIDVersion adds "id" first and "version" last to a spec body, as the
// server answers.
func withIDVersion(id, version, spec string) string {
	return `{"id":"` + id + `",` + spec[1:len(spec)-1] + `,"version":"` + version + `"}`
}

func TestCalendars(t *testing.T) {
	calJSON := withIDVersion("cal1", "cv1", oooSpec)
	runWiki(t, []wikiCase{
		{name: "C-CAL-01 list is one page with versions", reply: testutil.JSON(200, `{"data":[`+calJSON+`,{"id":"gregorian","name":"Gregorian","version":"g1"}]}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Calendars.List(ctx) },
			method: "GET", path: pP + "/calendars",
			check: func(t *testing.T, v any) {
				p := v.(*Page[Calendar])
				if len(p.Items) != 2 || p.HasMore {
					t.Fatalf("page = %+v", p)
				}
				if p.Items[0].ID != "cal1" || p.Items[0].Version != "cv1" || p.Items[0].Spec.Name != "The Calendar of Ooo" ||
					p.Items[1].ID != "gregorian" || p.Items[1].Version != "g1" {
					t.Errorf("calendars = %+v", p.Items)
				}
				// The version is not kept as an unmodelled spec field.
				assertJSON(t, mustMarshal(t, p.Items[0].Spec), `{"id":"cal1",`+oooSpec[1:])
			}},
		{name: "C-CAL-02 create sends the spec", reply: etag(testutil.JSON(201, calJSON), "cv1"),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Calendars.Create(ctx, ooo()) },
			method: "POST", path: pP + "/calendars", keyed: true, body: oooSpec,
			check: func(t *testing.T, v any) {
				cal := v.(*Calendar)
				if cal.ID != "cal1" || cal.Version != "cv1" || cal.Spec.ID != "cal1" {
					t.Errorf("calendar = %+v", cal)
				}
			}},
		{name: "C-CAL-02 create takes the version from the ETag", reply: etag(testutil.JSON(201, `{"id":"cal1","name":"x"}`), "cv9"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Calendars.Create(ctx, CalendarSpec{ID: "cal1", Name: "x"})
			},
			method: "POST", path: pP + "/calendars", keyed: true,
			body: `{"id":"cal1","name":"x","hasZeroYear":false,"maxMinutes":0,"months":[],"leapDays":[],"weekdays":[],"epochWeekday":0,"hoursInDay":0,"minutesInHour":0,"positiveEras":[]}`,
			check: func(t *testing.T, v any) {
				if cal := v.(*Calendar); cal.Version != "cv9" {
					t.Errorf("Version = %q, want cv9 from the ETag", cal.Version)
				}
			}},
		{name: "C-CAL-02 update replaces with If-Match", reply: etag(testutil.JSON(200, withIDVersion("cal1", "cv2", oooSpec)), "cv2"),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Calendars.Update(ctx, "cal1", ooo(), IfMatch("cv1"))
			},
			method: "PUT", path: pP + "/calendars/cal1", ifMatch: "cv1", body: oooSpec,
			check: func(t *testing.T, v any) {
				if cal := v.(*Calendar); cal.Version != "cv2" {
					t.Errorf("Version = %q", cal.Version)
				}
			}},
		{name: "C-CAL-02 update without If-Match", reply: testutil.JSON(200, calJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Calendars.Update(ctx, "cal1", ooo()) },
			method: "PUT", path: pP + "/calendars/cal1", body: oooSpec},
		{name: "C-CAL-02 delete with If-Match", reply: noContent(),
			call: func(ctx context.Context, c *Client) (any, error) {
				return nil, c.Calendars.Delete(ctx, "cal1", IfMatch("cv2"))
			},
			method: "DELETE", path: pP + "/calendars/cal1", ifMatch: "cv2"},
	})
}

func TestCalendars_C_CAL_02_StaleVersion(t *testing.T) {
	r := testutil.JSON(412, `{"message":"Calendar version is cv2, If-Match was cv1","code":"VERSION_MISMATCH","requestId":"rq1"}`)
	srv := testutil.NewFakeServer(t, r)
	c, _ := newTestClient(t, srv)
	_, err := c.Calendars.Update(context.Background(), "cal1", ooo(), IfMatch("cv1"))
	var ae *APIError
	if !errors.Is(err, ErrVersionMismatch) || !errors.As(err, &ae) || ae.CurrentVersion != "cv2" {
		t.Fatalf("err = %v (%+v), want ErrVersionMismatch with CurrentVersion cv2", err, ae)
	}
}

func TestCalendars_C_CAL_02_CreateRefusesIfMatch(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	if _, err := c.Calendars.Create(context.Background(), ooo(), IfMatch("v1")); !errors.Is(err, ErrIfMatchNotAllowed) {
		t.Errorf("err = %v, want ErrIfMatchNotAllowed", err)
	}
	if len(srv.Requests()) != 0 {
		t.Error("a request was sent")
	}
}

func TestCalendars_Builtins(t *testing.T) {
	want := []string{"gregorian", "harptos", "eberron", "exandria", "greyhawk", "elder-scrolls"}
	if got := BuiltinCalendars(); !slices.Equal(got, want) {
		t.Errorf("BuiltinCalendars = %v", got)
	}
	BuiltinCalendars()[0] = "changed"
	if !IsBuiltinCalendar("gregorian") || IsBuiltinCalendar("cal1") {
		t.Error("IsBuiltinCalendar")
	}
}

// unknownSpec has unmodelled fields at the top level and in every nested
// object kind.
const unknownSpec = `{"name":"Ooo","hasZeroYear":true,"maxMinutes":10,` +
	`"months":[{"id":"m","name":"M","length":1,"interval":1,"offset":0,"isIntercalary":false,"color":"#fff"}],` +
	`"leapDays":[{"id":"l","name":"L","month":0,"interval":"4","offset":0,"day":1,"addsWeekDay":false,"intercalary":false,"weekDay":"","note":{"a":[1,2]}}],` +
	`"weekdays":[{"id":"w","name":"W","abbr":"Wd"}],"epochWeekday":0,"hoursInDay":24,"minutesInHour":60,` +
	`"negativeEra":{"id":"n","name":"N","startsAt":-1,"resetMode":"none","x":1},` +
	`"positiveEras":[{"id":"p","name":"P","startsAt":0,"resetMode":"regnal","y":null}],` +
	`"moons":[{"id":"o","name":"O","phase":1.5,"shift":-0.25,"tide":true}],` +
	`"format":{"id":"f","year":"YYYY","month":"MMMM","day":"D","time":"HH","week":"W"},` +
	`"futureField":{"deep":[{"k":"v"}]},"another":"z"}`

func TestCalendars_C_CAL_03_LosslessRoundTrip(t *testing.T) {
	var spec CalendarSpec
	if err := json.Unmarshal([]byte(unknownSpec), &spec); err != nil {
		t.Fatal(err)
	}
	if spec.PositiveEras[0].ResetMode != "regnal" || spec.Moons.Or(nil)[0].Shift != -0.25 {
		t.Errorf("decoded %+v", spec)
	}
	// Unknown fields go last, in name order, at every level.
	want := `{"name":"Ooo","hasZeroYear":true,"maxMinutes":10,` +
		`"months":[{"id":"m","name":"M","length":1,"interval":1,"offset":0,"isIntercalary":false,"color":"#fff"}],` +
		`"leapDays":[{"id":"l","name":"L","month":0,"interval":"4","offset":0,"day":1,"addsWeekDay":false,"intercalary":false,"weekDay":"","note":{"a":[1,2]}}],` +
		`"weekdays":[{"id":"w","name":"W","abbr":"Wd"}],"epochWeekday":0,"hoursInDay":24,"minutesInHour":60,` +
		`"negativeEra":{"id":"n","name":"N","startsAt":-1,"resetMode":"none","x":1},` +
		`"positiveEras":[{"id":"p","name":"P","startsAt":0,"resetMode":"regnal","y":null}],` +
		`"moons":[{"id":"o","name":"O","phase":1.5,"shift":-0.25,"tide":true}],` +
		`"format":{"id":"f","year":"YYYY","month":"MMMM","day":"D","time":"HH","week":"W"},` +
		`"another":"z","futureField":{"deep":[{"k":"v"}]}}`
	assertJSON(t, mustMarshal(t, spec), want)

	// Editing a modelled field keeps the unmodelled ones.
	spec.Name = "Ooo Reckoning"
	spec.Months[0].Length = 2
	got := string(mustMarshal(t, spec))
	for _, s := range []string{`"name":"Ooo Reckoning"`, `"length":2`, `"color":"#fff"`, `"futureField":{"deep":[{"k":"v"}]}`, `"tide":true`, `"week":"W"`} {
		if !bytes.Contains([]byte(got), []byte(s)) {
			t.Errorf("edited spec lost %s: %s", s, got)
		}
	}

	// An unmodelled field differing only in case from a modelled one is kept
	// apart from it, as the server matches names exactly.
	var cased CalendarWeekday
	if err := json.Unmarshal([]byte(`{"id":"w","Name":"X"}`), &cased); err != nil {
		t.Fatal(err)
	}
	if cased.Name != "" {
		t.Errorf("Name = %q, want it unread", cased.Name)
	}
	assertJSON(t, mustMarshal(t, cased), `{"id":"w","name":"","Name":"X"}`)
}

// C-CAL-03: a calendar read through the client and written back sends the
// fields the server sent, except id and version.
func TestCalendars_C_CAL_03_UpdateEchoesUnknownFields(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.JSON(200, `{"data":[`+withIDVersion("cal1", "cv1", unknownSpec)+`]}`),
		testutil.JSON(200, withIDVersion("cal1", "cv2", unknownSpec)),
	)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	p, err := c.Calendars.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cal := p.Items[0]
	spec := cal.Spec
	spec.ID = ""
	if _, err := c.Calendars.Update(ctx, cal.ID, spec, IfMatch(cal.Version)); err != nil {
		t.Fatal(err)
	}
	sent := srv.Requests()[1]
	if sent.Method != http.MethodPut || sent.Header.Get("If-Match") != "cv1" {
		t.Errorf("sent %s If-Match %q", sent.Method, sent.Header.Get("If-Match"))
	}
	var a, b any
	_ = json.Unmarshal(sent.Body, &a)
	_ = json.Unmarshal([]byte(unknownSpec), &b)
	if string(mustMarshal(t, a)) != string(mustMarshal(t, b)) {
		t.Errorf("sent\n%s\nwant the same fields as\n%s", sent.Body, unknownSpec)
	}
}

// FuzzCalendarSpecRoundTrip checks that decoding and encoding a spec is
// stable: encode(decode(x)) decodes back to the same encoding.
func FuzzCalendarSpecRoundTrip(f *testing.F) {
	f.Add([]byte(oooSpec))
	f.Add([]byte(unknownSpec))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"months":[null],"moons":null,"Name":"x","name":"y"}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var s CalendarSpec
		if err := json.Unmarshal(data, &s); err != nil {
			return
		}
		once, err := json.Marshal(s)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		var s2 CalendarSpec
		if err := json.Unmarshal(once, &s2); err != nil {
			t.Fatalf("re-decode %s: %v", once, err)
		}
		twice, err := json.Marshal(s2)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(once, twice) {
			t.Fatalf("unstable:\n%s\n%s", once, twice)
		}
		// Every top-level field of the input survives.
		var in, out map[string]json.RawMessage
		if json.Unmarshal(data, &in) != nil {
			return
		}
		_ = json.Unmarshal(once, &out)
		for k := range in {
			if _, ok := out[k]; !ok && string(bytes.TrimSpace(in[k])) != "null" {
				t.Fatalf("field %q dropped: %s -> %s", k, data, once)
			}
		}
	})
}
