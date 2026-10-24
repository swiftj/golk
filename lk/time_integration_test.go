//go:build integration

package lk_test

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

// cleanupCalendar deletes calendar id when the test ends, with whatever
// version it has then. The sandbox sweep does not know calendars.
func cleanupCalendar(t *testing.T, c *lk.Client, id string) {
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

func lanesOf(t *testing.T, c *lk.Client, doc lk.DocRef) []lk.Lane {
	t.Helper()
	p, err := c.Timelines.Lanes(sandbox.Ctx(t), doc)
	if err != nil {
		t.Fatal(err)
	}
	lk.SortByRank(p.Items)
	return p.Items
}

// setSources chains timelines through the escape hatch until
// DocumentUpdate.Sources exists.
func setSources(t *testing.T, c *lk.Client, doc lk.DocRef, sources ...lk.DocRef) {
	t.Helper()
	list := []map[string]string{}
	for _, s := range sources {
		list = append(list, map[string]string{"resourceId": s.ResourceID, "documentId": s.DocumentID})
	}
	path := "/api/v3/projects/" + sandbox.ProjectID(t) + "/resources/" + doc.ResourceID + "/docs/" + doc.DocumentID
	if err := c.Do(sandbox.Ctx(t), lk.Request{Method: http.MethodPatch, Path: path, Body: map[string]any{"sources": list}}, nil); err != nil {
		t.Fatalf("setting sources: %v", err)
	}
}

// TestIntegration_Timeline ports the guide's "timelines and calendars"
// example (C-LANE-01/02, C-EVENT-01..03, C-CAL-01).
func TestIntegration_Timeline(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)

	cals, err := c.Calendars.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, cal := range cals.Items {
		if cal.Version == "" || cal.Spec.Name == "" {
			t.Errorf("calendar %s without version or name: %+v", cal.ID, cal)
		}
		ids = append(ids, cal.ID)
	}
	for _, b := range lk.BuiltinCalendars() {
		if !slices.Contains(ids, b) {
			t.Errorf("calendars.list lacks built-in %s: %v", b, ids)
		}
	}

	brothers := create(t, c, lk.ResourceCreate{Name: "Finn and Jake"})
	timeline := firstDoc(brothers)

	finn, err := c.Timelines.CreateLane(ctx, timeline, lk.LaneCreate{Name: "Finn", Rank: "a0", Size: lk.LaneLarge})
	if err != nil {
		t.Fatal(err)
	}
	jake, err := c.Timelines.CreateLane(ctx, timeline, lk.LaneCreate{Name: "Jake", Rank: "a1", Size: lk.LaneSmall,
		Style: lk.Some(lk.LaneStyle{Color: lk.Some("#f4a300")})})
	if err != nil {
		t.Fatal(err)
	}
	if jake.Style == nil || jake.Style.Color.Or("") != "#f4a300" || jake.Size != lk.LaneSmall {
		t.Errorf("jake lane = %+v style %+v", jake, jake.Style)
	}
	if d, err := c.Documents.Get(ctx, timeline); err != nil || d.Type != lk.DocumentTime {
		t.Errorf("first lane did not make a timeline: %+v %v", d, err)
	}
	if _, err := c.Timelines.UpdateLane(ctx, timeline, jake.ID, lk.LaneUpdate{Name: lk.Some("Jake the Dog")}); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Timelines.Lane(ctx, timeline, jake.ID); err != nil || got.Name != "Jake the Dog" || got.Size != lk.LaneSmall {
		t.Errorf("updated lane = %+v, %v", got, err)
	}

	year := float64(60 * 24 * 365)
	mud, err := c.Timelines.CreateEvent(ctx, timeline, lk.EventCreate{LaneID: jake.ID, Type: lk.EventTypeEvent,
		Name: lk.Some("Rolls in the magic mud puddle"), Start: lk.Some(1000 * year)})
	if err != nil {
		t.Fatal(err)
	}
	found, err := c.Timelines.CreateEvent(ctx, timeline, lk.EventCreate{LaneID: jake.ID, Type: lk.EventTypeEvent,
		Name: lk.Some("Jake's family finds baby Finn"), Start: lk.Some(1016 * year)})
	if err != nil {
		t.Fatal(err)
	}
	heroes, err := c.Timelines.CreateEvent(ctx, timeline, lk.EventCreate{LaneID: finn.ID, Type: lk.EventTypeEvent,
		Name: lk.Some("Adventuring out of the Tree Fort"), Start: lk.Some(1028 * year), End: lk.Some(1033 * year)})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(mud.FormattedStart, heroes.FormattedStart, heroes.FormattedEnd)
	t.Log(heroes.StartDate.Year, heroes.StartDate.Era.Abbr, heroes.StartDate.ISO, heroes.EndDate)
	if mud.FormattedStart == "" || heroes.FormattedEnd == "" || heroes.EndDate == nil || mud.EndDate != nil {
		t.Errorf("formatted dates: mud %+v heroes %+v", mud, heroes)
	}
	if heroes.StartDate.Year != 1028 || heroes.StartDate.ISO == "" || heroes.EndDate.ISO == "" || heroes.StartDate.Era.Abbr == "" {
		t.Errorf("heroes start %+v end %+v", heroes.StartDate, heroes.EndDate)
	}

	if _, err := c.Timelines.UpdateEvent(ctx, timeline, found.ID, lk.EventUpdate{LaneID: lk.Some(finn.ID), Name: lk.Some("Found in the woods by the Dogs")}); err != nil {
		t.Fatal(err)
	}
	moved, err := c.Timelines.Event(ctx, timeline, found.ID)
	if err != nil || moved.Name != "Found in the woods by the Dogs" || moved.LaneID != finn.ID || moved.Start != found.Start {
		t.Errorf("moved = %+v, %v", moved, err)
	}

	// A page of one forces the iterator to follow cursors.
	var names []string
	var last float64
	for e, err := range c.Timelines.AllEvents(ctx, timeline, lk.EventListParams{Limit: 1}) {
		if err != nil {
			t.Fatal(err)
		}
		if e.Start < last {
			t.Errorf("events out of start order: %v after %v", e.Start, last)
		}
		last = e.Start
		names = append(names, e.Name)
	}
	if len(names) != 3 {
		t.Errorf("events = %v", names)
	}
	lanes := lanesOf(t, c, timeline)
	if len(lanes) != 2 || lanes[0].ID != finn.ID || lanes[1].Name != "Jake the Dog" {
		t.Errorf("lanes = %+v", lanes)
	}

	if err := c.Timelines.DeleteEvent(ctx, timeline, mud.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Timelines.Event(ctx, timeline, mud.ID); !errors.Is(err, lk.ErrNotFound) {
		t.Errorf("deleted event read = %v, want ErrNotFound", err)
	}
	if err := c.Timelines.DeleteLane(ctx, timeline, jake.ID); err != nil {
		t.Fatal(err)
	}
	if lanes := lanesOf(t, c, timeline); len(lanes) != 1 || lanes[0].ID != finn.ID {
		t.Errorf("lanes after delete = %+v", lanes)
	}
}

// TestIntegration_TimelineChaining ports "Chaining timelines". Sources are
// set through the escape hatch (Client.Do) until DocumentUpdate.Sources
// exists.
func TestIntegration_TimelineChaining(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := create(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	candy := create(t, c, lk.ResourceCreate{Name: "The Candy Kingdom"})
	history, kingdom := firstDoc(ooo), firstDoc(candy)

	own, err := c.Timelines.CreateLane(ctx, history, lk.LaneCreate{Name: "The Land of Ooo", Rank: "a0", Size: lk.LaneLarge})
	if err != nil {
		t.Fatal(err)
	}
	src, err := c.Timelines.CreateLane(ctx, kingdom, lk.LaneCreate{Name: "The Candy Kingdom", Rank: "a1", Size: lk.LaneSmall})
	if err != nil {
		t.Fatal(err)
	}
	srcEvent, err := c.Timelines.CreateEvent(ctx, kingdom, lk.EventCreate{LaneID: src.ID, Type: lk.EventTypeEvent,
		Name: lk.Some("The first Candy Person"), FormattedStart: lk.Some("200")})
	if err != nil {
		t.Fatal(err)
	}

	setSources(t, c, history, kingdom)
	lanes := lanesOf(t, c, history)
	if len(lanes) != 2 || lanes[0].ID != own.ID || lanes[1].ID != src.ID {
		t.Errorf("chained lanes = %+v", lanes)
	}
	var events []string
	for e, err := range c.Timelines.AllEvents(ctx, history, lk.EventListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, e.ID)
	}
	if !slices.Equal(events, []string{srcEvent.ID}) {
		t.Errorf("chained events = %v, want the source's %s", events, srcEvent.ID)
	}
	// Get reads only the timeline's own lanes.
	if _, err := c.Timelines.Lane(ctx, history, src.ID); !errors.Is(err, lk.ErrNotFound) {
		t.Errorf("get of a chained lane = %v, want ErrNotFound", err)
	}

	setSources(t, c, history)
	if lanes := lanesOf(t, c, history); len(lanes) != 1 {
		t.Errorf("lanes after clearing sources = %+v", lanes)
	}
}

// oooCalendar is the guide's Calendar of Ooo with a sandbox name.
func oooCalendar() lk.CalendarSpec {
	month := func(id, name string, n float64) lk.CalendarMonth {
		return lk.CalendarMonth{ID: id, Name: name, Length: n, Interval: 1}
	}
	math := month("mathematical", "Mathematical", 5)
	math.IsIntercalary = true
	return lk.CalendarSpec{
		Name:        sandbox.Name("The Calendar of Ooo"),
		Description: lk.Some("Four kingdom months of thirty days and the five-day Mathematical festival, counted from the Great Mushroom War."),
		MaxMinutes:  1_800_000_000,
		Months: []lk.CalendarMonth{month("candy", "Candy", 30), month("ice", "Ice", 30), month("flame", "Flame", 30),
			month("lumpy-space", "Lumpy Space", 30), math},
		LeapDays: []lk.CalendarLeapDay{{ID: "gunter-day", Name: "Gunter Day", Month: 3, Day: 1, Interval: "4"}},
		Weekdays: []lk.CalendarWeekday{{ID: "finn", Name: "Finnsday"}, {ID: "jake", Name: "Jakesday"},
			{ID: "bubblegum", Name: "Bubblegumsday"}, {ID: "marceline", Name: "Marcelinesday"}, {ID: "bmo", Name: "BMOsday"}},
		WeekResetsEachMonth: lk.Some(false), HoursInDay: 24, MinutesInHour: 60,
		NegativeEra: lk.Some(lk.CalendarEra{ID: "before-war", Name: "Before the Mushroom War", Abbr: lk.Some("BMW"),
			HideAbbr: lk.Some(false), StartsAt: -180_000_000, ResetMode: lk.EraResetNone}),
		PositiveEras: []lk.CalendarEra{{ID: "after-war", Name: "After the Mushroom War", Abbr: lk.Some("AMW"),
			HideAbbr: lk.Some(false), StartsAt: 0, ResetMode: lk.EraResetNone}},
		Moons: lk.Some([]lk.CalendarMoon{{ID: "the-moon", Name: "The Moon", Phase: 40_320, Color: lk.Some("#E8E4D8")}}),
		Format: lk.Some(lk.CalendarFormat{ID: "ooo", Year: "YYYY E", Month: "MMMM YYYY E", Day: "MMMM D^, YYYY E",
			Time: "MMMM D^, YYYY E, HH:mm"}),
		HalfClock: lk.Some(false),
	}
}

// TestIntegration_Calendars ports the guide's "Calendars" example (C-CAL-01,
// C-CAL-02, C-CAL-03).
func TestIntegration_Calendars(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	spec := oooCalendar()
	cal, err := c.Calendars.Create(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	cleanupCalendar(t, c, cal.ID)
	if cal.ID == "" || cal.Version == "" || cal.Spec.Name != spec.Name || len(cal.Spec.Months) != 5 {
		t.Fatalf("created %+v", cal)
	}
	version := cal.Version

	ooo := create(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	timeline := firstDoc(ooo)
	lane, err := c.Timelines.CreateLane(ctx, timeline, lk.LaneCreate{Name: "Kingdoms", Rank: "a0", Size: lk.LaneLarge})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Documents.Update(ctx, timeline, lk.DocumentUpdate{CalendarID: lk.Some(cal.ID)}); err != nil {
		t.Fatal(err)
	}
	if d, err := c.Documents.Get(ctx, timeline); err != nil || d.CalendarID != cal.ID {
		t.Fatalf("calendarId = %+v, %v", d, err)
	}
	for _, e := range []struct{ name, start string }{
		{"Princess Bubblegum makes the first Candy Person", "200 AMW"},
		{"Finn is found in the woods", "Flame 14th, 1016 AMW"},
	} {
		if _, err := c.Timelines.CreateEvent(ctx, timeline, lk.EventCreate{LaneID: lane.ID, Type: lk.EventTypeEvent,
			Name: lk.Some(e.name), FormattedStart: lk.Some(e.start)}); err != nil {
			t.Fatal(err)
		}
	}
	var got []lk.Event
	for e, err := range c.Timelines.AllEvents(ctx, timeline, lk.EventListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		d := e.StartDate
		t.Log(e.FormattedStart, e.Name, "|", d.Day, d.MonthName, d.Year, d.Era.Abbr)
		got = append(got, e)
	}
	if len(got) != 2 || got[0].StartDate.Year != 200 || got[0].StartDate.Era.Abbr != "AMW" ||
		got[1].StartDate.MonthName != "Flame" || got[1].StartDate.Day != 14 || got[1].StartDate.Year != 1016 || got[1].StartDate.ISO != "" {
		t.Errorf("events = %+v", got)
	}

	renamed := spec
	renamed.Name = sandbox.Name("The Ooo Reckoning")
	up, err := c.Calendars.Update(ctx, cal.ID, renamed, lk.IfMatch(version))
	if err != nil {
		t.Fatal(err)
	}
	if up.Version == "" || up.Version == version || up.Spec.Name != renamed.Name {
		t.Errorf("renamed = %+v", up)
	}
	version = up.Version

	_, err = c.Calendars.Update(ctx, cal.ID, spec, lk.IfMatch(cal.Version))
	var ae *lk.APIError
	if !errors.Is(err, lk.ErrVersionMismatch) || !errors.As(err, &ae) || ae.CurrentVersion != version {
		t.Errorf("stale update = %v (%+v), want ErrVersionMismatch with CurrentVersion %s", err, ae, version)
	}

	// The guide's TypeScript catches a 400 ValidationError; the spec and the
	// server name the code KIND_MISMATCH.
	if _, err := c.Calendars.Update(ctx, lk.CalendarGregorian, spec); !errors.Is(err, lk.ErrKindMismatch) {
		t.Errorf("update of a built-in = %v, want ErrKindMismatch", err)
	}

	// The listed calendar round-trips: replacing it with its own spec works.
	for listed, err := range c.Calendars.All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		if listed.ID == cal.ID {
			again, err := c.Calendars.Update(ctx, cal.ID, listed.Spec, lk.IfMatch(listed.Version))
			if err != nil || again.Spec.Name != renamed.Name {
				t.Errorf("replace with listed spec = %+v, %v", again, err)
			} else {
				version = again.Version
			}
		}
	}

	if _, err := c.Documents.Update(ctx, timeline, lk.DocumentUpdate{CalendarID: lk.Some(lk.CalendarHarptos)}); err != nil {
		t.Fatal(err)
	}
	for e, err := range c.Timelines.AllEvents(ctx, timeline, lk.EventListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		t.Log("harptos:", e.FormattedStart, e.Name)
	}

	if err := c.Calendars.Delete(ctx, cal.ID, lk.IfMatch(cal.Version)); !errors.Is(err, lk.ErrVersionMismatch) {
		t.Errorf("stale delete = %v, want ErrVersionMismatch", err)
	}
	if err := c.Calendars.Delete(ctx, cal.ID, lk.IfMatch(version)); err != nil {
		t.Fatal(err)
	}
	if err := c.Calendars.Delete(ctx, cal.ID); !errors.Is(err, lk.ErrNotFound) {
		t.Errorf("second delete = %v, want ErrNotFound", err)
	}
}

// TestIntegration_Templates ports the guide's "templates" example (C-TPL-01,
// C-TPL-02).
func TestIntegration_Templates(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	kingdom := create(t, c, lk.ResourceCreate{Name: "Kingdom", Tags: lk.Some([]string{"Kingdom"})})
	if _, err := c.Documents.ReplaceBlocks(ctx, firstDoc(kingdom), blocks.List{blocks.P("Ruler, capital, allies.")}); err != nil {
		t.Fatal(err)
	}
	create(t, c, lk.ResourceCreate{Name: "Castle", ParentID: lk.Some(kingdom.ID)})

	tpl, err := c.Resources.SaveAsTemplate(ctx, kingdom.ID, lk.SaveAsTemplate{Name: lk.Some(sandbox.Name("Kingdom Template")), IncludeChildren: lk.Some(true)})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, tpl.ID)
	if tpl.ParentID != lk.ParentTemplates || tpl.Name != sandbox.Name("Kingdom Template") {
		t.Errorf("template = %+v", tpl)
	}

	var listed []string
	for r, err := range c.Templates.All(ctx, lk.TemplateListParams{Limit: 100}) {
		if err != nil {
			t.Fatal(err)
		}
		listed = append(listed, r.ID)
	}
	if !slices.Contains(listed, tpl.ID) {
		t.Errorf("templates.list lacks %s", tpl.ID)
	}

	realm, err := c.Resources.CreateFromTemplate(ctx, lk.FromTemplate{TemplateID: tpl.ID, Name: lk.Some(sandbox.Name("Realm")), IncludeChildren: lk.Some(true)})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, realm.ID)
	page, err := c.Documents.GetBlocks(ctx, firstDoc(realm))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(realm.Name, realm.Tags, page.Text)
	if !slices.Equal(realm.Tags, []string{"Kingdom"}) || page.Text != "Ruler, capital, allies." || realm.ParentID != "" {
		t.Errorf("realm = %+v, text %q", realm, page.Text)
	}
	var kids []string
	for r, err := range c.Resources.All(ctx, lk.ResourceListParams{ParentID: realm.ID}) {
		if err != nil {
			t.Fatal(err)
		}
		kids = append(kids, r.Name)
	}
	if !slices.Equal(kids, []string{sandbox.Name("Castle")}) {
		t.Errorf("realm children = %v", kids)
	}

	if _, err := c.Resources.CreateFromTemplate(ctx, lk.FromTemplate{TemplateID: kingdom.ID}); !errors.Is(err, lk.ErrValidation) {
		t.Errorf("copy from a non-template = %v, want ErrValidation", err)
	}
}
