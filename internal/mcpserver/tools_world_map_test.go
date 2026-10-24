package mcpserver

import (
	"net/http"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

// Fixtures of the map tool tests. All names and terms are made up.
const (
	mtRes = `{"id":"r1","name":"Ooo","isHidden":false,"documents":[
{"id":"d1","name":"Main","type":"page","rank":"a","isHidden":false},
{"id":"m1","name":"Land of Ooo","type":"map","rank":"b","isHidden":false},
{"id":"m2","name":"DM Map","type":"map","rank":"c","isHidden":true},
{"id":"b1","name":"Empty","type":"blank","rank":"d","isHidden":false}]}`
	mtResHidden = `{"id":"r1","name":"Ooo","isHidden":true,"documents":[
{"id":"m1","name":"Land of Ooo","type":"map","rank":"b","isHidden":false}]}`
	// mtDoc is m1 with a 512x256 image: bounds [[-128,0],[0,256]].
	mtDoc = `{"id":"m1","name":"Land of Ooo","type":"map","rank":"b","isHidden":false,
"mapImage":{"assetId":"a1","url":"https://assets.example/ooo.png","width":512,"height":256,"bounds":[[-128,0],[0,256]]}}`
	mtDocNoImage = `{"id":"m1","name":"Land of Ooo","type":"map","rank":"b","isHidden":false}`
	mtFeatures   = `{"data":[
{"id":"f1","type":"pin","name":"Candy Castle","pos":[-64,128],"parentId":"fold1","uri":"lk://resources/r2"},
{"id":"fold1","type":"folder","name":"Homes"},
{"id":"f2","type":"pin","name":"Secret Cave","pos":[-1,2],"isHidden":true}],"hasMore":false}`
	mtPin   = `{"id":"f9","type":"pin","name":"Candy Castle","pos":[-64,128]}`
	mtAsset = `{"id":"a1","name":"Ooo map","type":"IMAGE","url":"https://assets.example/ooo.png","width":512,"height":256}`
	mtPath  = "/api/v3/projects/p1/resources/r1/docs/m1"
)

func TestReadMap_X_MAP_01_ImageAndFeatures(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtDoc), wtOK(mtFeatures))
	r := h.call("read_map", map[string]any{"resource_id": "r1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	reqs := h.lk.Requests()
	if len(reqs) != 3 {
		t.Fatalf("requests %+v", reqs)
	}
	wtReq(t, reqs[1], http.MethodGet, mtPath, "", "")
	wtReq(t, reqs[2], http.MethodGet, mtPath+"/features", "", "")
	got := text(r)
	for _, want := range []string{
		"image https://assets.example/ooo.png, 512x256, bounds [[-128, 0], [0, 256]]",
		`- pin "Candy Castle" (f1) pos [-64, 128] folder fold1 link lk://resources/r2`,
		`- folder "Homes" (fold1)`,
		`- pin "Secret Cave" (f2) pos [-1, 2] [hidden]`,
		"3 feature(s).",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("text lacks %q:\n%s", want, got)
		}
	}
	var out ReadMapOut
	structured(t, r, &out)
	if out.DocumentID != "m1" || out.Features != 3 || out.Image == nil || out.Image.Width != 512 || out.Image.Bounds[0][0] != -128 {
		t.Errorf("out = %+v", out)
	}
}

func TestReadMap_X_MAP_01_TypeFilterAndNoImage(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtDocNoImage), wtOK(`{"data":[],"hasMore":false}`))
	r := h.call("read_map", map[string]any{"resource_id": "r1", "document_id": "m1", "type": "pin"})
	if r.IsError || !strings.Contains(text(r), "no image") || !strings.Contains(text(r), "0 pin feature(s).") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
	wtReq(t, h.lk.Requests()[2], http.MethodGet, mtPath+"/features", "type=pin", "")
}

func TestReadMap_X_MAP_01_RefusesNonMap(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes))
	r := h.call("read_map", map[string]any{"resource_id": "r1", "document_id": "d1"})
	if !r.IsError || !strings.Contains(text(r), "is a page document, not a map") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
}

func TestSetMapFeature_X_MAP_02_CreateByPixel(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtDoc), testutil.JSON(http.StatusCreated, mtPin))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "document_id": "m1", "type": "pin",
		"name": "Candy Castle", "pixel": []float64{256, 128}, "style": map[string]any{"icon_glyph": "castle", "icon_shape": "shield"}})
	if r.IsError {
		t.Fatal(text(r))
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	// pixel (256, 128) of 512x256 over [[-128,0],[0,256]] is [-64, 128].
	wtReq(t, w[0], http.MethodPost, mtPath+"/features", "", `{"type":"pin","name":"Candy Castle","pos":[-64,128],"iconGlyph":"castle","iconShape":"shield"}`)
	var out FeatureOut
	structured(t, r, &out)
	if out.FeatureID != "f9" || !out.Created || out.Pos == nil || *out.Pos != [2]float64{-64, 128} {
		t.Errorf("out = %+v", out)
	}
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], "Land of Ooo") || !strings.Contains(h.guard.checked[0], "Candy Castle") {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestSetMapFeature_X_MAP_02_PosAndPixelRefused(t *testing.T) {
	h := newHarness(t)
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "type": "pin", "name": "X", "pos": []float64{1, 2}, "pixel": []float64{3, 4}})
	if !r.IsError || !strings.Contains(text(r), "pos or pixel, not both") || len(h.lk.Requests()) != 0 {
		t.Fatalf("%v: %s; requests %+v", r.IsError, text(r), h.lk.Requests())
	}
}

func TestSetMapFeature_X_MAP_02_PixelNeedsImage(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtDocNoImage))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "type": "pin", "name": "X", "pixel": []float64{3, 4}})
	if !r.IsError || !strings.Contains(text(r), "has no image") || len(h.writes()) != 0 {
		t.Fatalf("%v: %s; writes %+v", r.IsError, text(r), h.writes())
	}
}

func TestSetMapFeature_X_MAP_02_UpdateSendsOnlyGivenFields(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(`{"id":"f1","type":"pin","name":"Candy Castle","pos":[-64,128]}`), wtOK(`{"id":"f1","type":"pin","name":"Candy Castle","pos":[-64,128],"iconColor":"#ff0000"}`))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "feature_id": "f1",
		"style": map[string]any{"icon_color": "#ff0000", "label_size": "lg", "fill_opacity": 0}, "uri": ""})
	if r.IsError {
		t.Fatal(text(r))
	}
	reqs := h.lk.Requests()
	wtReq(t, reqs[1], http.MethodGet, mtPath+"/features/f1", "", "")
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPatch, mtPath+"/features/f1", "", `{"iconColor":"#ff0000","labelSize":"lg","fillOpacity":0,"uri":""}`)
}

// The guard sees the feature as it will be: an update that changes only
// its colour is refused while its current name holds a term.
func TestSetMapFeature_X_GUARD_08_UpdateChecksTheWholeFeature(t *testing.T) {
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(mtRes), wtOK(`{"id":"f1","type":"pin","name":"Zorblax Spire","pos":[1,2]}`))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "feature_id": "f1", "style": map[string]any{"icon_color": "#ff0000"}})
	wtRefused(t, h, text(r))
}

func TestSetMapFeature_X_GUARD_08_RefusalSendsNothing(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"name":     {"name": "Zorblax Spire"},
		"uri":      {"name": "Spire", "uri": "lk://resources/zorblax"},
		"pinImage": {"name": "Spire", "style": map[string]any{"pin_image": map[string]any{"url": "https://assets.example/zorblax.png", "width": 8, "height": 8}}},
		// X-GUARD-11: a hidden feature is checked until hidden pins are
		// confirmed invisible to players.
		"hidden": {"name": "Zorblax Spire", "is_hidden": true},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.guard.terms = []string{"Zorblax", "zorblax"}
			h.lk.Enqueue(wtOK(mtRes))
			args["resource_id"], args["type"], args["pos"] = "r1", "pin", []float64{1, 2}
			wtRefused(t, h, text(h.call("set_map_feature", args)))
		})
	}
}

func TestSetMapFeature_X_GUARD_11_HiddenFeatureIsChecked(t *testing.T) {
	if hiddenFeaturesConfirmed {
		t.Skip("hidden features are DM-only")
	}
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(mtRes))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "type": "pin", "name": "Zorblax Spire", "is_hidden": true, "pos": []float64{1, 2}})
	wtRefused(t, h, text(r))
	// An update hiding a feature already named with a term is refused too.
	h.lk.Enqueue(wtOK(mtRes), wtOK(`{"id":"f2","type":"pin","name":"Zorblax Spire","isHidden":false}`))
	r = h.call("set_map_feature", map[string]any{"resource_id": "r1", "feature_id": "f2", "is_hidden": true})
	wtRefused(t, h, text(r))
}

func TestSetMapFeature_X_GUARD_08_RealGuardChecksMapName(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(strings.Replace(mtRes, "Land of Ooo", "Zorblax Land", 1)))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "type": "pin", "name": "Fine", "pos": []float64{1, 2}})
	wtRefused(t, h, text(r))
	// Fine names on a fine map pass the real guard.
	h.lk.Enqueue(wtOK(mtRes), testutil.JSON(http.StatusCreated, mtPin))
	r = h.call("set_map_feature", map[string]any{"resource_id": "r1", "type": "pin", "name": "Candy Castle", "pos": []float64{-64, 128}})
	if r.IsError {
		t.Fatal(text(r))
	}
}

func TestSetMapFeature_X_GUARD_08_HiddenTargetSkipsGuard(t *testing.T) {
	for name, c := range map[string]struct{ res, doc string }{
		"hidden document": {mtRes, "m2"},
		"hidden resource": {mtResHidden, "m1"},
	} {
		t.Run(name, func(t *testing.T) {
			h := rgHarness(t, "Zorblax")
			path := "/api/v3/projects/p1/resources/r1/docs/" + c.doc + "/features"
			h.lk.Enqueue(wtOK(c.res), testutil.JSON(http.StatusCreated, `{"id":"f9","type":"pin","name":"Zorblax Lair"}`))
			r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "document_id": c.doc, "type": "pin", "name": "Zorblax Lair", "pos": []float64{1, 2}})
			if r.IsError {
				t.Fatal(text(r))
			}
			w := h.writes()
			if len(w) != 1 {
				t.Fatalf("writes %+v", w)
			}
			wtReq(t, w[0], http.MethodPost, path, "", `{"type":"pin","name":"Zorblax Lair","pos":[1,2]}`)
		})
	}
	h := newHarness(t)
	h.guard.terms = []string{"Zorblax"}
	h.lk.Enqueue(wtOK(mtRes), testutil.JSON(http.StatusCreated, `{"id":"f9"}`))
	if r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "document_id": "m2", "type": "pin", "name": "Zorblax"}); r.IsError {
		t.Fatal(text(r))
	}
	if len(h.guard.checked) != 0 {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestSetMapFeature_X_TOOL_04_DryRunSendsNothing(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtDoc))
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "type": "pin", "name": "Candy Castle", "pixel": []float64{256, 128}, "dry_run": true})
	if r.IsError || !strings.Contains(text(r), "Dry run: would create pin \"Candy Castle\"") || !strings.Contains(text(r), "at pos [-64, 128]") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
	if len(h.writes()) != 0 {
		t.Errorf("dry run sent %+v", h.writes())
	}
	if len(h.guard.checked) != 1 {
		t.Errorf("dry run skipped the guard: %v", h.guard.checked)
	}
}

func TestSetMapFeature_CreateNeedsType(t *testing.T) {
	h := newHarness(t)
	r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "name": "X"})
	if !r.IsError || !strings.Contains(text(r), "type is required") || len(h.lk.Requests()) != 0 {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
}

func TestSetMapFeature_BlankDocumentBecomesMap(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), testutil.JSON(http.StatusCreated, `{"id":"f9","type":"folder","name":"Homes"}`))
	if r := h.call("set_map_feature", map[string]any{"resource_id": "r1", "document_id": "b1", "type": "folder", "name": "Homes"}); r.IsError {
		t.Fatal(text(r))
	}
	wtReq(t, h.writes()[0], http.MethodPost, "/api/v3/projects/p1/resources/r1/docs/b1/features", "", `{"type":"folder","name":"Homes"}`)
}

func TestDeleteMapFeature_X_MAP_02_FolderWarnsAndDeletes(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(`{"id":"fold1","type":"folder","name":"Homes"}`), wtOK(mtFeatures))
	r := h.call("delete_map_feature", map[string]any{"resource_id": "r1", "document_id": "m1", "feature_id": "fold1", "dry_run": true})
	if r.IsError || !strings.Contains(text(r), `Dry run: would delete folder "Homes" (fold1)`) || !strings.Contains(text(r), "the 1 feature(s) in it are not deleted and keep fold1 as their parentId") {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
	if len(h.writes()) != 0 {
		t.Fatalf("dry run sent %+v", h.writes())
	}
	h.lk.Enqueue(wtOK(`{"id":"f1","type":"pin","name":"Candy Castle"}`), testutil.Reply{Status: http.StatusNoContent})
	r = h.call("delete_map_feature", map[string]any{"resource_id": "r1", "document_id": "m1", "feature_id": "f1"})
	if r.IsError || !strings.Contains(text(r), `Deleted pin "Candy Castle" (f1)`) {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodDelete, mtPath+"/features/f1", "", "")
	if len(h.guard.checked) != 0 {
		t.Errorf("a delete ran the guard: %v", h.guard.checked)
	}
}

func TestSetMapImage_X_MAP_02_SetsImage(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtAsset), wtOK(mtDoc))
	r := h.call("set_map_image", map[string]any{"resource_id": "r1", "document_id": "m1", "asset_id": "a1"})
	if r.IsError {
		t.Fatal(text(r))
	}
	reqs := h.lk.Requests()
	wtReq(t, reqs[1], http.MethodGet, "/api/v3/projects/p1/assets/a1", "", "")
	w := h.writes()
	if len(w) != 1 {
		t.Fatalf("writes %+v", w)
	}
	wtReq(t, w[0], http.MethodPatch, mtPath, "", `{"mapImage":{"assetId":"a1"}}`)
	var out SetMapImageOut
	structured(t, r, &out)
	if out.Image == nil || out.Image.Width != 512 || out.Image.Height != 256 {
		t.Errorf("out = %+v", out)
	}
	if len(h.guard.checked) != 1 || !strings.Contains(h.guard.checked[0], "ooo.png") || !strings.Contains(h.guard.checked[0], "Ooo map") {
		t.Errorf("guard checked %v", h.guard.checked)
	}
}

func TestSetMapImage_X_GUARD_08_GuardAndHidden(t *testing.T) {
	h := rgHarness(t, "Zorblax")
	h.lk.Enqueue(wtOK(mtRes), wtOK(strings.Replace(mtAsset, "Ooo map", "Zorblax map", 1)))
	wtRefused(t, h, text(h.call("set_map_image", map[string]any{"resource_id": "r1", "document_id": "m1", "asset_id": "a1"})))

	// Dry run: nothing written.
	h.lk.Enqueue(wtOK(mtRes), wtOK(mtAsset))
	r := h.call("set_map_image", map[string]any{"resource_id": "r1", "document_id": "m1", "asset_id": "a1", "dry_run": true})
	if r.IsError || !strings.Contains(text(r), "Dry run") || len(h.writes()) != 0 {
		t.Fatalf("%v: %s; writes %+v", r.IsError, text(r), h.writes())
	}

	// A hidden map document may show the term.
	h.lk.Enqueue(wtOK(mtRes), wtOK(strings.Replace(mtAsset, "Ooo map", "Zorblax map", 1)), wtOK(mtDoc))
	if r := h.call("set_map_image", map[string]any{"resource_id": "r1", "document_id": "m2", "asset_id": "a1"}); r.IsError {
		t.Fatal(text(r))
	}
	if len(h.writes()) != 1 {
		t.Errorf("writes %+v", h.writes())
	}
}

func TestSetMapImage_RefusesPage(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(wtOK(mtRes))
	r := h.call("set_map_image", map[string]any{"resource_id": "r1", "document_id": "d1", "asset_id": "a1"})
	if !r.IsError || !strings.Contains(text(r), "not a map") || len(h.writes()) != 0 {
		t.Fatalf("%v: %s", r.IsError, text(r))
	}
}

func TestMapTools_X_RO_01_ReadOnlyHasOnlyReadMap(t *testing.T) {
	h := newHarness(t, readOnlyMode())
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	have := map[string]bool{}
	for _, tool := range res.Tools {
		have[tool.Name] = true
	}
	if !have["read_map"] || !have["list_templates"] || have["set_map_feature"] || have["delete_map_feature"] || have["set_map_image"] ||
		have["save_as_template"] || have["create_from_template"] {
		t.Errorf("read-only tools %v", have)
	}
}
