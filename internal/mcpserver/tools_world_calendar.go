package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

// calendarNote is in every calendar write tool's description: a calendar
// has no hidden flag, and every timeline that uses it shows its names.
const calendarNote = " A calendar has no hidden flag, and its month, weekday, era and moon names and date formats show on every timeline that uses it, so every string of the spec is checked by the spoiler guard."

func (s *server) addCalendarTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name:        "list_calendars",
		Description: "List the project's calendars: id, name, version, and whether it is custom or built-in (built-ins can't be changed; copy one with create_calendar).",
		Annotations: readOnly("List calendars"),
	}, func(in ListCalendarsIn) string { return in.Project }, s.listCalendars)
	addTool(s, ms, &mcp.Tool{
		Name:        "get_calendar",
		Description: "Get one calendar's full spec as JSON, with the version to pass to replace_calendar or delete_calendar.",
		Annotations: readOnly("Get calendar"),
	}, func(in GetCalendarIn) string { return in.Project }, s.getCalendar)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name:        "create_calendar",
		Description: "Create a custom calendar from a spec (JSON object: name, months, leapDays, weekdays, eras, clock, moons, format; get_calendar shows one)." + calendarNote,
		Annotations: write("Create calendar", false, false),
	}, func(in CreateCalendarIn) string { return in.Project }, serial(s, s.createCalendar))
	addTool(s, ms, &mcp.Tool{
		Name: "replace_calendar",
		Description: "Replace a custom calendar's whole spec. Pass the version from get_calendar; the write fails if the calendar changed since. " +
			"Built-in calendars are refused." + calendarNote,
		Annotations: write("Replace calendar", true, true),
	}, func(in ReplaceCalendarIn) string { return in.Project }, serial(s, s.replaceCalendar))
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_calendar",
		Description: "Delete a custom calendar. Pass the version from get_calendar. Built-in calendars are refused. Timelines that use it lose their calendar.",
		Annotations: write("Delete calendar", true, false),
	}, func(in DeleteCalendarIn) string { return in.Project }, serial(s, s.deleteCalendar))
}

// ListCalendarsIn is list_calendars' input.
type ListCalendarsIn struct {
	ProjectArg
}

// CalendarOut is one calendar.
type CalendarOut struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
	BuiltIn bool   `json:"built_in"`
	DryRun  bool   `json:"dry_run,omitempty"`
}

// ListCalendarsOut is list_calendars' output.
type ListCalendarsOut struct {
	Calendars []CalendarOut `json:"calendars"`
}

func calendarOut(cal lk.Calendar) CalendarOut {
	return CalendarOut{ID: cal.ID, Name: cal.Spec.Name, Version: cal.Version, BuiltIn: lk.IsBuiltinCalendar(cal.ID)}
}

func builtinWord(b bool) string {
	if b {
		return "built-in"
	}
	return "custom"
}

func (s *server) listCalendars(ctx context.Context, c *lk.Client, _ ListCalendarsIn) (toolResult[ListCalendarsOut], error) {
	var zero toolResult[ListCalendarsOut]
	out := ListCalendarsOut{Calendars: []CalendarOut{}}
	var b strings.Builder
	for cal, err := range c.Calendars.All(ctx) {
		if err != nil {
			return zero, err
		}
		co := calendarOut(cal)
		out.Calendars = append(out.Calendars, co)
		fmt.Fprintf(&b, "- %q (%s) %s, version %s\n", co.Name, co.ID, builtinWord(co.BuiltIn), co.Version)
	}
	fmt.Fprintf(&b, "%d calendar(s).", len(out.Calendars))
	return toolResult[ListCalendarsOut]{out: out, text: b.String()}, nil
}

// findCalendar returns calendar id, from the one-page listing (the API has
// no get-one operation).
func findCalendar(ctx context.Context, c *lk.Client, id string) (*lk.Calendar, error) {
	if id == "" {
		return nil, errors.New("calendar_id is required")
	}
	for cal, err := range c.Calendars.All(ctx) {
		if err != nil {
			return nil, err
		}
		if cal.ID == id {
			return &cal, nil
		}
	}
	return nil, fmt.Errorf("no calendar %q. Hint: list_calendars lists the ids", id)
}

// GetCalendarIn is get_calendar's input.
type GetCalendarIn struct {
	ProjectArg
	CalendarID string `json:"calendar_id"`
}

func (s *server) getCalendar(ctx context.Context, c *lk.Client, in GetCalendarIn) (toolResult[CalendarOut], error) {
	var zero toolResult[CalendarOut]
	cal, err := findCalendar(ctx, c, in.CalendarID)
	if err != nil {
		return zero, err
	}
	spec, err := json.MarshalIndent(cal.Spec, "", "  ")
	if err != nil {
		return zero, err
	}
	co := calendarOut(*cal)
	return toolResult[CalendarOut]{out: co, text: fmt.Sprintf("Calendar %q (%s), %s, version %s. Spec:\n%s", co.Name, co.ID, builtinWord(co.BuiltIn), co.Version, spec)}, nil
}

// decodeSpec turns the spec argument into a CalendarSpec. The spec type is
// lossless: fields it doesn't model are kept and sent.
func decodeSpec(raw map[string]any) (lk.CalendarSpec, error) {
	var spec lk.CalendarSpec
	if raw == nil {
		return spec, errors.New("spec is required: a calendar spec object (get_calendar shows one)")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return spec, err
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return spec, fmt.Errorf("spec is not a calendar spec: %w", err)
	}
	if strings.TrimSpace(spec.Name) == "" {
		return spec, errors.New("spec.name is required")
	}
	return spec, nil
}

func refuseBuiltin(id string) error {
	if lk.IsBuiltinCalendar(id) {
		return fmt.Errorf("%q is a built-in calendar, which can't be changed or deleted; nothing was sent. Hint: copy it with create_calendar under a new id", id)
	}
	return nil
}

// calendarErr adds a calendar-specific hint to a version mismatch (the
// generic hint talks about pages).
func calendarErr(err error) error {
	if errors.Is(err, lk.ErrVersionMismatch) {
		return fmt.Errorf("%s\nHint: the calendar changed since that version was read; call get_calendar again and redo the change on the new version and spec", strings.SplitN(explain(err).Error(), "\nHint:", 2)[0])
	}
	return err
}

// CreateCalendarIn is create_calendar's input.
type CreateCalendarIn struct {
	ProjectArg
	DryRunArg
	Spec map[string]any `json:"spec" jsonschema:"the calendar spec as a JSON object; an id is optional (a built-in id is refused)"`
}

func (s *server) createCalendar(ctx context.Context, c *lk.Client, in CreateCalendarIn) (toolResult[CalendarOut], error) {
	var zero toolResult[CalendarOut]
	spec, err := decodeSpec(in.Spec)
	if err != nil {
		return zero, err
	}
	if err := refuseBuiltin(spec.ID); err != nil {
		return zero, err
	}
	// X-GUARD-09: every string of the spec.
	if err := s.check(spec); err != nil {
		return zero, err
	}
	out := CalendarOut{ID: spec.ID, Name: spec.Name, DryRun: in.DryRun}
	what := fmt.Sprintf("calendar %q with %d month(s), %d weekday(s) and %d era(s)", spec.Name, len(spec.Months), len(spec.Weekdays), len(spec.PositiveEras))
	if in.DryRun {
		return toolResult[CalendarOut]{out: out, text: fmt.Sprintf("Dry run: would create %s. Nothing was written.", what)}, nil
	}
	cal, err := c.Calendars.Create(ctx, spec)
	if err != nil {
		return zero, err
	}
	out = calendarOut(*cal)
	return toolResult[CalendarOut]{out: out, text: fmt.Sprintf("Created %s: id %s, version %s.", what, cal.ID, cal.Version)}, nil
}

// ReplaceCalendarIn is replace_calendar's input.
type ReplaceCalendarIn struct {
	ProjectArg
	DryRunArg
	CalendarID string         `json:"calendar_id"`
	Version    string         `json:"version" jsonschema:"the version from get_calendar or list_calendars"`
	Spec       map[string]any `json:"spec" jsonschema:"the whole new spec as a JSON object (start from get_calendar's)"`
}

func (s *server) replaceCalendar(ctx context.Context, c *lk.Client, in ReplaceCalendarIn) (toolResult[CalendarOut], error) {
	var zero toolResult[CalendarOut]
	if in.CalendarID == "" || in.Version == "" {
		return zero, errors.New("calendar_id and version are required")
	}
	if err := refuseBuiltin(in.CalendarID); err != nil {
		return zero, err
	}
	spec, err := decodeSpec(in.Spec)
	if err != nil {
		return zero, err
	}
	if spec.ID != "" && spec.ID != in.CalendarID {
		return zero, fmt.Errorf("spec.id %q differs from calendar_id %q", spec.ID, in.CalendarID)
	}
	if err := s.check(spec); err != nil {
		return zero, err
	}
	out := CalendarOut{ID: in.CalendarID, Name: spec.Name, Version: in.Version, DryRun: in.DryRun}
	if in.DryRun {
		cur, err := findCalendar(ctx, c, in.CalendarID)
		if err != nil {
			return zero, err
		}
		stale := ""
		if cur.Version != in.Version {
			stale = fmt.Sprintf(" Note: the calendar is now at version %s, so the real write would fail; call get_calendar again.", cur.Version)
		}
		return toolResult[CalendarOut]{out: out, text: fmt.Sprintf("Dry run: would replace calendar %q (%s) at version %s with the spec named %q.%s Nothing was written.",
			cur.Spec.Name, cur.ID, in.Version, spec.Name, stale)}, nil
	}
	cal, err := c.Calendars.Update(ctx, in.CalendarID, spec, lk.IfMatch(in.Version))
	if err != nil {
		return zero, calendarErr(err)
	}
	out = calendarOut(*cal)
	return toolResult[CalendarOut]{out: out, text: fmt.Sprintf("Replaced calendar %q (%s); new version %s. Every timeline that uses it shows the new spec.", cal.Spec.Name, cal.ID, cal.Version)}, nil
}

// DeleteCalendarIn is delete_calendar's input.
type DeleteCalendarIn struct {
	ProjectArg
	DryRunArg
	CalendarID string `json:"calendar_id"`
	Version    string `json:"version" jsonschema:"the version from get_calendar or list_calendars"`
}

func (s *server) deleteCalendar(ctx context.Context, c *lk.Client, in DeleteCalendarIn) (toolResult[CalendarOut], error) {
	var zero toolResult[CalendarOut]
	// No guard: deleting shows players nothing.
	if in.CalendarID == "" || in.Version == "" {
		return zero, errors.New("calendar_id and version are required")
	}
	if err := refuseBuiltin(in.CalendarID); err != nil {
		return zero, err
	}
	out := CalendarOut{ID: in.CalendarID, Version: in.Version, DryRun: in.DryRun}
	if in.DryRun {
		cur, err := findCalendar(ctx, c, in.CalendarID)
		if err != nil {
			return zero, err
		}
		out.Name = cur.Spec.Name
		stale := ""
		if cur.Version != in.Version {
			stale = fmt.Sprintf(" Note: the calendar is now at version %s, so the real delete would fail.", cur.Version)
		}
		return toolResult[CalendarOut]{out: out, text: fmt.Sprintf("Dry run: would delete calendar %q (%s) at version %s.%s Nothing was written.", cur.Spec.Name, cur.ID, in.Version, stale)}, nil
	}
	if err := c.Calendars.Delete(ctx, in.CalendarID, lk.IfMatch(in.Version)); err != nil {
		return zero, calendarErr(err)
	}
	return toolResult[CalendarOut]{out: out, text: fmt.Sprintf("Deleted calendar %s (version %s).", in.CalendarID, in.Version)}, nil
}
