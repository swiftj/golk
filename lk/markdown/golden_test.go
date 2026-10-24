package markdown

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files in testdata")

// golden compares got with testdata/name, or rewrites it with -update.
func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from the rendering (run with -update to accept):\n%s", path, got)
	}
}

// The golden files pin the rendering of every construct (M-API-02: the same
// page always renders the same bytes).
func TestRender_M_API_02_Golden(t *testing.T) {
	page, _ := goldenPage(t)
	md := roundTrip(t, page)
	golden(t, "golden_page.md", md)
	ids, err := ToMarkdown(page, ToOptions{IDs: true})
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "golden_page_ids.md", ids)

	var b strings.Builder
	for _, s := range shapes {
		md, err := ToMarkdown(listOf(t, "["+s.json+"]"), ToOptions{})
		if err != nil {
			t.Fatal(err)
		}
		b.WriteString("###### " + s.name + "\n\n" + md + "\n")
	}
	golden(t, "shapes.md", b.String())
}

// Each golden file parses back to the blocks it was rendered from.
func TestParse_M_API_01_GoldenFiles(t *testing.T) {
	page, _ := goldenPage(t)
	for _, name := range []string{"golden_page.md", "golden_page_ids.md"} {
		src, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := parse(string(src))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if a, b := shown(t, got), shown(t, page); a != b {
			t.Errorf("%s: got %s\nwant %s", name, a, b)
		}
	}
}
