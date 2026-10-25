package lk

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var (
	contractRow = regexp.MustCompile(`^\| ([CMX]-[A-Z][A-Z0-9]*-\d+)\b[^|]*\|.*\| ([^|]+) \|$`)
	// A test names an ID in its function name (C_RETRY_03) or in a t.Run
	// subtest name ("C-RETRY-03 ...").
	testFuncID = regexp.MustCompile(`func (?:Test|Fuzz|Example)\w*`)
	idInFunc   = regexp.MustCompile(`[CMX]_[A-Z][A-Z0-9]*_\d+`)
	idInRun    = regexp.MustCompile(`t\.Run\("([CMX]-[A-Z][A-Z0-9]*-\d+)`)
	idInCase   = regexp.MustCompile(`name:\s*"([CMX]-[A-Z][A-Z0-9]*-\d+)`)
)

// Every in-scope contract item (phase column starting with "1", "2a", "2b", "3", "4", "5" or "6") must be named
// by at least one test in the module (docs/SDK_CONTRACT.md, Rules).
func TestContractCoverage(t *testing.T) {
	inScope := contractItems(t)
	if len(inScope) < 240 {
		t.Fatalf("parsed only %d in-scope items; is the table format still recognised?", len(inScope))
	}
	covered := map[string]bool{}
	root := filepath.Join("..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, fn := range testFuncID.FindAllString(string(src), -1) {
			for _, id := range idInFunc.FindAllString(fn, -1) {
				covered[strings.ReplaceAll(id, "_", "-")] = true
			}
		}
		for _, re := range []*regexp.Regexp{idInRun, idInCase} {
			for _, m := range re.FindAllStringSubmatch(string(src), -1) {
				covered[m[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, id := range inScope {
		if !covered[id] {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d in-scope contract items have no test naming them:\n%s", len(missing), strings.Join(missing, "\n"))
	}
}

func contractItems(t *testing.T) []string {
	t.Helper()
	f, err := os.Open("../docs/SDK_CONTRACT.md")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var ids []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		m := contractRow.FindStringSubmatch(sc.Text())
		if m == nil {
			continue
		}
		phase := strings.TrimSpace(m[2])
		if (strings.HasPrefix(phase, "1") || strings.HasPrefix(phase, "2a") || strings.HasPrefix(phase, "2b") || strings.HasPrefix(phase, "3") || strings.HasPrefix(phase, "4") || strings.HasPrefix(phase, "5") || strings.HasPrefix(phase, "6")) && !slices.Contains(ids, m[1]) {
			ids = append(ids, m[1])
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return ids
}
