package lk

import (
	"context"
	"iter"
	"net/url"
	"strconv"

	"github.com/swiftj/golk/lk/internal/gen"
	"github.com/swiftj/golk/lk/internal/opt"
)

// TimelinesService manages a timeline document's lanes and events. A
// timeline is a document of type time; its first lane converts a blank
// document to one. Lane and event writes are unversioned: they take no
// IfMatch (DECISIONS.md D-014, A-33).
type TimelinesService struct{ c *Client }

// LaneSize is a lane's height. A size this SDK does not know is kept as is.
type LaneSize string

// Lane sizes.
const (
	LaneSmall LaneSize = "sm"
	LaneLarge LaneSize = "lg"
)

// LaneStyle is a lane's look. Only the fields that are set are sent.
type LaneStyle struct {
	// Color is the fill color of every event in the lane, as #RRGGBB. When
	// set, it replaces each event's color and hides the lane and event
	// images.
	Color Optional[string]
	// ImageURL is the lane's background image: the URL of an image asset of
	// this project.
	ImageURL Optional[string]
	Opacity  Optional[float64]
}

// Lane is one lane of a timeline. Events sit on lanes.
type Lane struct {
	ID   string
	Name string
	// Rank orders lanes; compare ranks as plain strings (SortByRank).
	Rank string
	Size LaneSize
	// Style is nil when the lane has none.
	Style *LaneStyle
}

func (l Lane) rank() string { return l.Rank }

// LaneCreate is the body of Timelines.CreateLane. Name, Rank and Size are
// required.
type LaneCreate struct {
	// ID is a client-chosen ID (see NewID).
	ID   Optional[string]
	Name string
	// Rank is a fractional index such as "a0"; "a0V" sorts between "a0"
	// and "a1".
	Rank  string
	Size  LaneSize
	Style Optional[LaneStyle]
}

// LaneUpdate is the body of Timelines.UpdateLane. Only the fields that are
// set are sent and changed.
type LaneUpdate struct {
	Name  Optional[string]
	Rank  Optional[string]
	Size  Optional[LaneSize]
	Style Optional[LaneStyle]
}

// EventType is the kind of an event. Events are created as EventTypeEvent
// (a point or range) or EventTypeNote (an annotation); reads may also carry
// the placeholder types. A type this SDK does not know is kept as is.
type EventType string

// Event types.
const (
	EventTypeEvent                EventType = "event"
	EventTypeNote                 EventType = "note"
	EventTypePlaceholder          EventType = "placeholder"
	EventTypeInvisiblePlaceholder EventType = "invisible-placeholder"
)

// EventDetail is the precision of an event's dates.
type EventDetail int

// Event date precisions. Zero means the server sent none, which it reads
// as day.
const (
	DetailYear  EventDetail = 1
	DetailMonth EventDetail = 2
	DetailDay   EventDetail = 3
	DetailTime  EventDetail = 4
)

// EventEra is the era a date falls in, as the calendar defines it.
type EventEra struct {
	ID   string
	Name string
	Abbr string
}

// EventDate is an event date resolved in the document's calendar.
type EventDate struct {
	// Year is counted within Era as the app displays it; always 0 or more.
	Year int
	// Month is 1 for the calendar's first month.
	Month     int
	MonthName string
	// Day is 1 for the month's first day.
	Day    int
	Hour   int
	Minute int
	Era    EventEra
	// ISO is set only on the Gregorian calendar: an ISO 8601 date cut to
	// the event's detail, such as "1016" or "1016-03-14".
	ISO string
}

// Event is one event of a timeline.
type Event struct {
	ID     string
	LaneID string
	Type   EventType
	Name   string
	// Rank orders events within a lane.
	Rank string
	// Start is the start in minutes on the calendar's timeline.
	Start float64
	// End is the end in minutes; unset for an event with no end.
	End Optional[float64]
	// Detail is the precision of the dates; zero means day.
	Detail EventDetail
	// FormattedStart and FormattedEnd are the dates as the app displays
	// them in the document's calendar.
	FormattedStart string
	FormattedEnd   string
	// StartDate is the start resolved in the document's calendar.
	StartDate EventDate
	// EndDate is nil for an event with no end.
	EndDate *EventDate
	// Color is the fill color; ignored while the lane sets a style color.
	Color     string
	IconGlyph string
	ImageURL  string
	// URI links a resource or document: lk://resources/<id>,
	// lk://documents/<id> or lk://resources/<id>/docs/<id>.
	URI            string
	IsHidden       bool
	ShowOnCalendar bool
}

// EventCreate is the body of Timelines.CreateEvent. LaneID, Type and a
// start (Start, or FormattedStart) are required; fields left unset are not
// sent.
type EventCreate struct {
	// ID is a client-chosen ID (see NewID).
	ID     Optional[string]
	LaneID string
	// Type is EventTypeEvent or EventTypeNote.
	Type EventType
	Name Optional[string]
	// Start and End are minutes on the calendar's timeline.
	Start Optional[float64]
	End   Optional[float64]
	// FormattedStart and FormattedEnd are dates written in the document's
	// calendar, such as "1000" or "March 3rd, 2012". The server parses them
	// into Start, End and Detail.
	FormattedStart Optional[string]
	FormattedEnd   Optional[string]
	Detail         Optional[EventDetail]
	Color          Optional[string]
	IconGlyph      Optional[string]
	// ImageURL is the URL of an image asset of this project.
	ImageURL Optional[string]
	// URI links a resource or document of this project; "" clears it.
	URI            Optional[string]
	IsHidden       Optional[bool]
	ShowOnCalendar Optional[bool]
}

// EventUpdate is the body of Timelines.UpdateEvent. Only the fields that
// are set are sent and changed; LaneID moves the event to another lane.
type EventUpdate struct {
	LaneID         Optional[string]
	Name           Optional[string]
	Start          Optional[float64]
	End            Optional[float64]
	FormattedStart Optional[string]
	FormattedEnd   Optional[string]
	Detail         Optional[EventDetail]
	Color          Optional[string]
	IconGlyph      Optional[string]
	ImageURL       Optional[string]
	// URI "" clears the link.
	URI            Optional[string]
	IsHidden       Optional[bool]
	ShowOnCalendar Optional[bool]
}

// EventListParams pages Timelines.Events.
type EventListParams struct {
	// Limit is the page size. Zero uses the server default.
	Limit int
	// Cursor starts at the page a previous Page.NextCursor names, with the
	// same Limit. Empty starts at the first page.
	Cursor string
}

func laneParams(doc DocRef, laneID string) map[string]string {
	p := doc.params()
	p["laneId"] = laneID
	return p
}

func eventParams(doc DocRef, eventID string) map[string]string {
	p := doc.params()
	p["eventId"] = eventID
	return p
}

// Lanes returns every lane on one page, including the lanes of chained
// timelines (the document's sources). Order them by Rank.
func (s *TimelinesService) Lanes(ctx context.Context, doc DocRef, opts ...CallOption) (*Page[Lane], error) {
	var w gen.LaneList
	if err := s.c.call(ctx, gen.OpListLanes.Method, gen.OpListLanes.Path, doc.params(), request{out: &w}, opts); err != nil {
		return nil, err
	}
	out := make([]Lane, len(w.Data))
	for i, l := range w.Data {
		out[i] = laneFromWire(l)
	}
	return singlePage(out, w.HasMore.Or(false)), nil
}

// AllLanes iterates over the timeline's lanes, chained ones included.
func (s *TimelinesService) AllLanes(ctx context.Context, doc DocRef, opts ...CallOption) iter.Seq2[Lane, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Lane], error) { return s.Lanes(ctx, doc, opts...) })
}

// Lane returns one of this document's own lanes (not a chained one).
func (s *TimelinesService) Lane(ctx context.Context, doc DocRef, laneID string, opts ...CallOption) (*Lane, error) {
	return s.lane(ctx, gen.OpGetLane, laneParams(doc, laneID), nil, opts)
}

// CreateLane creates a lane and returns it. On a blank document it makes
// the document a timeline.
func (s *TimelinesService) CreateLane(ctx context.Context, doc DocRef, l LaneCreate, opts ...CallOption) (*Lane, error) {
	body := gen.LaneCreateParams{ID: l.ID, Name: l.Name, Rank: l.Rank, Size: gen.LaneCreateParamsSize(l.Size)}
	if st, ok := l.Style.Get(); ok {
		body.Style = opt.Some(gen.LaneCreateParamsStyle{Color: st.Color, ImageURL: st.ImageURL, Opacity: st.Opacity})
	}
	return s.lane(ctx, gen.OpCreateLane, doc.params(), body, opts)
}

// UpdateLane changes only the fields set in u and returns the lane.
func (s *TimelinesService) UpdateLane(ctx context.Context, doc DocRef, laneID string, u LaneUpdate, opts ...CallOption) (*Lane, error) {
	body := gen.LaneUpdateParams{Name: u.Name, Rank: u.Rank, Size: convOpt[gen.LaneUpdateParamsSize](u.Size)}
	if st, ok := u.Style.Get(); ok {
		body.Style = opt.Some(gen.LaneUpdateParamsStyle{Color: st.Color, ImageURL: st.ImageURL, Opacity: st.Opacity})
	}
	return s.lane(ctx, gen.OpUpdateLane, laneParams(doc, laneID), body, opts)
}

// DeleteLane deletes the lane and every event on it.
func (s *TimelinesService) DeleteLane(ctx context.Context, doc DocRef, laneID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteLane.Method, gen.OpDeleteLane.Path, laneParams(doc, laneID), request{}, opts)
}

func (s *TimelinesService) lane(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (*Lane, error) {
	var w gen.Lane
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	l := laneFromWire(w)
	return &l, nil
}

// Events returns one page of events ordered by start, including the events
// of chained timelines: the first page, or the one p.Cursor names.
func (s *TimelinesService) Events(ctx context.Context, doc DocRef, p EventListParams, opts ...CallOption) (*Page[Event], error) {
	q := url.Values{}
	if p.Limit != 0 {
		q.Set("limit", strconv.Itoa(p.Limit))
	}
	pg := &pager[gen.EventList, Event]{
		c: s.c, tmpl: gen.OpListEvents.Path, params: doc.params(), query: q, opts: opts,
		items: func(w gen.EventList) ([]Event, opt.Optional[bool], opt.Nullable[string]) {
			out := make([]Event, len(w.Data))
			for i, e := range w.Data {
				out[i] = eventFromWire(e)
			}
			return out, w.HasMore, w.NextCursor
		},
	}
	return pg.page(ctx, p.Cursor)
}

// AllEvents iterates over the timeline's events in start order, fetching
// pages as it goes.
func (s *TimelinesService) AllEvents(ctx context.Context, doc DocRef, p EventListParams, opts ...CallOption) iter.Seq2[Event, error] {
	return iterate(ctx, func(ctx context.Context) (*Page[Event], error) { return s.Events(ctx, doc, p, opts...) })
}

// Event returns one of this document's own events (not a chained one).
func (s *TimelinesService) Event(ctx context.Context, doc DocRef, eventID string, opts ...CallOption) (*Event, error) {
	return s.event(ctx, gen.OpGetEvent, eventParams(doc, eventID), nil, opts)
}

// CreateEvent creates an event and returns it with its formatted dates.
func (s *TimelinesService) CreateEvent(ctx context.Context, doc DocRef, e EventCreate, opts ...CallOption) (*Event, error) {
	body := gen.EventCreateParams{
		ID: e.ID, LaneID: e.LaneID, Type: gen.EventCreateParamsType(e.Type), Name: e.Name,
		Start: e.Start, End: e.End, FormattedStart: e.FormattedStart, FormattedEnd: e.FormattedEnd,
		Detail: detailWire(e.Detail), Color: e.Color, IconGlyph: e.IconGlyph, ImageURL: e.ImageURL,
		URI: e.URI, IsHidden: e.IsHidden, ShowOnCalendar: e.ShowOnCalendar,
	}
	return s.event(ctx, gen.OpCreateEvent, doc.params(), body, opts)
}

// UpdateEvent changes only the fields set in u and returns the event.
func (s *TimelinesService) UpdateEvent(ctx context.Context, doc DocRef, eventID string, u EventUpdate, opts ...CallOption) (*Event, error) {
	body := gen.EventUpdateParams{
		LaneID: u.LaneID, Name: u.Name, Start: u.Start, End: u.End,
		FormattedStart: u.FormattedStart, FormattedEnd: u.FormattedEnd, Detail: detailWire(u.Detail),
		Color: u.Color, IconGlyph: u.IconGlyph, ImageURL: u.ImageURL, URI: u.URI,
		IsHidden: u.IsHidden, ShowOnCalendar: u.ShowOnCalendar,
	}
	return s.event(ctx, gen.OpUpdateEvent, eventParams(doc, eventID), body, opts)
}

// DeleteEvent deletes the event.
func (s *TimelinesService) DeleteEvent(ctx context.Context, doc DocRef, eventID string, opts ...CallOption) error {
	return s.c.call(ctx, gen.OpDeleteEvent.Method, gen.OpDeleteEvent.Path, eventParams(doc, eventID), request{}, opts)
}

func (s *TimelinesService) event(ctx context.Context, op gen.Op, params map[string]string, body any, opts []CallOption) (*Event, error) {
	var w gen.Event
	if err := s.c.call(ctx, op.Method, op.Path, params, request{body: body, out: &w}, opts); err != nil {
		return nil, err
	}
	e := eventFromWire(w)
	return &e, nil
}

func detailWire(d Optional[EventDetail]) opt.Optional[float64] {
	if v, ok := d.Get(); ok {
		return opt.Some(float64(v))
	}
	return opt.Optional[float64]{}
}

func laneFromWire(w gen.Lane) Lane {
	l := Lane{ID: w.ID, Name: w.Name, Rank: w.Rank, Size: LaneSize(w.Size)}
	if st, ok := w.Style.Get(); ok {
		l.Style = &LaneStyle{Color: st.Color, ImageURL: st.ImageURL, Opacity: st.Opacity}
	}
	return l
}

func eventDateFromWire(year, month int64, monthName string, day, hour, minute int64, era gen.EventDateEra, iso opt.Optional[string]) EventDate {
	return EventDate{
		Year: int(year), Month: int(month), MonthName: monthName, Day: int(day), Hour: int(hour), Minute: int(minute),
		Era: EventEra{ID: era.ID, Name: era.Name, Abbr: era.Abbr}, ISO: iso.Or(""),
	}
}

func eventFromWire(w gen.Event) Event {
	sd := w.StartDate
	e := Event{
		ID: w.ID, LaneID: w.LaneID, Type: EventType(w.Type), Name: w.Name.Or(""), Rank: w.Rank.Or(""),
		Start: w.Start, End: w.End, Detail: EventDetail(w.Detail.Or(0)),
		FormattedStart: w.FormattedStart.Or(""), FormattedEnd: w.FormattedEnd.Or(""),
		StartDate: eventDateFromWire(sd.Year, sd.Month, sd.MonthName, sd.Day, sd.Hour, sd.Minute, sd.Era, sd.Iso),
		Color:     w.Color.Or(""), IconGlyph: w.IconGlyph.Or(""), ImageURL: w.ImageURL.Or(""), URI: w.URI.Or(""),
		IsHidden: w.IsHidden.Or(false), ShowOnCalendar: w.ShowOnCalendar.Or(false),
	}
	if ed, ok := w.EndDate.Get(); ok {
		d := eventDateFromWire(ed.Year, ed.Month, ed.MonthName, ed.Day, ed.Hour, ed.Minute, ed.Era, ed.Iso)
		e.EndDate = &d
	}
	return e
}
