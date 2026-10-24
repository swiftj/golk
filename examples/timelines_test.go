//go:build integration

package examples

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// Ports of the guide's "timelines and calendars", "Chaining timelines" and
// "Calendars" examples, with made-up names under the sandbox prefix.

// TestTimelinesAndCalendars ports "timelines and calendars".
func TestTimelinesAndCalendars(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)

	cals, err := c.Calendars.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, cal := range cals.Items {
		names = append(names, cal.Spec.Name)
	}
	t.Log(names)

	brothers := createResource(t, c, lk.ResourceCreate{Name: "Finn and Jake"})
	timeline := page(brothers)

	finn, err := c.Timelines.CreateLane(ctx, timeline, lk.LaneCreate{Name: "Finn", Rank: "a0", Size: lk.LaneLarge})
	if err != nil {
		t.Fatal(err)
	}
	jake, err := c.Timelines.CreateLane(ctx, timeline, lk.LaneCreate{Name: "Jake", Rank: "a1", Size: lk.LaneSmall,
		Style: lk.Some(lk.LaneStyle{Color: lk.Some("#f4a300")})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Timelines.UpdateLane(ctx, timeline, jake.ID, lk.LaneUpdate{Name: lk.Some("Jake the Dog")}); err != nil {
		t.Fatal(err)
	}
	got, err := c.Timelines.Lane(ctx, timeline, jake.ID)
	if err != nil || got.Name != "Jake the Dog" {
		t.Fatalf("lane = %+v, %v", got, err)
	}

	year := float64(60 * 24 * 365)
	event := func(lane string, name string, start float64, end ...float64) *lk.Event {
		e := lk.EventCreate{LaneID: lane, Type: lk.EventTypeEvent, Name: lk.Some(name), Start: lk.Some(start)}
		if len(end) > 0 {
			e.End = lk.Some(end[0])
		}
		ev, err := c.Timelines.CreateEvent(ctx, timeline, e)
		if err != nil {
			t.Fatal(err)
		}
		return ev
	}
	mud := event(jake.ID, "Rolls in the magic mud puddle", 1000*year)
	found := event(jake.ID, "Jake's family finds baby Finn", 1016*year)
	heroes := event(finn.ID, "Adventuring out of the Tree Fort", 1028*year, 1033*year)
	t.Log(mud.FormattedStart, heroes.FormattedStart, heroes.FormattedEnd)
	if heroes.EndDate == nil {
		t.Fatal("heroes has no endDate")
	}
	t.Log(heroes.StartDate.Year, heroes.StartDate.Era.Abbr, heroes.StartDate.ISO, heroes.EndDate.ISO)

	if _, err := c.Timelines.UpdateEvent(ctx, timeline, found.ID, lk.EventUpdate{LaneID: lk.Some(finn.ID), Name: lk.Some("Found in the woods by the Dogs")}); err != nil {
		t.Fatal(err)
	}
	moved, err := c.Timelines.Event(ctx, timeline, found.ID)
	if err != nil || moved.LaneID != finn.ID {
		t.Fatalf("moved = %+v, %v", moved, err)
	}
	t.Log(moved.Name, moved.LaneID == finn.ID)

	n := 0
	for e, err := range c.Timelines.AllEvents(ctx, timeline, lk.EventListParams{Limit: 50}) {
		if err != nil {
			t.Fatal(err)
		}
		t.Log(e.Start, e.Name)
		n++
	}
	lanes, err := c.Timelines.Lanes(ctx, timeline)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lanes.Items {
		t.Log(l.Rank, l.Name, l.Size)
	}
	if n != 3 || len(lanes.Items) != 2 {
		t.Errorf("%d events, %d lanes", n, len(lanes.Items))
	}

	if err := c.Timelines.DeleteEvent(ctx, timeline, mud.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.Timelines.DeleteLane(ctx, timeline, jake.ID); err != nil {
		t.Fatal(err)
	}
}

// TestChainingTimelines ports "Chaining timelines".
func TestChainingTimelines(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ooo := createResource(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	candy := createResource(t, c, lk.ResourceCreate{Name: "The Candy Kingdom"})
	history, kingdom := page(ooo), page(candy)

	if _, err := c.Timelines.CreateLane(ctx, history, lk.LaneCreate{Name: "The Land of Ooo", Rank: "a0", Size: lk.LaneLarge}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Timelines.CreateLane(ctx, kingdom, lk.LaneCreate{Name: "The Candy Kingdom", Rank: "a1", Size: lk.LaneSmall}); err != nil {
		t.Fatal(err)
	}

	chained, err := c.Documents.Update(ctx, history, lk.DocumentUpdate{Sources: lk.Some([]lk.DocRef{kingdom})})
	if err != nil {
		t.Fatal(err)
	}
	t.Log(chained.Sources)
	lanes, err := c.Timelines.Lanes(ctx, history)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lanes.Items {
		t.Log(l.Rank, l.Name)
	}
	if !slices.Equal(chained.Sources, []lk.DocRef{kingdom}) || len(lanes.Items) != 2 {
		t.Errorf("sources %+v, lanes %+v", chained.Sources, lanes.Items)
	}

	if _, err := c.Documents.Update(ctx, history, lk.DocumentUpdate{Sources: lk.Some([]lk.DocRef{})}); err != nil {
		t.Fatal(err)
	}
	lanes, err = c.Timelines.Lanes(ctx, history)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(len(lanes.Items))
	if len(lanes.Items) != 1 {
		t.Errorf("lanes after clearing = %d", len(lanes.Items))
	}
}

// TestCalendars ports "Calendars": the Calendar of Ooo.
func TestCalendars(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	month := func(id, name string, length float64) lk.CalendarMonth {
		return lk.CalendarMonth{ID: id, Name: name, Length: length, Interval: 1}
	}
	mathematical := month("mathematical", "Mathematical", 5)
	mathematical.IsIntercalary = true
	spec := lk.CalendarSpec{
		Name:        sandbox.Name("The Calendar of Ooo"),
		Description: lk.Some("Four kingdom months of thirty days and the five-day Mathematical festival, counted from the Great Mushroom War."),
		HasZeroYear: false,
		MaxMinutes:  1_800_000_000,
		Months: []lk.CalendarMonth{month("candy", "Candy", 30), month("ice", "Ice", 30), month("flame", "Flame", 30),
			month("lumpy-space", "Lumpy Space", 30), mathematical},
		LeapDays: []lk.CalendarLeapDay{{ID: "gunter-day", Name: "Gunter Day", Month: 3, Day: 1, Interval: "4"}},
		Weekdays: []lk.CalendarWeekday{{ID: "finn", Name: "Finnsday"}, {ID: "jake", Name: "Jakesday"},
			{ID: "bubblegum", Name: "Bubblegumsday"}, {ID: "marceline", Name: "Marcelinesday"}, {ID: "bmo", Name: "BMOsday"}},
		EpochWeekday:        0,
		WeekResetsEachMonth: lk.Some(false),
		HoursInDay:          24,
		MinutesInHour:       60,
		NegativeEra: lk.Some(lk.CalendarEra{ID: "before-war", Name: "Before the Mushroom War", Abbr: lk.Some("BMW"),
			HideAbbr: lk.Some(false), StartsAt: -180_000_000, ResetMode: lk.EraResetNone}),
		PositiveEras: []lk.CalendarEra{{ID: "after-war", Name: "After the Mushroom War", Abbr: lk.Some("AMW"),
			HideAbbr: lk.Some(false), StartsAt: 0, ResetMode: lk.EraResetNone}},
		Moons: lk.Some([]lk.CalendarMoon{{ID: "the-moon", Name: "The Moon", Phase: 40_320, Shift: 0, Color: lk.Some("#E8E4D8")}}),
		Format: lk.Some(lk.CalendarFormat{ID: "ooo", Year: "YYYY E", Month: "MMMM YYYY E", Day: "MMMM D^, YYYY E",
			Time: "MMMM D^, YYYY E, HH:mm"}),
		HalfClock: lk.Some(false),
	}

	calendar, err := c.Calendars.Create(ctx, spec)
	if err != nil {
		t.Fatal(err)
	}
	version := calendar.Version
	// The sandbox sweep does not know calendars; delete it with its latest
	// version, as the guide's finally block does.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.Calendars.Delete(ctx, calendar.ID, lk.IfMatch(version)); err != nil {
			t.Errorf("cleanup: deleting calendar %s: %v", calendar.ID, err)
		}
	})

	ooo := createResource(t, c, lk.ResourceCreate{Name: "The Land of Ooo"})
	timeline := page(ooo)
	lane, err := c.Timelines.CreateLane(ctx, timeline, lk.LaneCreate{Name: "Kingdoms", Rank: "a0", Size: lk.LaneLarge})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Documents.Update(ctx, timeline, lk.DocumentUpdate{CalendarID: lk.Some(calendar.ID)}); err != nil {
		t.Fatal(err)
	}
	d, err := c.Documents.Get(ctx, timeline)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(d.CalendarID == calendar.ID)
	for _, e := range [][2]string{
		{"Princess Bubblegum makes the first Candy Person", "200 AMW"},
		{"Finn is found in the woods", "Flame 14th, 1016 AMW"},
	} {
		if _, err := c.Timelines.CreateEvent(ctx, timeline, lk.EventCreate{LaneID: lane.ID, Type: lk.EventTypeEvent,
			Name: lk.Some(e[0]), FormattedStart: lk.Some(e[1])}); err != nil {
			t.Fatal(err)
		}
	}
	var formatted []string
	for event, err := range c.Timelines.AllEvents(ctx, timeline, lk.EventListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		sd := event.StartDate
		t.Log(event.FormattedStart, event.Name, "|", sd.Day, sd.MonthName, sd.Year, sd.Era.Abbr)
		formatted = append(formatted, event.FormattedStart)
	}
	if !slices.Equal(formatted, []string{"200 AMW", "Flame 14th, 1016 AMW"}) {
		t.Errorf("formatted starts = %q", formatted)
	}

	renamedSpec := spec
	renamedSpec.Name = sandbox.Name("The Ooo Reckoning")
	renamed, err := c.Calendars.Update(ctx, calendar.ID, renamedSpec, lk.IfMatch(version))
	if err != nil {
		t.Fatal(err)
	}
	version = renamed.Version
	if _, err := c.Calendars.Update(ctx, calendar.ID, spec, lk.IfMatch(calendar.Version)); errors.Is(err, lk.ErrVersionMismatch) {
		t.Logf("stale version refused: %v", err)
	} else {
		t.Errorf("stale update = %v, want ErrVersionMismatch", err)
	}
	// The guide catches a 400 ValidationError; the code is KIND_MISMATCH.
	if _, err := c.Calendars.Update(ctx, lk.CalendarGregorian, spec); errors.Is(err, lk.ErrKindMismatch) {
		t.Logf("built-in calendars cannot change: %v", err)
	} else {
		t.Errorf("built-in update = %v, want ErrKindMismatch", err)
	}

	if _, err := c.Documents.Update(ctx, timeline, lk.DocumentUpdate{CalendarID: lk.Some(lk.CalendarHarptos)}); err != nil {
		t.Fatal(err)
	}
	for event, err := range c.Timelines.AllEvents(ctx, timeline, lk.EventListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		t.Log(event.FormattedStart, event.Name)
	}
}
