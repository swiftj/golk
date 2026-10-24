package lk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/swiftj/golk/lk/internal/gen"
)

// CalendarsService manages the project's calendars. A timeline document
// names its calendar by ID (Document.CalendarID); the calendar decides what
// an event's minutes mean as a date.
type CalendarsService struct{ c *Client }

// Built-in calendar IDs. Built-in calendars cannot be changed or deleted;
// copy one with Calendars.Create to change it.
const (
	CalendarGregorian    = "gregorian"
	CalendarHarptos      = "harptos"
	CalendarEberron      = "eberron"
	CalendarExandria     = "exandria"
	CalendarGreyhawk     = "greyhawk"
	CalendarElderScrolls = "elder-scrolls"
)

// BuiltinCalendars returns the IDs of the built-in calendars, in the order
// the guide lists them.
func BuiltinCalendars() []string {
	return []string{CalendarGregorian, CalendarHarptos, CalendarEberron, CalendarExandria, CalendarGreyhawk, CalendarElderScrolls}
}

// IsBuiltinCalendar reports whether id names a built-in calendar.
func IsBuiltinCalendar(id string) bool { return slices.Contains(BuiltinCalendars(), id) }

// CalendarEraResetMode says how an era numbers its dates.
type CalendarEraResetMode string

// Era reset modes. A mode this SDK does not know is kept as is.
const (
	// EraResetNone keeps counting years from the calendar epoch. A negative
	// era always has this mode.
	EraResetNone CalendarEraResetMode = "none"
	// EraResetYear restarts year numbering at the era start.
	EraResetYear CalendarEraResetMode = "year"
	// EraResetCalendar restarts year, month and day numbering.
	EraResetCalendar CalendarEraResetMode = "calendar"
)

// CalendarSpec is a calendar: its months, leap days, weekdays, clock, eras,
// moons and date formats. Every field the spec defines is typed here.
//
// A spec is lossless: fields the server sends that this SDK does not model,
// at any level, are kept when it is decoded and written back when it is
// encoded, so reading a calendar and replacing it never drops content
// (SDK_CONTRACT.md C-CAL-03). A null optional field reads as unset.
type CalendarSpec struct {
	// ID is the calendar ID, 1–100 characters. On Create it is minted when
	// empty; on Update it must be empty or equal the calendar's ID.
	ID string `json:"id,omitempty"`
	// Name is 1 or more characters.
	Name        string           `json:"name"`
	Description Optional[string] `json:"description,omitzero"`
	// HasZeroYear is true when a year 0 exists; false makes year -1
	// directly precede year 1.
	HasZeroYear bool `json:"hasZeroYear"`
	// MaxMinutes is the latest minute the calendar represents.
	MaxMinutes float64 `json:"maxMinutes"`
	// Months run in order; at least one.
	Months []CalendarMonth `json:"months"`
	// LeapDays may be empty.
	LeapDays []CalendarLeapDay `json:"leapDays"`
	// Weekdays run in order; at least one.
	Weekdays []CalendarWeekday `json:"weekdays"`
	// EpochWeekday is the zero-based index into Weekdays of the epoch's
	// first day.
	EpochWeekday float64 `json:"epochWeekday"`
	// WeekResetsEachMonth starts every month on the first weekday.
	WeekResetsEachMonth Optional[bool] `json:"weekResetsEachMonth,omitzero"`
	HoursInDay          float64        `json:"hoursInDay"`
	MinutesInHour       float64        `json:"minutesInHour"`
	// NegativeEra is the era before the epoch; unset, the calendar starts
	// at the first positive era.
	NegativeEra Optional[CalendarEra] `json:"negativeEra,omitzero"`
	// PositiveEras are in ascending StartsAt order; at least one.
	PositiveEras []CalendarEra            `json:"positiveEras"`
	Moons        Optional[[]CalendarMoon] `json:"moons,omitzero"`
	Format       Optional[CalendarFormat] `json:"format,omitzero"`
	// HalfClock renders a 12-hour clock with AM and PM.
	HalfClock Optional[bool] `json:"halfClock,omitzero"`
	calExtras
}

// CalendarMonth is a month of a calendar.
type CalendarMonth struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Length is the days in the month, 1 or more.
	Length float64 `json:"length"`
	// Interval: the month occurs every Interval years (1 is every year).
	Interval float64 `json:"interval"`
	// Offset shifts which years a month with Interval above 1 occurs in.
	Offset float64 `json:"offset"`
	// IsIntercalary marks a festival or gap between regular months.
	IsIntercalary bool `json:"isIntercalary"`
	calExtras
}

// CalendarLeapDay is an extra day added to the end of one month in the
// years its Interval selects.
type CalendarLeapDay struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Month is the zero-based index into Months of the month it lengthens.
	Month float64 `json:"month"`
	// Interval selects years as comma-separated terms: "4" is every fourth
	// year and "400,!100,4" is the Gregorian rule.
	Interval string `json:"interval"`
	// Offset is the year offset applied to every Interval term.
	Offset float64 `json:"offset"`
	// Day, AddsWeekDay, Intercalary and WeekDay are stored but not read by
	// date arithmetic; the app writes 1, false, false and "".
	Day         float64 `json:"day"`
	AddsWeekDay bool    `json:"addsWeekDay"`
	Intercalary bool    `json:"intercalary"`
	WeekDay     string  `json:"weekDay"`
	calExtras
}

// CalendarWeekday is a day of the week.
type CalendarWeekday struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	calExtras
}

// CalendarEra is an era. A positive era runs from StartsAt to the next
// one's; the negative era runs from StartsAt (negative) to minute -1 and
// always has ResetMode EraResetNone.
type CalendarEra struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Abbr is rendered by the E format token; the server defaults it to AD.
	Abbr     Optional[string] `json:"abbr,omitzero"`
	HideAbbr Optional[bool]   `json:"hideAbbr,omitzero"`
	// StartsAt is the minute the era begins.
	StartsAt  int64                `json:"startsAt"`
	ResetMode CalendarEraResetMode `json:"resetMode"`
	calExtras
}

// CalendarMoon is a moon whose phases the calendar view shows.
type CalendarMoon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	// Phase is one full lunar cycle in minutes.
	Phase float64 `json:"phase"`
	// Shift offsets the cycle in minutes.
	Shift float64 `json:"shift"`
	// Color is a hex code; the server defaults it to #FFFFFF.
	Color Optional[string] `json:"color,omitzero"`
	calExtras
}

// CalendarFormat holds the templates for formatted dates, including events'
// FormattedStart and FormattedEnd. Tokens: YYYY year, E and EE era abbr and
// name, MMMM and MMM month name and abbreviation, M and MM month number, D
// and DD day, DDDD and DDD weekday, H and HH hour, m and mm minute; ^ after
// a number adds an ordinal suffix and [text] is literal.
type CalendarFormat struct {
	ID    string `json:"id"`
	Year  string `json:"year"`
	Month string `json:"month"`
	Day   string `json:"day"`
	Time  string `json:"time"`
	calExtras
}

// calExtras holds fields of a calendar object this SDK does not model, so
// they are written back unchanged. Callers cannot set them.
type calExtras struct {
	m map[string]json.RawMessage
}

// calFieldNames returns the JSON names t's exported fields declare.
func calFieldNames(t reflect.Type) map[string]bool {
	m := map[string]bool{}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() || f.Anonymous {
			continue
		}
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "" {
			name = f.Name
		}
		m[name] = true
	}
	return m
}

// decodeCal decodes data into v (a pointer to a method-free copy of a
// calendar struct) and keeps every field v does not declare in e.
//
// Known fields are matched by their exact name, as the server matches them:
// encoding/json alone would also read "Name" into the name field.
func decodeCal(data []byte, v any, e *calExtras) error {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	if all == nil {
		// null, as encoding/json treats it: no change.
		return nil
	}
	known := calFieldNames(reflect.TypeOf(v).Elem())
	own := map[string]json.RawMessage{}
	e.m = nil
	for k, raw := range all {
		if known[k] {
			own[k] = raw
			continue
		}
		if e.m == nil {
			e.m = map[string]json.RawMessage{}
		}
		e.m[k] = append(json.RawMessage(nil), raw...)
	}
	b, err := json.Marshal(own)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// encodeCal encodes v (a method-free copy of a calendar struct) with the
// extra fields last, in name order. An extra field never overrides a
// modelled one.
func encodeCal(v any, extra map[string]json.RawMessage) ([]byte, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return body, nil
	}
	if len(body) < 2 || body[0] != '{' {
		return nil, fmt.Errorf("lk: %T does not encode as an object", v)
	}
	known := calFieldNames(reflect.TypeOf(v).Elem())
	var b bytes.Buffer
	b.Write(body[:len(body)-1])
	n := len(body) > 2
	for _, k := range slices.Sorted(maps.Keys(extra)) {
		if known[k] {
			continue
		}
		if n {
			b.WriteByte(',')
		}
		name, _ := json.Marshal(k)
		b.Write(name)
		b.WriteByte(':')
		b.Write(extra[k])
		n = true
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// orEmpty makes a nil slice encode as [], since the spec's arrays are
// required and null is refused.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// MarshalJSON encodes the spec with the fields it was read with.
func (s CalendarSpec) MarshalJSON() ([]byte, error) {
	type plain CalendarSpec
	p := plain(s)
	p.Months, p.LeapDays, p.Weekdays, p.PositiveEras = orEmpty(p.Months), orEmpty(p.LeapDays), orEmpty(p.Weekdays), orEmpty(p.PositiveEras)
	return encodeCal(&p, s.m)
}

// UnmarshalJSON decodes a spec and keeps the fields it does not model.
func (s *CalendarSpec) UnmarshalJSON(data []byte) error {
	type plain CalendarSpec
	return decodeCal(data, (*plain)(s), &s.calExtras)
}

func (m CalendarMonth) MarshalJSON() ([]byte, error) {
	type plain CalendarMonth
	return encodeCal((*plain)(&m), m.m)
}

func (m *CalendarMonth) UnmarshalJSON(data []byte) error {
	type plain CalendarMonth
	return decodeCal(data, (*plain)(m), &m.calExtras)
}

func (d CalendarLeapDay) MarshalJSON() ([]byte, error) {
	type plain CalendarLeapDay
	return encodeCal((*plain)(&d), d.m)
}

func (d *CalendarLeapDay) UnmarshalJSON(data []byte) error {
	type plain CalendarLeapDay
	return decodeCal(data, (*plain)(d), &d.calExtras)
}

func (w CalendarWeekday) MarshalJSON() ([]byte, error) {
	type plain CalendarWeekday
	return encodeCal((*plain)(&w), w.m)
}

func (w *CalendarWeekday) UnmarshalJSON(data []byte) error {
	type plain CalendarWeekday
	return decodeCal(data, (*plain)(w), &w.calExtras)
}

func (e CalendarEra) MarshalJSON() ([]byte, error) {
	type plain CalendarEra
	return encodeCal((*plain)(&e), e.m)
}

func (e *CalendarEra) UnmarshalJSON(data []byte) error {
	type plain CalendarEra
	return decodeCal(data, (*plain)(e), &e.calExtras)
}

func (m CalendarMoon) MarshalJSON() ([]byte, error) {
	type plain CalendarMoon
	return encodeCal((*plain)(&m), m.m)
}

func (m *CalendarMoon) UnmarshalJSON(data []byte) error {
	type plain CalendarMoon
	return decodeCal(data, (*plain)(m), &m.calExtras)
}

func (f CalendarFormat) MarshalJSON() ([]byte, error) {
	type plain CalendarFormat
	return encodeCal((*plain)(&f), f.m)
}

func (f *CalendarFormat) UnmarshalJSON(data []byte) error {
	type plain CalendarFormat
	return decodeCal(data, (*plain)(f), &f.calExtras)
}

// Calendar is a calendar with its ID and version.
type Calendar struct {
	ID string
	// Version is the calendar version; pass it to IfMatch on Update or
	// Delete. Versions are opaque; compare them only for equality.
	Version string
	// Spec is the calendar's spec, ID included. Edit it and pass it to
	// Update to replace the calendar without dropping unmodelled fields.
	Spec CalendarSpec
}

// calendarWire receives a calendar: the spec plus "version", which is
// taken out so the spec does not keep it as an unmodelled field.
type calendarWire struct{ versionedBody }

func (w *calendarWire) calendar() (*Calendar, error) {
	var spec CalendarSpec
	if len(w.raw) > 0 {
		if err := json.Unmarshal(w.raw, &spec); err != nil {
			return nil, fmt.Errorf("lk: decoding calendar: %w", err)
		}
	}
	return &Calendar{ID: spec.ID, Version: w.version, Spec: spec}, nil
}

// List returns every calendar on one page: the project's custom calendars,
// then the built-in ones, each with its version.
func (s *CalendarsService) List(ctx context.Context, opts ...CallOption) (*Page[Calendar], error) {
	var w struct {
		Data    []calendarWire `json:"data"`
		HasMore bool           `json:"hasMore"`
	}
	if err := s.c.call(ctx, gen.OpListCalendars.Method, gen.OpListCalendars.Path, nil, request{out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Calendar, len(w.Data))
	for i := range w.Data {
		cal, err := w.Data[i].calendar()
		if err != nil {
			return nil, err
		}
		out[i] = *cal
	}
	return singlePage(out, w.HasMore), nil
}

// All iterates over every calendar, custom ones first.
func (s *CalendarsService) All(ctx context.Context, opts ...CallOption) iter.Seq2[Calendar, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Calendar], error) { return s.List(ctx, opts...) })
}

// Create creates a custom calendar from spec and returns it with its ID and
// version. An ID already used, or a built-in ID, fails with ErrDuplicateID;
// a spec that cannot render a date fails with ErrValidation.
func (s *CalendarsService) Create(ctx context.Context, spec CalendarSpec, opts ...CallOption) (*Calendar, error) {
	var w calendarWire
	if err := s.c.call(ctx, gen.OpCreateCalendar.Method, gen.OpCreateCalendar.Path, nil,
		request{body: spec, out: &w, readsVersion: true}, opts); err != nil {
		return nil, err
	}
	return w.calendar()
}

// Update replaces the whole spec of a custom calendar and returns it with
// its new version. It takes IfMatch: a calendar changed since that version
// fails with ErrVersionMismatch, and APIError.CurrentVersion holds the
// current one. A built-in calendar cannot be changed (ErrKindMismatch); an
// unknown ID fails with ErrNotFound.
func (s *CalendarsService) Update(ctx context.Context, id string, spec CalendarSpec, opts ...CallOption) (*Calendar, error) {
	var w calendarWire
	if err := s.c.call(ctx, gen.OpUpdateCalendar.Method, gen.OpUpdateCalendar.Path, map[string]string{"calendarId": id},
		request{body: spec, out: &w, versioned: true}, opts); err != nil {
		return nil, err
	}
	return w.calendar()
}

// Delete deletes a custom calendar. It takes IfMatch. A built-in calendar
// cannot be deleted (ErrKindMismatch).
func (s *CalendarsService) Delete(ctx context.Context, id string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteCalendar.Method, gen.OpDeleteCalendar.Path, map[string]string{"calendarId": id},
		request{versioned: true}, opts)
}
