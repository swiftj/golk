package mcpserver

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

const rtBase = "/api/v3/projects/p1"

// rtReq is an expected LegendKeeper request.
type rtReq struct {
	method, path string
	query        url.Values
}

// rtAssertRequests checks that the fake server got exactly want, in order,
// each a bodiless GET-style read with the test key.
func rtAssertRequests(t *testing.T, h *harness, want ...rtReq) {
	t.Helper()
	got := h.lk.Requests()
	if len(got) != len(want) {
		var seen []string
		for _, r := range got {
			seen = append(seen, r.Method+" "+r.Path+"?"+r.RawQuery)
		}
		t.Fatalf("got %d requests %v, want %d", len(got), seen, len(want))
	}
	for i, w := range want {
		g := got[i]
		q, err := url.ParseQuery(g.RawQuery)
		if err != nil {
			t.Fatal(err)
		}
		if w.query == nil {
			w.query = url.Values{}
		}
		if g.Method != w.method || g.Path != w.path || q.Encode() != w.query.Encode() {
			t.Errorf("request %d = %s %s?%s, want %s %s?%s", i, g.Method, g.Path, g.RawQuery, w.method, w.path, w.query.Encode())
		}
		if len(g.Body) != 0 {
			t.Errorf("request %d has a body: %s", i, g.Body)
		}
		if a := g.Header.Get("Authorization"); a != "Bearer test-key" {
			t.Errorf("request %d Authorization = %q", i, a)
		}
	}
}

func rtGet(path string, kv ...string) rtReq {
	q := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		q.Set(kv[i], kv[i+1])
	}
	return rtReq{method: "GET", path: rtBase + path, query: q}
}

// rtStructuredJSON is a result's structured content as JSON.
func rtStructuredJSON(t *testing.T, h *harness, r any) string {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

const rtSearchJSON = `{"data":[
{"resource":{"id":"r1","name":"Candy Kingdom","tags":["place"]}},
{"resource":{"id":"r2","name":"Princess Bubblegum","tags":[]},"document":{"id":"d2","name":"Main","highlights":[{"segment":"rules the Candy Kingdom","index":0,"matchStart":10,"matchEnd":23}]}}],
"hasMore":false}`

const rtListJSON = `{"data":[
{"id":"r1","name":"Candy Kingdom","tags":["place","sweet"],"aliases":[],"hasChildren":true,"isHidden":false,"documents":[{"id":"d1","name":"Main","type":"page"},{"id":"m1","name":"Map","type":"map"}]},
{"id":"r9","name":"Lich Lair","tags":[],"aliases":[],"hasChildren":false,"isHidden":true,"documents":[]}],
"hasMore":true,"nextCursor":"c2"}`

const rtList2JSON = `{"data":[{"id":"r3","name":"Ice Kingdom","tags":[],"aliases":[],"hasChildren":false,"documents":[]}],"hasMore":false,"nextCursor":null}`

const rtResourceJSON = `{"id":"r2","name":"Princess Bubblegum","parentId":"r1","tags":["royal"],"aliases":["PB","Bonnie"],
"rank":"a","isHidden":false,"isLocked":true,"permissions":{},
"ancestors":[{"id":"r1","name":"Candy Kingdom"}],
"documents":[{"id":"d2","name":"Main","type":"page","rank":"a","isHidden":false},{"id":"d3","name":"DM notes","type":"page","rank":"b","isHidden":true}],
"properties":[
{"kind":"keyValue","id":"pr1","rank":"a","title":"Stats","rows":[{"id":"k1","key":"Age","text":"827"}]},
{"kind":"textField","id":"pr2","rank":"b","title":"True origin","isHidden":true,"children":[{"kind":"paragraph","id":"x1","content":[{"text":"Grown from the Mother Gum"}]}]}]}`

const rtTagsJSON = `{"data":[{"tag":"place","count":12},{"tag":"royal","count":3}],"hasMore":false}`

const rtAssetsJSON = `{"data":[
{"id":"a1","name":"Castle","type":"IMAGE","url":"https://assets.legendkeeper.com/p1/castle.png","width":800,"height":600,"isHidden":false},
{"id":"f1","name":"Maps","type":"FOLDER"},
{"id":"a2","name":"Lich portrait","type":"IMAGE","url":"https://assets.legendkeeper.com/p1/lich.png","width":64,"height":64,"isHidden":true}],
"hasMore":true,"nextCursor":"ac2"}`

func TestReadTools_X_TOOL_01_X_OUT_01(t *testing.T) {
	tests := []struct {
		name    string
		replies []testutil.Reply
		tool    string
		args    map[string]any
		reqs    []rtReq
		text    string
		// out is the exact structured content.
		out string
	}{
		{
			name: "search", tool: "search",
			args:    map[string]any{"query": "candy kingdom", "limit": 5},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, rtSearchJSON)},
			reqs:    []rtReq{rtGet("/search", "q", "candy kingdom", "limit", "5")},
			text:    "- Candy Kingdom (r1)\n- Princess Bubblegum (r2) › Main (d2)\n2 result(s).",
			out:     `{"hits":[{"resource_id":"r1","resource_name":"Candy Kingdom"},{"resource_id":"r2","resource_name":"Princess Bubblegum","document_id":"d2","document_name":"Main"}]}`,
		},
		{
			name: "search no hits", tool: "search",
			args:    map[string]any{"query": "Finn"},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, `{"data":[]}`)},
			reqs:    []rtReq{rtGet("/search", "q", "Finn")},
			text:    `No results for "Finn".`,
			out:     `{"hits":[]}`,
		},
		{
			name: "list_resources root", tool: "list_resources",
			args:    map[string]any{"root": true, "limit": 2},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, rtListJSON)},
			reqs:    []rtReq{rtGet("/resources", "parentId", "root", "limit", "2")},
			text: "- Candy Kingdom (r1) [has children] tags: place, sweet; docs: Main (page, d1), Map (map, m1)\n" +
				"- Lich Lair (r9) [hidden]\n" +
				`2 resource(s) on this page; more follow: pass cursor "c2" for the next page.`,
			out: `{"resources":[{"id":"r1","name":"Candy Kingdom","tags":["place","sweet"],"has_children":true,"documents":[{"id":"d1","name":"Main","type":"page"},{"id":"m1","name":"Map","type":"map"}]},` +
				`{"id":"r9","name":"Lich Lair","hidden":true,"documents":[]}],"has_more":true,"next_cursor":"c2"}`,
		},
		{
			name: "list_resources children", tool: "list_resources",
			args:    map[string]any{"parent_id": "r1"},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, rtList2JSON)},
			reqs:    []rtReq{rtGet("/resources", "parentId", "r1")},
			text:    "- Ice Kingdom (r3)\n1 resource(s); no more pages.",
			out:     `{"resources":[{"id":"r3","name":"Ice Kingdom","documents":[]}]}`,
		},
		{
			name: "list_resources whole project", tool: "list_resources",
			args:    map[string]any{},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, `{"data":[]}`)},
			reqs:    []rtReq{rtGet("/resources")},
			text:    "0 resource(s); no more pages.",
			out:     `{"resources":[]}`,
		},
		{
			name: "get_resource", tool: "get_resource",
			args:    map[string]any{"resource_id": "r2"},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, rtResourceJSON)},
			reqs:    []rtReq{rtGet("/resources/r2")},
			text: "Princess Bubblegum (resource r2) [locked]\n" +
				"Parent path: Candy Kingdom (r1)\n" +
				"Tags: royal\n" +
				"Aliases: PB, Bonnie\n" +
				"Documents:\n" +
				"- Main (page, d2)\n" +
				"- DM notes (page, d3) [hidden from players]\n" +
				"Sidebar properties (2), as LK Markdown; isHidden marks DM-only content:\n" +
				"Hidden from players: True origin (pr2)\n\n" +
				":::infobox\n:::keyvalue title=Stats\n- Age {text=827}\n:::\n\n:::textfield title=\"True origin\" isHidden\nGrown from the Mother Gum\n:::\n:::\n",
			out: `{"id":"r2","name":"Princess Bubblegum","parent_id":"r1","ancestors":[{"id":"r1","name":"Candy Kingdom"}],"tags":["royal"],"aliases":["PB","Bonnie"],"locked":true,` +
				`"documents":[{"id":"d2","name":"Main","type":"page"},{"id":"d3","name":"DM notes","type":"page","hidden":true}],` +
				`"properties":[{"id":"pr1","kind":"keyValue","title":"Stats"},{"id":"pr2","kind":"textField","title":"True origin","hidden":true}]}`,
		},
		{
			name: "get_resource no properties", tool: "get_resource",
			args: map[string]any{"resource_id": "r1"},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, `{"id":"r1","name":"Candy Kingdom","tags":[],"aliases":[],"rank":"a","isHidden":true,`+
				`"permissions":{},"documents":[],"properties":[],"ancestors":[]}`)},
			reqs: []rtReq{rtGet("/resources/r1")},
			text: "Candy Kingdom (resource r1) [hidden from players]\nParent: none (top level)\nDocuments:\n- none\nSidebar properties: none.",
			out:  `{"id":"r1","name":"Candy Kingdom","tags":[],"aliases":[],"hidden":true,"documents":[],"properties":[]}`,
		},
		{
			name: "list_tags", tool: "list_tags",
			args:    map[string]any{},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, rtTagsJSON)},
			reqs:    []rtReq{rtGet("/tags")},
			text:    "- place (12)\n- royal (3)\n2 tag(s).",
			out:     `{"tags":[{"tag":"place","count":12},{"tag":"royal","count":3}]}`,
		},
		{
			name: "list_assets", tool: "list_assets",
			args:    map[string]any{"folder_id": "f0", "limit": 3},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, rtAssetsJSON)},
			reqs:    []rtReq{rtGet("/assets", "folderId", "f0", "limit", "3")},
			text: "- Castle (a1) IMAGE 800x600 https://assets.legendkeeper.com/p1/castle.png\n" +
				"- Maps (f1) FOLDER\n" +
				"- Lich portrait (a2) IMAGE 64x64 [hidden] https://assets.legendkeeper.com/p1/lich.png\n" +
				`3 asset(s) on this page; more follow: pass cursor "ac2" for the next page.`,
			out: `{"assets":[{"id":"a1","name":"Castle","type":"IMAGE","url":"https://assets.legendkeeper.com/p1/castle.png","width":800,"height":600},` +
				`{"id":"f1","name":"Maps","type":"FOLDER"},` +
				`{"id":"a2","name":"Lich portrait","type":"IMAGE","url":"https://assets.legendkeeper.com/p1/lich.png","width":64,"height":64,"hidden":true}],"has_more":true,"next_cursor":"ac2"}`,
		},
		{
			name: "list_assets root", tool: "list_assets",
			args:    map[string]any{},
			replies: []testutil.Reply{testutil.JSON(http.StatusOK, `{"data":[]}`)},
			reqs:    []rtReq{rtGet("/assets")},
			text:    "0 asset(s); no more pages.",
			out:     `{"assets":[]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.lk.Enqueue(tt.replies...)
			r := h.call(tt.tool, tt.args)
			if r.IsError {
				t.Fatal(text(r))
			}
			rtAssertRequests(t, h, tt.reqs...)
			if got := text(r); got != tt.text {
				t.Errorf("text\n%s\nwant\n%s", got, tt.text)
			}
			var want any
			if err := json.Unmarshal([]byte(tt.out), &want); err != nil {
				t.Fatal(err)
			}
			if got := rtStructuredJSON(t, h, r.StructuredContent); got != rtStructuredJSON(t, h, want) {
				t.Errorf("structured\n%s\nwant\n%s", got, tt.out)
			}
		})
	}
}

// X-OUT-01: property and page content is in the text only.
func TestGetResource_X_OUT_01_ContentOnlyInText(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, rtResourceJSON))
	r := h.call("get_resource", map[string]any{"resource_id": "r2"})
	if r.IsError {
		t.Fatal(text(r))
	}
	for _, content := range []string{"Grown from the Mother Gum", "827"} {
		if !strings.Contains(text(r), content) {
			t.Errorf("text lacks %q", content)
		}
		if strings.Contains(rtStructuredJSON(t, h, r.StructuredContent), content) {
			t.Errorf("structured output holds content %q", content)
		}
	}
}

func TestSearch_X_OUT_01_HighlightsNotInStructured(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, rtSearchJSON))
	r := h.call("search", map[string]any{"query": "candy"})
	if strings.Contains(rtStructuredJSON(t, h, r.StructuredContent), "rules the") {
		t.Error("structured output holds a page passage")
	}
}

// Invalid input is a tool error and sends nothing.
func TestReadTools_X_TOOL_01_InvalidInputSendsNothing(t *testing.T) {
	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{"search", map[string]any{"query": ""}, "query is empty"},
		{"search", map[string]any{"query": "   "}, "query is empty"},
		{"search", map[string]any{"query": strings.Repeat("é", maxQuery+1)}, "the limit is 500"},
		{"search", map[string]any{"query": "x", "limit": 101}, "limit must be between 1 and 100"},
		{"list_resources", map[string]any{"root": true, "parent_id": "r1"}, "not both"},
		{"list_resources", map[string]any{"limit": -1}, "limit must be between 1 and 100"},
		{"list_resources", map[string]any{"cursor": "made-up"}, "unknown cursor"},
		{"list_assets", map[string]any{"cursor": "made-up"}, "unknown cursor"},
		{"list_assets", map[string]any{"limit": 500}, "limit must be between 1 and 100"},
		{"get_resource", map[string]any{"resource_id": ""}, "resource_id is empty"},
	}
	for _, tt := range tests {
		t.Run(tt.tool+" "+tt.want, func(t *testing.T) {
			h := newHarness(t)
			r := h.call(tt.tool, tt.args)
			if !r.IsError || !strings.Contains(text(r), tt.want) {
				t.Errorf("result %v: %s", r.IsError, text(r))
			}
			rtAssertRequests(t, h)
		})
	}
}

func TestSearch_X_TOOL_01_QueryAtLimitIsSent(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, `{"data":[]}`))
	q := strings.Repeat("é", maxQuery)
	if r := h.call("search", map[string]any{"query": q}); r.IsError {
		t.Fatal(text(r))
	}
	rtAssertRequests(t, h, rtGet("/search", "q", q))
}

// A cursor fetches the next page with the same filters, and works once
// per page it names.
func TestListResources_X_TOOL_01_Cursor(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, rtListJSON), testutil.JSON(http.StatusOK, rtList2JSON))
	r1 := h.call("list_resources", map[string]any{"root": true, "limit": 2})
	var out ListResourcesOut
	structured(t, r1, &out)
	if out.NextCursor != "c2" {
		t.Fatalf("next_cursor = %q", out.NextCursor)
	}
	// Other filters with that cursor are refused without a request.
	if r := h.call("list_resources", map[string]any{"parent_id": "r1", "cursor": "c2"}); !r.IsError || !strings.Contains(text(r), "unknown cursor") {
		t.Errorf("cursor with other filters: %v %s", r.IsError, text(r))
	}
	r2 := h.call("list_resources", map[string]any{"root": true, "limit": 2, "cursor": "c2"})
	if r2.IsError {
		t.Fatal(text(r2))
	}
	if got, want := text(r2), "- Ice Kingdom (r3)\n1 resource(s); no more pages."; got != want {
		t.Errorf("text %q, want %q", got, want)
	}
	rtAssertRequests(t, h,
		rtGet("/resources", "parentId", "root", "limit", "2"),
		rtGet("/resources", "parentId", "root", "limit", "2", "cursor", "c2"))
}

func TestListAssets_X_TOOL_01_Cursor(t *testing.T) {
	h := newHarness(t)
	h.lk.Enqueue(testutil.JSON(http.StatusOK, rtAssetsJSON), testutil.JSON(http.StatusOK, `{"data":[],"hasMore":false}`))
	if r := h.call("list_assets", map[string]any{"folder_id": "f0"}); r.IsError {
		t.Fatal(text(r))
	}
	if r := h.call("list_assets", map[string]any{"folder_id": "f0", "cursor": "ac2"}); r.IsError {
		t.Fatal(text(r))
	}
	rtAssertRequests(t, h, rtGet("/assets", "folderId", "f0"), rtGet("/assets", "folderId", "f0", "cursor", "ac2"))
}

func TestPageCache_X_TOOL_01_Bounded(t *testing.T) {
	pc := newPageCache()
	for i := range pageCacheSize + 10 {
		pc.put(strings.Repeat("k", i+1), i)
	}
	if len(pc.pages) != pageCacheSize || len(pc.order) != pageCacheSize {
		t.Errorf("cache holds %d/%d entries", len(pc.pages), len(pc.order))
	}
	if _, ok := pc.get("k"); ok {
		t.Error("the oldest entry was kept")
	}
}

// API errors of the new tools carry code, request ID and hint (X-ERR-01).
func TestReadTools_X_ERR_01_APIError(t *testing.T) {
	for _, tool := range []struct {
		name string
		args map[string]any
	}{
		{"search", map[string]any{"query": "x"}},
		{"list_resources", map[string]any{}},
		{"get_resource", map[string]any{"resource_id": "r1"}},
		{"list_tags", map[string]any{}},
		{"list_assets", map[string]any{}},
	} {
		t.Run(tool.name, func(t *testing.T) {
			h := newHarness(t)
			h.lk.Enqueue(testutil.APIError(http.StatusForbidden, "FORBIDDEN", "req-7"))
			r := h.call(tool.name, tool.args)
			for _, want := range []string{"FORBIDDEN", "HTTP 403", "req-7", "Hint:"} {
				if !r.IsError || !strings.Contains(text(r), want) {
					t.Errorf("error text lacks %q: %s", want, text(r))
				}
			}
		})
	}
}
