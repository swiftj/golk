//go:build integration

package lk_test

// Phase 4 probes (SDK_CONTRACT.md A-33 to A-36). Run with
//
//	make test-integration ARGS='-run TestProbe4 -v ./lk/'
//
// and copy the observations into DECISIONS.md D-014.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

// TestProbe4_WorldVersionsAndHidden covers A-33 (ETags on feature, lane
// and event writes) and A-34 (isHidden round trip).
func TestProbe4_WorldVersionsAndHidden(t *testing.T) {
	p := newProber(t)
	_, mapDoc := p.page(sandbox.Name("Probe map"))
	pin, meta := p.must(http.MethodPost, mapDoc+"/features", map[string]any{"type": "pin", "name": "Probe pin", "pos": []float64{-10, 20}, "isHidden": true})
	t.Logf("A-33 feature create: status %d ETag %q version field %v", meta.Status, meta.ETag, pin["version"])
	t.Logf("A-34 feature create isHidden=true reads back: %v", pin["isHidden"])
	id, _ := pin["id"].(string)
	up, meta := p.must(http.MethodPatch, mapDoc+"/features/"+id, map[string]any{"isHidden": false})
	t.Logf("A-33 feature update: ETag %q; A-34 isHidden=false reads back: %v", meta.ETag, up["isHidden"])
	got, _ := p.must(http.MethodGet, mapDoc+"/features/"+id, nil)
	t.Logf("A-34 feature get isHidden: %v (absent means false)", got["isHidden"])

	_, timeDoc := p.page(sandbox.Name("Probe timeline"))
	lane, meta := p.must(http.MethodPost, timeDoc+"/lanes", map[string]any{"name": "Probe lane", "rank": "a0", "size": "lg"})
	t.Logf("A-33 lane create: status %d ETag %q version %v", meta.Status, meta.ETag, lane["version"])
	laneID, _ := lane["id"].(string)
	ev, meta := p.must(http.MethodPost, timeDoc+"/events", map[string]any{"laneId": laneID, "type": "event", "name": "Probe event", "formattedStart": "1000", "isHidden": true})
	t.Logf("A-33 event create: ETag %q version %v; A-34 isHidden reads back %v; formattedStart %v detail %v", meta.ETag, ev["version"], ev["isHidden"], ev["formattedStart"], ev["detail"])
	evID, _ := ev["id"].(string)
	ev2, meta := p.must(http.MethodPatch, timeDoc+"/events/"+evID, map[string]any{"isHidden": false})
	t.Logf("A-33 event update: ETag %q; A-34 isHidden=false reads back %v", meta.ETag, ev2["isHidden"])
}

// TestProbe4_CalendarVersion covers A-36.
func TestProbe4_CalendarVersion(t *testing.T) {
	p := newProber(t)
	month := func(id, name string, n int) map[string]any {
		return map[string]any{"id": id, "name": name, "isIntercalary": false, "length": n, "interval": 1, "offset": 0}
	}
	spec := map[string]any{
		"name": sandbox.Name("Probe calendar"), "hasZeroYear": false, "maxMinutes": 1_800_000_000,
		"months":   []any{month("candy", "Candy", 30), month("ice", "Ice", 30)},
		"leapDays": []any{}, "weekdays": []any{map[string]any{"id": "finn", "name": "Finnsday"}, map[string]any{"id": "jake", "name": "Jakesday"}},
		"epochWeekday": 0, "weekResetsEachMonth": false, "hoursInDay": 24, "minutesInHour": 60,
		"negativeEra":  map[string]any{"id": "before", "name": "Before", "abbr": "BW", "hideAbbr": false, "startsAt": -180_000_000, "resetMode": "none"},
		"positiveEras": []any{map[string]any{"id": "after", "name": "After", "abbr": "AW", "hideAbbr": false, "startsAt": 0, "resetMode": "none"}},
		"moons":        []any{},
		"format":       map[string]any{"id": "probe", "year": "YYYY E", "month": "MMMM YYYY E", "day": "MMMM D^, YYYY E", "time": "MMMM D^, YYYY E, HH:mm"},
		"halfClock":    false,
	}
	cal, meta := p.must(http.MethodPost, "/calendars", spec)
	id, _ := cal["id"].(string)
	version, _ := cal["version"].(string)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		_ = p.c.Do(ctx, lk.Request{Method: http.MethodDelete, Path: "/api/v3/projects/" + p.pid + "/calendars/" + id}, nil, lk.IfMatch(version))
	})
	t.Logf("A-36 create: body version %q, ETag %q", cal["version"], meta.ETag)
	spec["name"] = sandbox.Name("Probe calendar renamed")
	up, meta := p.must(http.MethodPut, "/calendars/"+id, spec, lk.IfMatch(version))
	t.Logf("A-36 update: body version %q, ETag %q, changed %v", up["version"], meta.ETag, up["version"] != version)
	stale := version
	version, _ = up["version"].(string)
	_, _, err := p.do(http.MethodPut, "/calendars/"+id, spec, lk.IfMatch(stale))
	e := apiErr(t, err)
	t.Logf("A-36 stale If-Match: %d %s %q CurrentVersion %q", e.Status, e.Code, e.Message, e.CurrentVersion)
	list, _ := p.must(http.MethodGet, "/calendars", nil)
	t.Logf("A-36 list shape keys: %v", keys(list))
}

// TestProbe4_TemplateHidden covers A-35.
func TestProbe4_TemplateHidden(t *testing.T) {
	p := newProber(t)
	parent := create(t, p.c, lk.ResourceCreate{Name: "Probe kingdom"})
	child := create(t, p.c, lk.ResourceCreate{Name: "Probe secret child", ParentID: lk.Some(parent.ID), IsHidden: lk.Some(true)})
	tpl, _ := p.must(http.MethodPost, "/resources/"+parent.ID+"/save-as-template", map[string]any{"name": sandbox.Name("Probe Template"), "includeChildren": true})
	tplID, _ := tpl["id"].(string)
	sandbox.Cleanup(t, tplID)
	t.Logf("A-35 template parentId %v isHidden %v", tpl["parentId"], tpl["isHidden"])
	kids, _ := p.must(http.MethodGet, "/resources?parentId="+tplID, nil)
	t.Logf("A-35 template children: %v (source child %s hidden)", show(kids["data"]), child.ID)
	cp, _ := p.must(http.MethodPost, "/resources/from-template", map[string]any{"templateId": tplID, "name": sandbox.Name("Probe realm"), "includeChildren": true})
	cpID, _ := cp["id"].(string)
	sandbox.Cleanup(t, cpID)
	ck, _ := p.must(http.MethodGet, "/resources?parentId="+cpID, nil)
	t.Logf("A-35 copy children: %v", show(ck["data"]))
}

func keys(m map[string]any) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
