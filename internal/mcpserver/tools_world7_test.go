package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// X-ASSET-02: get_asset and list_assets show an asset's description.
func TestAssets_X_ASSET_02_ReadDescription(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(`{"id":"a1","name":"Castle","type":"IMAGE","folderId":"f1","description":"The east gate","url":"https://assets.example/c.png","width":8,"height":4}`))
	got := wtMustOK(t, h, "get_asset", map[string]any{"asset_id": "a1"})
	if !strings.Contains(got, "— The east gate") || !strings.Contains(got, "In folder f1.") {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.lk.Requests()[0], http.MethodGet, "/api/v3/projects/p1/assets/a1", "", "")
}

// X-ASSET-03, X-GUARD-16: create_asset_folder and update_asset send only
// what is given; a visible asset's name and description are checked, a
// hidden one's aren't.
func TestAssets_X_ASSET_03_X_GUARD_16_Writes(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusCreated, `{"id":"f2","name":"Maps","type":"FOLDER","folderId":"f1"}`))
	wtMustOK(t, h, "create_asset_folder", map[string]any{"name": "Maps", "folder_id": "f1"})
	wtReq(t, h.writes()[0], http.MethodPost, "/api/v3/projects/p1/assets", "", `{"name":"Maps","folderId":"f1"}`)

	h.lk.Enqueue(wtOK(`{"id":"a1","name":"Castle","type":"IMAGE","folderId":"f1"}`), noContentReply())
	wtMustOK(t, h, "update_asset", map[string]any{"asset_id": "a1", "description": "Gate", "move_to_root": true})
	wtReq(t, h.writes()[1], http.MethodPatch, "/api/v3/projects/p1/assets/a1", "", `{"description":"Gate","folderId":null}`)

	h = newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(`{"id":"a1","name":"Castle","type":"IMAGE"}`))
	wtRefused(t, h, wtMustFail(t, h, "update_asset", map[string]any{"asset_id": "a1", "name": "Zorblax lair"}, "spoiler guard"))
	h.lk.Enqueue(wtOK(`{"id":"a1","name":"Castle","type":"IMAGE","isHidden":true}`), noContentReply())
	wtMustOK(t, h, "update_asset", map[string]any{"asset_id": "a1", "name": "Zorblax lair"})
	wtRefused(t, newHarnessGuard(t, "Zorblax"), wtMustFail(t, newHarnessGuard(t, "Zorblax"), "create_asset_folder", map[string]any{"name": "Zorblax"}, "spoiler guard"))
}

func newHarnessGuard(t *testing.T, terms ...string) *harness {
	h := newHarness(t)
	h.guard.terms = terms
	return h
}

// X-TPL-02: the template tools send id and after/before.
func TestTemplates_X_TPL_02_IDAndPosition(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(ttRes), wtOK(ttBlocks), wtOK(wtNoProps), testutil.JSON(http.StatusCreated, ttCopy))
	wtMustOK(t, h, "save_as_template", map[string]any{"resource_id": "r1", "id": "tmpl1234", "before": "t0"})
	wtReq(t, h.writes()[0], http.MethodPost, wtPath+"/r1/save-as-template", "", `{"id":"tmpl1234","before":"t0"}`)
	wtMustFail(t, h, "create_from_template", map[string]any{"template_id": "t1", "after": "a", "before": "b"}, "not both")
}

// X-TL-03: lanes and events take a client id; events take detail; read_timeline
// reads one lane or event.
func TestTimeline_X_TL_03_IDsDetailAndItemReads(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(tlRes), testutil.JSON(http.StatusCreated, tlLane))
	wtMustOK(t, h, "set_timeline_lane", map[string]any{"resource_id": "r1", "name": "Jake", "rank": "a1", "size": "sm", "id": "lane1234"})
	wtReq(t, h.writes()[0], http.MethodPost, tlDocPath+"/lanes", "", `{"id":"lane1234","name":"Jake","rank":"a1","size":"sm"}`)

	h.lk.Enqueue(wtOK(tlRes), testutil.JSON(http.StatusCreated, tlEvent))
	wtMustOK(t, h, "set_timeline_event", map[string]any{"resource_id": "r1", "lane_id": "l1", "type": "event", "name": "Fort", "formatted_start": "1028", "detail": 1, "id": "even1234"})
	wtReq(t, h.writes()[1], http.MethodPost, tlDocPath+"/events", "", `{"id":"even1234","laneId":"l1","type":"event","name":"Fort","formattedStart":"1028","detail":1}`)
	wtMustFail(t, h, "set_timeline_event", map[string]any{"resource_id": "r1", "event_id": "e1", "detail": 9}, "detail is 1")

	h.lk.Enqueue(wtOK(tlRes), wtOK(tlEvent))
	got := wtMustOK(t, h, "read_timeline", map[string]any{"resource_id": "r1", "event_id": "e1"})
	if !strings.Contains(got, `Event "Tree Fort" (e1), 1028 AD`) {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.lk.Requests()[len(h.lk.Requests())-1], http.MethodGet, tlDocPath+"/events/e1", "", "")
}

// X-MAP-03: set_map_feature sends id, polygons and visibility_range;
// read_map reads one feature.
func TestMap_X_MAP_03_FieldsAndItemRead(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), testutil.JSON(http.StatusCreated, mtPin))
	wtMustOK(t, h, "set_map_feature", map[string]any{"resource_id": "r1", "document_id": "m1", "type": "region", "id": "feat1234",
		"polygons": [][][2]float64{{{1, 2}, {3, 4}, {5, 6}}}, "visibility_range": []float64{1, 5}})
	wtReq(t, h.writes()[0], http.MethodPost, mtPath+"/features", "", `{"id":"feat1234","type":"region","polygons":[[[1,2],[3,4],[5,6]]],"visibilityRange":[1,5]}`)

	h.lk.Enqueue(wtOK(mtRes), wtOK(mtDoc), wtOK(mtPin))
	got := wtMustOK(t, h, "read_map", map[string]any{"resource_id": "r1", "feature_id": "f9"})
	if !strings.Contains(got, `pin "Candy Castle" (f9)`) {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.lk.Requests()[len(h.lk.Requests())-1], http.MethodGet, mtPath+"/features/f9", "", "")
}

// X-BOARD-03: read_board with page_id lists one page's shapes.
func TestBoard_X_BOARD_03_PageShapes(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(btRes), wtOK(`{"data":[`+btShape+`],"version":"bv2"}`))
	got := wtMustOK(t, h, "read_board", map[string]any{"resource_id": "r1", "page_id": "page:page"})
	if !strings.Contains(got, `- geo shape:candy at (0, 0) text "Candy Kingdom"`) || !strings.Contains(got, "version bv2") {
		t.Errorf("text %q", got)
	}
	wtReq(t, h.lk.Requests()[1], http.MethodGet, btPath+"/board/shapes", "pageId=page%3Apage", "")
}
