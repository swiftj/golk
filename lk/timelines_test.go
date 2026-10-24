package lk

import (
	"context"
	"errors"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

const (
	laneJSON  = `{"id":"l1","name":"Jake","rank":"a1","size":"sm","style":{"color":"#f4a300"}}`
	eventJSON = `{"id":"e1","laneId":"l1","type":"event","name":"Adventuring out of the Tree Fort","rank":"a0",
"start":540000000,"end":543000000,"detail":1,"formattedStart":"1028 AD","formattedEnd":"1033 AD",
"startDate":{"year":1028,"month":1,"monthName":"January","day":1,"hour":0,"minute":0,"era":{"id":"ad","name":"Anno Domini","abbr":"AD"},"iso":"1028"},
"endDate":{"year":1033,"month":1,"monthName":"January","day":1,"hour":0,"minute":0,"era":{"id":"ad","name":"Anno Domini","abbr":"AD"},"iso":"1033"},
"color":"#0079CC","iconGlyph":"tree","uri":"lk://resources/r2","isHidden":true,"showOnCalendar":true}`
	pT = pD
)

func TestTimelines_Lanes(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-LANE-01 list is one page", reply: testutil.JSON(200, `{"data":[`+laneJSON+`,{"id":"l0","name":"Finn","rank":"a0","size":"lg"}]}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Timelines.Lanes(ctx, doc) },
			method: "GET", path: pT + "/lanes",
			check: func(t *testing.T, v any) {
				p := v.(*Page[Lane])
				if len(p.Items) != 2 || p.HasMore || p.NextCursor != "" {
					t.Fatalf("page = %+v", p)
				}
				l := p.Items[0]
				if l.ID != "l1" || l.Size != LaneSmall || l.Style == nil || l.Style.Color.Or("") != "#f4a300" || l.Style.Opacity.IsSet() {
					t.Errorf("lane = %+v style %+v", l, l.Style)
				}
				if p.Items[1].Style != nil || p.Items[1].Size != LaneLarge {
					t.Errorf("lane without style = %+v", p.Items[1])
				}
			}},
		{name: "C-LANE-01 get", reply: testutil.JSON(200, laneJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Timelines.Lane(ctx, doc, "l1") },
			method: "GET", path: pT + "/lanes/l1"},
		{name: "C-LANE-02 create sends required fields only", reply: testutil.JSON(201, laneJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.CreateLane(ctx, doc, LaneCreate{Name: "Finn", Rank: "a0", Size: LaneLarge})
			},
			method: "POST", path: pT + "/lanes", keyed: true, body: `{"name":"Finn","rank":"a0","size":"lg"}`},
		{name: "C-LANE-02 create with style and id", reply: testutil.JSON(201, laneJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.CreateLane(ctx, doc, LaneCreate{ID: Some("lanejake"), Name: "Jake", Rank: "a1", Size: LaneSmall,
					Style: Some(LaneStyle{Color: Some("#f4a300"), Opacity: Some(0.5)})})
			},
			method: "POST", path: pT + "/lanes", keyed: true,
			body: `{"id":"lanejake","name":"Jake","rank":"a1","size":"sm","style":{"color":"#f4a300","opacity":0.5}}`},
		{name: "C-LANE-02 update sends only set fields", reply: testutil.JSON(200, laneJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.UpdateLane(ctx, doc, "l1", LaneUpdate{Name: Some("Jake the Dog")})
			},
			method: "PATCH", path: pT + "/lanes/l1", body: `{"name":"Jake the Dog"}`},
		{name: "C-LANE-02 update size and style", reply: testutil.JSON(200, laneJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.UpdateLane(ctx, doc, "l1", LaneUpdate{Size: Some(LaneLarge), Style: Some(LaneStyle{ImageURL: Some("https://assets.example/l.png")})})
			},
			method: "PATCH", path: pT + "/lanes/l1", body: `{"size":"lg","style":{"imageUrl":"https://assets.example/l.png"}}`},
		{name: "C-LANE-02 delete", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Timelines.DeleteLane(ctx, doc, "l1") },
			method: "DELETE", path: pT + "/lanes/l1"},
	})
}

func TestTimelines_Events(t *testing.T) {
	runWiki(t, []wikiCase{
		{name: "C-EVENT-01 list first page without query", reply: testutil.JSON(200, `{"data":[`+eventJSON+`],"hasMore":true,"nextCursor":"c2"}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.Events(ctx, doc, EventListParams{})
			},
			method: "GET", path: pT + "/events",
			check: func(t *testing.T, v any) {
				p := v.(*Page[Event])
				if len(p.Items) != 1 || !p.HasMore || p.NextCursor != "c2" {
					t.Errorf("page = %+v", p)
				}
			}},
		{name: "C-EVENT-01 list with limit and cursor", reply: testutil.JSON(200, `{"data":[],"hasMore":false,"nextCursor":null}`),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.Events(ctx, doc, EventListParams{Limit: 50, Cursor: "c2"})
			},
			method: "GET", path: pT + "/events", query: "cursor=c2&limit=50"},
		{name: "C-EVENT-03 get reads every field", reply: testutil.JSON(200, eventJSON),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Timelines.Event(ctx, doc, "e1") },
			method: "GET", path: pT + "/events/e1",
			check: func(t *testing.T, v any) {
				e := v.(*Event)
				want := EventDate{Year: 1028, Month: 1, MonthName: "January", Day: 1, Era: EventEra{ID: "ad", Name: "Anno Domini", Abbr: "AD"}, ISO: "1028"}
				if e.StartDate != want {
					t.Errorf("StartDate = %+v", e.StartDate)
				}
				if e.EndDate == nil || e.EndDate.Year != 1033 || e.EndDate.ISO != "1033" {
					t.Errorf("EndDate = %+v", e.EndDate)
				}
				if e.Start != 540000000 || e.End.Or(0) != 543000000 || e.Detail != DetailYear || e.FormattedStart != "1028 AD" ||
					e.FormattedEnd != "1033 AD" || e.Type != EventTypeEvent || e.LaneID != "l1" || e.Rank != "a0" ||
					e.Color != "#0079CC" || e.IconGlyph != "tree" || e.URI != "lk://resources/r2" || !e.IsHidden || !e.ShowOnCalendar {
					t.Errorf("event = %+v", e)
				}
			}},
		{name: "C-EVENT-03 an event without end and an unknown type", reply: testutil.JSON(200,
			`{"id":"e2","laneId":"l1","type":"milestone","start":0,"startDate":{"year":1,"month":1,"monthName":"Candy","day":1,"hour":0,"minute":0,"era":{"id":"a","name":"After","abbr":"AW"}}}`),
			call:   func(ctx context.Context, c *Client) (any, error) { return c.Timelines.Event(ctx, doc, "e2") },
			method: "GET", path: pT + "/events/e2",
			check: func(t *testing.T, v any) {
				e := v.(*Event)
				if e.EndDate != nil || e.End.IsSet() || e.Type != "milestone" || e.Detail != 0 || e.StartDate.ISO != "" {
					t.Errorf("event = %+v", e)
				}
			}},
		{name: "C-EVENT-02 create with minutes", reply: testutil.JSON(201, eventJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.CreateEvent(ctx, doc, EventCreate{LaneID: "l1", Type: EventTypeEvent, Name: Some("Adventuring"),
					Start: Some(540000000.0), End: Some(543000000.0)})
			},
			method: "POST", path: pT + "/events", keyed: true,
			body: `{"end":543000000,"laneId":"l1","name":"Adventuring","start":540000000,"type":"event"}`},
		{name: "C-EVENT-02 create with formatted dates and every option", reply: testutil.JSON(201, eventJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.CreateEvent(ctx, doc, EventCreate{ID: Some("evfinn01"), LaneID: "l1", Type: EventTypeNote,
					FormattedStart: Some("Flame 14th, 1016 AMW"), FormattedEnd: Some("1017 AMW"), Detail: Some(DetailDay),
					Color: Some("#ff0000"), IconGlyph: Some("sword"), ImageURL: Some("https://assets.example/e.png"),
					URI: Some(""), IsHidden: Some(false), ShowOnCalendar: Some(true)})
			},
			method: "POST", path: pT + "/events", keyed: true,
			body: `{"color":"#ff0000","detail":3,"formattedEnd":"1017 AMW","formattedStart":"Flame 14th, 1016 AMW","iconGlyph":"sword","id":"evfinn01","imageUrl":"https://assets.example/e.png","isHidden":false,"laneId":"l1","showOnCalendar":true,"type":"note","uri":""}`},
		{name: "C-EVENT-02 update moves to another lane", reply: testutil.JSON(200, eventJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.UpdateEvent(ctx, doc, "e1", EventUpdate{LaneID: Some("l0"), Name: Some("Found in the woods by the Dogs")})
			},
			method: "PATCH", path: pT + "/events/e1", body: `{"laneId":"l0","name":"Found in the woods by the Dogs"}`},
		{name: "C-EVENT-02 empty update", reply: testutil.JSON(200, eventJSON),
			call: func(ctx context.Context, c *Client) (any, error) {
				return c.Timelines.UpdateEvent(ctx, doc, "e1", EventUpdate{})
			},
			method: "PATCH", path: pT + "/events/e1", body: `{}`},
		{name: "C-EVENT-02 delete", reply: noContent(),
			call:   func(ctx context.Context, c *Client) (any, error) { return nil, c.Timelines.DeleteEvent(ctx, doc, "e1") },
			method: "DELETE", path: pT + "/events/e1"},
	})
}

// Events are server-paged: AllEvents follows the cursor and keeps the
// limit; lanes come on one page.
func TestTimelines_C_EVENT_01_AllEventsFollowsCursor(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.JSON(200, `{"data":[`+eventJSON+`],"hasMore":true,"nextCursor":"c2"}`),
		testutil.JSON(200, `{"data":[`+eventJSON+`],"hasMore":false,"nextCursor":null}`),
	)
	c, _ := newTestClient(t, srv)
	n := 0
	for _, err := range c.Timelines.AllEvents(context.Background(), doc, EventListParams{Limit: 1}) {
		if err != nil {
			t.Fatal(err)
		}
		n++
	}
	reqs := srv.Requests()
	if n != 2 || len(reqs) != 2 || reqs[0].RawQuery != "limit=1" || reqs[1].RawQuery != "cursor=c2&limit=1" {
		t.Errorf("n = %d, queries %q %q", n, reqs[0].RawQuery, reqs[len(reqs)-1].RawQuery)
	}
}

func TestTimelines_C_LANE_01_LanesClaimingMoreAreInconsistent(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, `{"data":[`+laneJSON+`],"hasMore":true,"nextCursor":"x"}`))
	c, _ := newTestClient(t, srv)
	var err error
	for _, err = range c.Timelines.AllLanes(context.Background(), doc) {
		if err != nil {
			break
		}
	}
	if !errors.Is(err, ErrPaginationInconsistent) {
		t.Errorf("err = %v, want ErrPaginationInconsistent", err)
	}
}

// A-33: lane and event writes are unversioned, so IfMatch is refused
// before any request.
func TestTimelines_IfMatchRefused(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	calls := map[string]func() error{
		"CreateLane": func() error {
			_, err := c.Timelines.CreateLane(ctx, doc, LaneCreate{Name: "x", Rank: "a0", Size: LaneSmall}, IfMatch("v1"))
			return err
		},
		"UpdateLane": func() error {
			_, err := c.Timelines.UpdateLane(ctx, doc, "l1", LaneUpdate{}, IfMatch("v1"))
			return err
		},
		"DeleteLane": func() error { return c.Timelines.DeleteLane(ctx, doc, "l1", IfMatch("v1")) },
		"CreateEvent": func() error {
			_, err := c.Timelines.CreateEvent(ctx, doc, EventCreate{LaneID: "l1", Type: EventTypeEvent}, IfMatch("v1"))
			return err
		},
		"UpdateEvent": func() error {
			_, err := c.Timelines.UpdateEvent(ctx, doc, "e1", EventUpdate{}, IfMatch("v1"))
			return err
		},
		"DeleteEvent": func() error { return c.Timelines.DeleteEvent(ctx, doc, "e1", IfMatch("v1")) },
	}
	for name, call := range calls {
		if err := call(); !errors.Is(err, ErrIfMatchNotAllowed) {
			t.Errorf("%s: err = %v, want ErrIfMatchNotAllowed", name, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Errorf("%d requests sent", n)
	}
}
