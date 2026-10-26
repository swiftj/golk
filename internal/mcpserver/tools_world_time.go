package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/swiftj/golk/lk"
)

func (s *server) addTimelineTools(ms *mcp.Server, writes bool) {
	addTool(s, ms, &mcp.Tool{
		Name: "read_timeline",
		Description: "Read a timeline (time) document: its calendar, its lanes by rank, and one page of events in start order with formatted dates, lane, id, link and hidden flag. " +
			"Lanes and events marked [chained] come from the timelines it chains (its sources). Omit document_id for the resource's first timeline. " +
			"When next_cursor is returned, call again with it (and the same limit) for the next page of events.",
		Annotations: readOnly("Read timeline"),
	}, func(in ReadTimelineIn) string { return in.Project }, s.readTimeline)
	if !writes {
		return
	}
	addTool(s, ms, &mcp.Tool{
		Name: "set_timeline_lane",
		Description: "Create a lane on a timeline document, or update one when lane_id is given. Only the fields given are sent. " +
			"Create needs name, rank (a fractional index such as a0) and size (sm or lg), and turns a blank document into a timeline. " +
			"The lane as it will be is checked by the spoiler guard unless the document or resource is hidden.",
		Annotations: write("Set timeline lane", false, false),
	}, func(in SetLaneIn) string { return in.Project }, serial(s, s.setLane))
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_timeline_lane",
		Description: "Delete one of a timeline's own lanes AND every event on it. Use dry_run to see how many events would go.",
		Annotations: write("Delete timeline lane", true, false),
	}, func(in DeleteLaneIn) string { return in.Project }, serial(s, s.deleteLane))
	addTool(s, ms, &mcp.Tool{
		Name: "set_timeline_event",
		Description: "Create an event on a timeline document, or update one when event_id is given. Only the fields given are sent. " +
			"Create needs lane_id, type (event or note) and a start: formatted_start (a date in the timeline's calendar, such as \"1000\" or \"March 3rd, 2012\") or start in minutes; " +
			"give dates either formatted or in minutes, not both. The event as it will be (name, link, image, dates) is checked by the spoiler guard unless the document or resource is hidden; " +
			"a hidden event is checked too, as players may still see it.",
		Annotations: write("Set timeline event", false, false),
	}, func(in SetEventIn) string { return in.Project }, serial(s, s.setEvent))
	addTool(s, ms, &mcp.Tool{
		Name:        "delete_timeline_event",
		Description: "Delete one of a timeline's own events.",
		Annotations: write("Delete timeline event", true, false),
	}, func(in DeleteEventIn) string { return in.Project }, serial(s, s.deleteEvent))
}

// TimelineRefArg names a timeline document. DocumentID defaults to the
// resource's first time document.
type TimelineRefArg struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id,omitempty" jsonschema:"the timeline document; omit for the resource's first timeline"`
}

// timelineTarget is a timeline document with its resource.
type timelineTarget struct {
	r   *lk.Resource
	d   *lk.Document
	ref lk.DocRef
}

// hidden reports whether players can't see the timeline at all: the
// document or its resource is hidden (D-012: only an entity's own flag).
func (t timelineTarget) hidden() bool { return t.r.IsHidden || t.d.IsHidden }

func (t timelineTarget) describe() string {
	return fmt.Sprintf("timeline %q (%s) of %q (%s)", t.d.Name, t.d.ID, t.r.Name, t.r.ID)
}

// timeline resolves a timeline document. A document_id may also name a
// blank document when allowBlank is set (a lane create converts it).
func timeline(ctx context.Context, c *lk.Client, a TimelineRefArg, allowBlank bool) (timelineTarget, error) {
	var zero timelineTarget
	if a.ResourceID == "" {
		return zero, errors.New("resource_id is required")
	}
	r, err := c.Resources.Get(ctx, a.ResourceID)
	if err != nil {
		return zero, err
	}
	var d *lk.Document
	if a.DocumentID == "" {
		for i := range r.Documents {
			if r.Documents[i].Type == lk.DocumentTime {
				d = &r.Documents[i]
				break
			}
		}
		if d == nil {
			return zero, fmt.Errorf("resource %q (%s) has no timeline document; documents: %s. Hint: pass document_id (a blank document becomes a timeline with its first lane)", r.Name, r.ID, docList(r.Documents))
		}
	} else if d, err = findDoc(r, a.DocumentID); err != nil {
		return zero, err
	}
	if d.Type != lk.DocumentTime && (!allowBlank || d.Type != lk.DocumentBlank) {
		return zero, fmt.Errorf("document %q (%s) of %q is a %s document, not a timeline", d.Name, d.ID, r.Name, d.Type)
	}
	return timelineTarget{r: r, d: d, ref: lk.DocRef{ResourceID: r.ID, DocumentID: d.ID}}, nil
}

// ReadTimelineIn is read_timeline's input.
type ReadTimelineIn struct {
	ProjectArg
	TimelineRefArg
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous call, with the same limit"`
	Limit   int    `json:"limit,omitempty" jsonschema:"events per page, 1-100; omit for the server default"`
	LaneID  string `json:"lane_id,omitempty" jsonschema:"read just this lane, with every field; not with event_id"`
	EventID string `json:"event_id,omitempty" jsonschema:"read just this event, with every field; not with lane_id"`
}

// SourceOut is one chained timeline.
type SourceOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
}

// LaneOut is one lane.
type LaneOut struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Rank string `json:"rank"`
	Size string `json:"size"`
	// Chained is set for a lane of a chained timeline, with its Source.
	Chained bool       `json:"chained,omitempty"`
	Source  *SourceOut `json:"source,omitempty"`
}

// EventOut is one event.
type EventOut struct {
	ID             string   `json:"id"`
	LaneID         string   `json:"lane_id"`
	Type           string   `json:"type"`
	Name           string   `json:"name,omitempty"`
	FormattedStart string   `json:"formatted_start,omitempty"`
	FormattedEnd   string   `json:"formatted_end,omitempty"`
	Start          float64  `json:"start"`
	End            *float64 `json:"end,omitempty"`
	URI            string   `json:"uri,omitempty"`
	Hidden         bool     `json:"hidden"`
	Chained        bool     `json:"chained,omitempty"`
}

// ReadTimelineOut is read_timeline's output.
type ReadTimelineOut struct {
	ResourceID string      `json:"resource_id"`
	DocumentID string      `json:"document_id"`
	Name       string      `json:"name"`
	Hidden     bool        `json:"hidden"`
	CalendarID string      `json:"calendar_id,omitempty"`
	Sources    []SourceOut `json:"sources,omitempty"`
	Lanes      []LaneOut   `json:"lanes"`
	Events     []EventOut  `json:"events"`
	HasMore    bool        `json:"has_more"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

func eventOut(e lk.Event) EventOut {
	o := EventOut{ID: e.ID, LaneID: e.LaneID, Type: string(e.Type), Name: e.Name, FormattedStart: e.FormattedStart,
		FormattedEnd: e.FormattedEnd, Start: e.Start, URI: e.URI, Hidden: e.IsHidden}
	if v, ok := e.End.Get(); ok {
		o.End = &v
	}
	return o
}

func (s *server) readTimeline(ctx context.Context, c *lk.Client, in ReadTimelineIn) (toolResult[ReadTimelineOut], error) {
	var zero toolResult[ReadTimelineOut]
	if err := checkPageLimit(in.Limit); err != nil {
		return zero, err
	}
	t, err := timeline(ctx, c, in.TimelineRefArg, false)
	if err != nil {
		return zero, err
	}
	if in.LaneID != "" || in.EventID != "" {
		return s.readTimelineItem(ctx, c, t, in)
	}
	// Resources.Get leaves out a document's sources; Documents.Get has them.
	full, err := c.Documents.Get(ctx, t.ref)
	if err != nil {
		return zero, err
	}
	lanes, err := c.Timelines.Lanes(ctx, t.ref)
	if err != nil {
		return zero, err
	}
	// The listing includes the lanes of chained timelines. A lane is
	// chained when a source's own listing (with its chain) has it.
	chainedBy := map[string]lk.DocRef{}
	for _, src := range full.Sources {
		sl, err := c.Timelines.Lanes(ctx, src)
		if err != nil {
			return zero, fmt.Errorf("reading chained timeline %s/%s: %w", src.ResourceID, src.DocumentID, err)
		}
		for _, l := range sl.Items {
			if _, ok := chainedBy[l.ID]; !ok {
				chainedBy[l.ID] = src
			}
		}
	}
	page, err := c.Timelines.Events(ctx, t.ref, lk.EventListParams{Limit: in.Limit, Cursor: in.Cursor})
	if err != nil {
		return zero, err
	}
	ls := lanes.Items
	lk.SortByRank(ls)
	out := ReadTimelineOut{ResourceID: t.r.ID, DocumentID: t.d.ID, Name: full.Name, Hidden: t.hidden(), CalendarID: full.CalendarID,
		Lanes: []LaneOut{}, Events: []EventOut{}, HasMore: page.HasMore, NextCursor: page.NextCursor}
	for _, src := range full.Sources {
		out.Sources = append(out.Sources, SourceOut{ResourceID: src.ResourceID, DocumentID: src.DocumentID})
	}
	laneName := map[string]string{}
	var b strings.Builder
	fmt.Fprintf(&b, "Timeline %q (%s) of %q (%s)%s; calendar %s.\n", full.Name, full.ID, t.r.Name, t.r.ID,
		listFlags(flagIf(t.d.IsHidden, "hidden document"), flagIf(t.r.IsHidden, "hidden resource")), orDash(full.CalendarID))
	if len(full.Sources) > 0 {
		refs := make([]string, len(full.Sources))
		for i, src := range full.Sources {
			refs[i] = src.ResourceID + "/" + src.DocumentID
		}
		fmt.Fprintf(&b, "Chains (sources, in order): %s.\n", strings.Join(refs, ", "))
	}
	fmt.Fprintf(&b, "Lanes by rank (%d):\n", len(ls))
	for _, l := range ls {
		lo := LaneOut{ID: l.ID, Name: l.Name, Rank: l.Rank, Size: string(l.Size)}
		chained := ""
		if src, ok := chainedBy[l.ID]; ok {
			lo.Chained, lo.Source = true, &SourceOut{ResourceID: src.ResourceID, DocumentID: src.DocumentID}
			chained = "chained from " + src.ResourceID + "/" + src.DocumentID
		}
		laneName[l.ID] = l.Name
		out.Lanes = append(out.Lanes, lo)
		fmt.Fprintf(&b, "- %q (%s) rank %s, %s%s\n", l.Name, l.ID, l.Rank, l.Size, listFlags(chained))
	}
	b.WriteString("Events by start:\n")
	for _, e := range page.Items {
		eo := eventOut(e)
		_, eo.Chained = chainedBy[e.LaneID]
		out.Events = append(out.Events, eo)
		when := orDash(e.FormattedStart)
		if e.FormattedEnd != "" {
			when += " – " + e.FormattedEnd
		}
		link := ""
		if e.URI != "" {
			link = " → " + e.URI
		}
		lane := laneName[e.LaneID]
		if lane == "" {
			lane = e.LaneID
		}
		typ := ""
		if e.Type != lk.EventTypeEvent {
			typ = string(e.Type)
		}
		fmt.Fprintf(&b, "- %s: %q on %q (%s)%s%s\n", when, e.Name, lane, e.ID, link,
			listFlags(typ, flagIf(e.IsHidden, "hidden"), flagIf(eo.Chained, "chained")))
	}
	pagedFooter(&b, len(out.Events), "event(s)", page.NextCursor, page.HasMore)
	return toolResult[ReadTimelineOut]{out: out, text: b.String()}, nil
}

// LaneStyleIn is a lane's style; only the fields given are sent.
type LaneStyleIn struct {
	Color    *string  `json:"color,omitempty" jsonschema:"fill color of every event in the lane, #RRGGBB; replaces the events' colors and hides their images"`
	ImageURL *string  `json:"image_url,omitempty" jsonschema:"the lane's background image: the URL of an image asset of this project"`
	Opacity  *float64 `json:"opacity,omitempty"`
}

// SetLaneIn is set_timeline_lane's input.
type SetLaneIn struct {
	ProjectArg
	DryRunArg
	TimelineRefArg
	LaneID string       `json:"lane_id,omitempty" jsonschema:"the lane to update; omit to create one"`
	Name   *string      `json:"name,omitempty"`
	Rank   *string      `json:"rank,omitempty" jsonschema:"fractional index ordering lanes, such as a0; a0V sorts between a0 and a1"`
	Size   *string      `json:"size,omitempty" jsonschema:"sm or lg"`
	Style  *LaneStyleIn `json:"style,omitempty"`
	ID     string       `json:"id,omitempty" jsonschema:"a client-chosen cuid2 id for a new lane; omit to let the server choose"`
}

// LaneWriteOut is set_timeline_lane's output.
type LaneWriteOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	LaneID     string `json:"lane_id,omitempty"`
	Name       string `json:"name"`
	Rank       string `json:"rank"`
	Size       string `json:"size"`
	Created    bool   `json:"created,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func (st *LaneStyleIn) lk() lk.LaneStyle {
	var o lk.LaneStyle
	if st.Color != nil {
		o.Color = lk.Some(*st.Color)
	}
	if st.ImageURL != nil {
		o.ImageURL = lk.Some(*st.ImageURL)
	}
	if st.Opacity != nil {
		o.Opacity = lk.Some(*st.Opacity)
	}
	return o
}

func (s *server) setLane(ctx context.Context, c *lk.Client, in SetLaneIn) (toolResult[LaneWriteOut], error) {
	var zero toolResult[LaneWriteOut]
	if in.Size != nil && *in.Size != string(lk.LaneSmall) && *in.Size != string(lk.LaneLarge) {
		return zero, fmt.Errorf("size must be sm or lg, not %q", *in.Size)
	}
	create := in.LaneID == ""
	if !create && in.ID != "" {
		return zero, errors.New("id names a new lane; to update one use lane_id")
	}
	if create && (in.Name == nil || in.Rank == nil || in.Size == nil) {
		return zero, errors.New("creating a lane needs name, rank and size (pass lane_id to update one)")
	}
	if !create && in.Name == nil && in.Rank == nil && in.Size == nil && in.Style == nil {
		return zero, errors.New("nothing to change: give name, rank, size or style")
	}
	t, err := timeline(ctx, c, in.TimelineRefArg, create)
	if err != nil {
		return zero, err
	}
	// The lane as it will be: the current lane with the change on top.
	var after lk.Lane
	if !create {
		cur, err := c.Timelines.Lane(ctx, t.ref, in.LaneID)
		if err != nil {
			return zero, err
		}
		after = *cur
	}
	style := map[string]any{}
	if after.Style != nil {
		style["color"], style["imageUrl"] = after.Style.Color.Or(""), after.Style.ImageURL.Or("")
	}
	if in.Name != nil {
		after.Name = *in.Name
	}
	if in.Rank != nil {
		after.Rank = *in.Rank
	}
	if in.Size != nil {
		after.Size = lk.LaneSize(*in.Size)
	}
	if in.Style != nil {
		if in.Style.Color != nil {
			style["color"] = *in.Style.Color
		}
		if in.Style.ImageURL != nil {
			style["imageUrl"] = *in.Style.ImageURL
		}
	}
	shown, err := s.shownToPlayers(ctx, c, t)
	if err != nil {
		return zero, err
	}
	if shown {
		if err := s.check(map[string]any{"lane": map[string]any{"name": after.Name, "style": style}}); err != nil {
			return zero, err
		}
	}
	out := LaneWriteOut{ResourceID: t.r.ID, DocumentID: t.d.ID, LaneID: in.LaneID, Name: after.Name, Rank: after.Rank,
		Size: string(after.Size), Created: create, DryRun: in.DryRun}
	verb := fmt.Sprintf("update lane %q (%s)", after.Name, in.LaneID)
	if create {
		verb = fmt.Sprintf("create lane %q (rank %s, %s)", after.Name, after.Rank, after.Size)
		if t.d.Type == lk.DocumentBlank {
			verb += ", turning the blank document into a timeline"
		}
	}
	if in.DryRun {
		return toolResult[LaneWriteOut]{out: out, text: fmt.Sprintf("Dry run: would %s on %s. Nothing was written.", verb, t.describe())}, nil
	}
	var l *lk.Lane
	if create {
		lc := lk.LaneCreate{Name: *in.Name, Rank: *in.Rank, Size: lk.LaneSize(*in.Size), ID: optStr(in.ID)}
		if in.Style != nil {
			lc.Style = lk.Some(in.Style.lk())
		}
		l, err = c.Timelines.CreateLane(ctx, t.ref, lc)
	} else {
		var u lk.LaneUpdate
		if in.Name != nil {
			u.Name = lk.Some(*in.Name)
		}
		if in.Rank != nil {
			u.Rank = lk.Some(*in.Rank)
		}
		if in.Size != nil {
			u.Size = lk.Some(lk.LaneSize(*in.Size))
		}
		if in.Style != nil {
			u.Style = lk.Some(in.Style.lk())
		}
		l, err = c.Timelines.UpdateLane(ctx, t.ref, in.LaneID, u)
	}
	if err != nil {
		return zero, err
	}
	out.LaneID, out.Name, out.Rank, out.Size = l.ID, l.Name, l.Rank, string(l.Size)
	did := "Updated"
	if create {
		did = "Created"
	}
	return toolResult[LaneWriteOut]{out: out, text: fmt.Sprintf("%s lane %q (%s), rank %s, %s, on %s.", did, l.Name, l.ID, l.Rank, l.Size, t.describe())}, nil
}

// DeleteLaneIn is delete_timeline_lane's input.
type DeleteLaneIn struct {
	ProjectArg
	DryRunArg
	TimelineRefArg
	LaneID string `json:"lane_id"`
}

// DeleteLaneOut is delete_timeline_lane's output.
type DeleteLaneOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	LaneID     string `json:"lane_id"`
	Name       string `json:"name"`
	Events     int    `json:"events"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func (s *server) deleteLane(ctx context.Context, c *lk.Client, in DeleteLaneIn) (toolResult[DeleteLaneOut], error) {
	var zero toolResult[DeleteLaneOut]
	// No guard: deleting takes content away from players.
	if in.LaneID == "" {
		return zero, errors.New("lane_id is required")
	}
	t, err := timeline(ctx, c, in.TimelineRefArg, false)
	if err != nil {
		return zero, err
	}
	l, err := c.Timelines.Lane(ctx, t.ref, in.LaneID)
	if err != nil {
		return zero, err
	}
	n := 0
	for e, err := range c.Timelines.AllEvents(ctx, t.ref, lk.EventListParams{Limit: 100}) {
		if err != nil {
			return zero, err
		}
		if e.LaneID == l.ID {
			n++
		}
	}
	out := DeleteLaneOut{ResourceID: t.r.ID, DocumentID: t.d.ID, LaneID: l.ID, Name: l.Name, Events: n, DryRun: in.DryRun}
	what := fmt.Sprintf("lane %q (%s) and its %d event(s) from %s", l.Name, l.ID, n, t.describe())
	if in.DryRun {
		return toolResult[DeleteLaneOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s. Nothing was written.", what)}, nil
	}
	if err := c.Timelines.DeleteLane(ctx, t.ref, l.ID); err != nil {
		return zero, err
	}
	return toolResult[DeleteLaneOut]{out: out, text: fmt.Sprintf("Deleted %s.", what)}, nil
}

// SetEventIn is set_timeline_event's input.
type SetEventIn struct {
	ProjectArg
	DryRunArg
	TimelineRefArg
	EventID        string   `json:"event_id,omitempty" jsonschema:"the event to update; omit to create one"`
	LaneID         *string  `json:"lane_id,omitempty" jsonschema:"the lane; required to create, moves the event on update"`
	Type           *string  `json:"type,omitempty" jsonschema:"event or note; required to create, can't be changed"`
	Name           *string  `json:"name,omitempty"`
	FormattedStart *string  `json:"formatted_start,omitempty" jsonschema:"start date written in the timeline's calendar, such as 1000 or March 3rd, 2012"`
	FormattedEnd   *string  `json:"formatted_end,omitempty" jsonschema:"end date written in the timeline's calendar"`
	Start          *float64 `json:"start,omitempty" jsonschema:"start in minutes on the calendar's timeline; not with formatted dates"`
	End            *float64 `json:"end,omitempty" jsonschema:"end in minutes; not with formatted dates"`
	URI            *string  `json:"uri,omitempty" jsonschema:"link: lk://resources/<id>, lk://documents/<id> or lk://resources/<id>/docs/<id>; empty clears it"`
	Color          *string  `json:"color,omitempty" jsonschema:"fill color, #RRGGBB"`
	IconGlyph      *string  `json:"icon_glyph,omitempty" jsonschema:"a Font Awesome 6 icon name, lowercase with no fa- prefix, e.g. crown, chess-rook, dragon, skull, house; look names up with find_icon; empty clears it"`
	ImageURL       *string  `json:"image_url,omitempty" jsonschema:"the URL of an image asset of this project"`
	IsHidden       *bool    `json:"is_hidden,omitempty" jsonschema:"hide the event from players; it is still checked by the spoiler guard"`
	ShowOnCalendar *bool    `json:"show_on_calendar,omitempty"`
	Detail         *int     `json:"detail,omitempty" jsonschema:"the precision of the dates: 1 year, 2 month, 3 day, 4 time"`
	ID             string   `json:"id,omitempty" jsonschema:"a client-chosen cuid2 id for a new event; omit to let the server choose"`
}

// EventWriteOut is set_timeline_event's output.
type EventWriteOut struct {
	ResourceID string   `json:"resource_id"`
	DocumentID string   `json:"document_id"`
	Event      EventOut `json:"event"`
	Created    bool     `json:"created,omitempty"`
	DryRun     bool     `json:"dry_run,omitempty"`
}

func (s *server) setEvent(ctx context.Context, c *lk.Client, in SetEventIn) (toolResult[EventWriteOut], error) {
	var zero toolResult[EventWriteOut]
	if in.Detail != nil && (*in.Detail < 1 || *in.Detail > 4) {
		return zero, errors.New("detail is 1 (year), 2 (month), 3 (day) or 4 (time)")
	}
	normalizeGlyph(in.IconGlyph)
	if in.EventID != "" && in.ID != "" {
		return zero, errors.New("id names a new event; to update one use event_id")
	}
	formatted, minutes := in.FormattedStart != nil || in.FormattedEnd != nil, in.Start != nil || in.End != nil
	if formatted && minutes {
		return zero, errors.New("give dates either as formatted_start/formatted_end or as start/end in minutes, not both")
	}
	if in.Type != nil && *in.Type != string(lk.EventTypeEvent) && *in.Type != string(lk.EventTypeNote) {
		return zero, fmt.Errorf("type must be event or note, not %q", *in.Type)
	}
	create := in.EventID == ""
	if create {
		if in.LaneID == nil || in.Type == nil || (in.FormattedStart == nil && in.Start == nil) {
			return zero, errors.New("creating an event needs lane_id, type (event or note) and formatted_start or start (pass event_id to update one)")
		}
	} else {
		if in.Type != nil {
			return zero, errors.New("an event's type can't be changed; delete it and create a new one")
		}
		if in.LaneID == nil && in.Name == nil && !formatted && !minutes && in.URI == nil && in.Color == nil &&
			in.IconGlyph == nil && in.ImageURL == nil && in.IsHidden == nil && in.ShowOnCalendar == nil && in.Detail == nil {
			return zero, errors.New("nothing to change: give at least one field")
		}
	}
	t, err := timeline(ctx, c, in.TimelineRefArg, false)
	if err != nil {
		return zero, err
	}
	// The event as it will be: the current event with the change on top.
	var after lk.Event
	if !create {
		cur, err := c.Timelines.Event(ctx, t.ref, in.EventID)
		if err != nil {
			return zero, err
		}
		after = *cur
	} else {
		after.Type = lk.EventType(*in.Type)
	}
	if in.LaneID != nil {
		after.LaneID = *in.LaneID
	}
	if in.Name != nil {
		after.Name = *in.Name
	}
	if in.FormattedStart != nil {
		after.FormattedStart = *in.FormattedStart
	}
	if in.FormattedEnd != nil {
		after.FormattedEnd = *in.FormattedEnd
	}
	if in.Start != nil {
		after.Start = *in.Start
	}
	if in.End != nil {
		after.End = lk.Some(*in.End)
	}
	for _, f := range []struct {
		dst *string
		src *string
	}{{&after.URI, in.URI}, {&after.Color, in.Color}, {&after.IconGlyph, in.IconGlyph}, {&after.ImageURL, in.ImageURL}} {
		if f.src != nil {
			*f.dst = *f.src
		}
	}
	if in.IsHidden != nil {
		after.IsHidden = *in.IsHidden
	}
	if in.ShowOnCalendar != nil {
		after.ShowOnCalendar = *in.ShowOnCalendar
	}
	// A hidden event is exempt only once hidden events are confirmed
	// DM-only (X-GUARD-11).
	shown, err := s.shownToPlayers(ctx, c, t)
	if err != nil {
		return zero, err
	}
	if shown && len(shownEvents([]lk.Event{after})) > 0 {
		visible := map[string]any{"event": map[string]any{
			"name": after.Name, "uri": after.URI, "imageUrl": after.ImageURL, "iconGlyph": after.IconGlyph, "color": after.Color,
			"formattedStart": after.FormattedStart, "formattedEnd": after.FormattedEnd,
		}}
		if err := s.check(visible); err != nil {
			return zero, err
		}
	}
	out := EventWriteOut{ResourceID: t.r.ID, DocumentID: t.d.ID, Event: eventOut(after), Created: create, DryRun: in.DryRun}
	verb := fmt.Sprintf("update event %q (%s)", after.Name, in.EventID)
	if create {
		verb = fmt.Sprintf("create %s %q on lane %s", after.Type, after.Name, after.LaneID)
	}
	if in.DryRun {
		return toolResult[EventWriteOut]{out: out, text: fmt.Sprintf("Dry run: would %s on %s. Nothing was written.", verb, t.describe())}, nil
	}
	var e *lk.Event
	if create {
		ec := lk.EventCreate{LaneID: *in.LaneID, Type: lk.EventType(*in.Type)}
		setOpt(&ec.Name, in.Name)
		setOpt(&ec.FormattedStart, in.FormattedStart)
		setOpt(&ec.FormattedEnd, in.FormattedEnd)
		setOpt(&ec.Start, in.Start)
		setOpt(&ec.End, in.End)
		setOpt(&ec.URI, in.URI)
		setOpt(&ec.Color, in.Color)
		setOpt(&ec.IconGlyph, in.IconGlyph)
		setOpt(&ec.ImageURL, in.ImageURL)
		setOpt(&ec.IsHidden, in.IsHidden)
		setOpt(&ec.ShowOnCalendar, in.ShowOnCalendar)
		ec.ID = optStr(in.ID)
		if in.Detail != nil {
			ec.Detail = lk.Some(lk.EventDetail(*in.Detail))
		}
		e, err = c.Timelines.CreateEvent(ctx, t.ref, ec)
	} else {
		var u lk.EventUpdate
		setOpt(&u.LaneID, in.LaneID)
		setOpt(&u.Name, in.Name)
		setOpt(&u.FormattedStart, in.FormattedStart)
		setOpt(&u.FormattedEnd, in.FormattedEnd)
		setOpt(&u.Start, in.Start)
		setOpt(&u.End, in.End)
		setOpt(&u.URI, in.URI)
		setOpt(&u.Color, in.Color)
		setOpt(&u.IconGlyph, in.IconGlyph)
		setOpt(&u.ImageURL, in.ImageURL)
		setOpt(&u.IsHidden, in.IsHidden)
		setOpt(&u.ShowOnCalendar, in.ShowOnCalendar)
		if in.Detail != nil {
			u.Detail = lk.Some(lk.EventDetail(*in.Detail))
		}
		e, err = c.Timelines.UpdateEvent(ctx, t.ref, in.EventID, u)
	}
	if err != nil {
		return zero, err
	}
	out.Event = eventOut(*e)
	did := "Updated"
	if create {
		did = "Created"
	}
	when := orDash(e.FormattedStart)
	if e.FormattedEnd != "" {
		when += " – " + e.FormattedEnd
	}
	return toolResult[EventWriteOut]{out: out, text: fmt.Sprintf("%s %s %q (%s) at %s on lane %s of %s%s.", did, e.Type, e.Name, e.ID, when, e.LaneID,
		t.describe(), listFlags(flagIf(e.IsHidden, "hidden")))}, nil
}

// DeleteEventIn is delete_timeline_event's input.
type DeleteEventIn struct {
	ProjectArg
	DryRunArg
	TimelineRefArg
	EventID string `json:"event_id"`
}

// DeleteEventOut is delete_timeline_event's output.
type DeleteEventOut struct {
	ResourceID string `json:"resource_id"`
	DocumentID string `json:"document_id"`
	EventID    string `json:"event_id"`
	Name       string `json:"name"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

func (s *server) deleteEvent(ctx context.Context, c *lk.Client, in DeleteEventIn) (toolResult[DeleteEventOut], error) {
	var zero toolResult[DeleteEventOut]
	// No guard: deleting takes content away from players.
	if in.EventID == "" {
		return zero, errors.New("event_id is required")
	}
	t, err := timeline(ctx, c, in.TimelineRefArg, false)
	if err != nil {
		return zero, err
	}
	e, err := c.Timelines.Event(ctx, t.ref, in.EventID)
	if err != nil {
		return zero, err
	}
	out := DeleteEventOut{ResourceID: t.r.ID, DocumentID: t.d.ID, EventID: e.ID, Name: e.Name, DryRun: in.DryRun}
	what := fmt.Sprintf("event %q (%s, %s) from %s", e.Name, e.ID, orDash(e.FormattedStart), t.describe())
	if in.DryRun {
		return toolResult[DeleteEventOut]{out: out, text: fmt.Sprintf("Dry run: would delete %s. Nothing was written.", what)}, nil
	}
	if err := c.Timelines.DeleteEvent(ctx, t.ref, e.ID); err != nil {
		return zero, err
	}
	return toolResult[DeleteEventOut]{out: out, text: fmt.Sprintf("Deleted %s.", what)}, nil
}

// chainContent reads what the timelines in srcs would show on a timeline
// that chains them: each one's lanes and events (their own chains
// included, as their listings include them), for the guard.
func chainContent(ctx context.Context, c *lk.Client, srcs []lk.DocRef) ([]map[string]any, error) {
	var out []map[string]any
	for _, src := range srcs {
		lanes, err := c.Timelines.Lanes(ctx, src)
		if err != nil {
			return nil, fmt.Errorf("reading source timeline %s/%s to check it: %w", src.ResourceID, src.DocumentID, err)
		}
		var events []lk.Event
		for e, err := range c.Timelines.AllEvents(ctx, src, lk.EventListParams{Limit: 100}) {
			if err != nil {
				return nil, fmt.Errorf("reading source timeline %s/%s to check it: %w", src.ResourceID, src.DocumentID, err)
			}
			events = append(events, e)
		}
		out = append(out, map[string]any{"lanes": lanes.Items, "events": shownEvents(events)})
	}
	return out, nil
}

// chainedIntoVisible returns a visible timeline that shows target through
// its chain, directly or through other chained timelines, or nil. A
// visible timeline's sources are checked even when hidden (D-014), so a
// write to a hidden timeline that a visible one chains is player-visible
// too (red-team review, X-GUARD-08). LegendKeeper has no reverse lookup,
// so every timeline document of the project is read: one listing per
// resource holding a timeline. Only writes to hidden timelines pay this.
func chainedIntoVisible(ctx context.Context, c *lk.Client, target lk.DocRef) (*lk.DocRef, error) {
	type node struct {
		visible bool
		sources []lk.DocRef
	}
	graph := map[lk.DocRef]node{}
	var order []lk.DocRef
	for it, err := range c.Resources.All(ctx, lk.ResourceListParams{Limit: 100}) {
		if err != nil {
			return nil, fmt.Errorf("listing timelines to see which chain this one: %w", err)
		}
		if !slices.ContainsFunc(it.Documents, func(d lk.DocumentSummary) bool { return d.Type == lk.DocumentTime }) {
			continue
		}
		ds, err := c.Documents.List(ctx, it.ID)
		if err != nil {
			return nil, fmt.Errorf("listing timelines to see which chain this one: %w", err)
		}
		for _, d := range ds.Items {
			if d.Type != lk.DocumentTime {
				continue
			}
			ref := lk.DocRef{ResourceID: it.ID, DocumentID: d.ID}
			graph[ref] = node{visible: !it.IsHidden && !d.IsHidden, sources: d.Sources}
			order = append(order, ref)
		}
	}
	for _, ref := range order {
		if !graph[ref].visible || ref == target {
			continue
		}
		seen := map[lk.DocRef]bool{ref: true}
		stack := slices.Clone(graph[ref].sources)
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if cur == target {
				return &ref, nil
			}
			if seen[cur] {
				continue
			}
			seen[cur] = true
			stack = append(stack, graph[cur].sources...)
		}
	}
	return nil, nil
}

// shownToPlayers reports whether players can see timeline t: it is
// visible, or a visible timeline chains it (chainedIntoVisible). With the
// guard off nothing is read.
func (s *server) shownToPlayers(ctx context.Context, c *lk.Client, t timelineTarget) (bool, error) {
	if !t.hidden() {
		return true, nil
	}
	if s.cfg.GuardOff {
		return false, nil
	}
	via, err := chainedIntoVisible(ctx, c, t.ref)
	return via != nil, err
}

// readTimelineItem reads one lane or event of t with every field.
func (s *server) readTimelineItem(ctx context.Context, c *lk.Client, t timelineTarget, in ReadTimelineIn) (toolResult[ReadTimelineOut], error) {
	var zero toolResult[ReadTimelineOut]
	if in.LaneID != "" && in.EventID != "" {
		return zero, errors.New("give lane_id or event_id, not both")
	}
	out := ReadTimelineOut{ResourceID: t.ref.ResourceID, DocumentID: t.ref.DocumentID, Lanes: []LaneOut{}, Events: []EventOut{}}
	var what string
	var v any
	if in.LaneID != "" {
		l, err := c.Timelines.Lane(ctx, t.ref, in.LaneID)
		if err != nil {
			return zero, err
		}
		what, v = fmt.Sprintf("Lane %q (%s)", l.Name, l.ID), l
	} else {
		e, err := c.Timelines.Event(ctx, t.ref, in.EventID)
		if err != nil {
			return zero, err
		}
		what, v = fmt.Sprintf("Event %q (%s), %s to %s", e.Name, e.ID, orDash(e.FormattedStart), orDash(e.FormattedEnd)), e
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return zero, err
	}
	return toolResult[ReadTimelineOut]{out: out, text: fmt.Sprintf("%s of %s:\n%s", what, t.describe(), data)}, nil
}
