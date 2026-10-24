package guard

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

const sentinel = "Zorblax"

// secretTree builds, from seed, a blocks-shaped JSON tree in which the
// sentinel appears only inside DM-only subtrees (kind "secret", or
// isHidden true). When public is true, the sentinel is also put in exactly
// one public string, whose path is returned.
func secretTree(seed uint64, public bool) (json.RawMessage, string) {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	publicAt, count, found := -1, 0, ""
	if public {
		publicAt = r.IntN(8)
	}
	// Near misses that must never match.
	noise := []string{"hello", "Zorblaxian", "AZorblax", "Zor blax", "the north", "ZORBLAXES", "zorblax2"}
	str := func(dm bool, path string) string {
		if dm {
			if r.IntN(2) == 0 {
				return "a " + []string{"Zorblax", "zorblax's", "(ZORBLAX)"}[r.IntN(3)] + " b"
			}
		} else {
			if count == publicAt {
				publicAt, found = -1, path
				count++
				return "see Zorblax."
			}
			count++
		}
		return noise[r.IntN(len(noise))]
	}
	var node func(depth int, dm bool, path string) any
	kids := func(depth int, dm bool, path string) []any {
		var out []any
		for i := range 1 + r.IntN(3) {
			out = append(out, node(depth+1, dm, fmt.Sprintf("%s.children[%d]", path, i)))
		}
		return out
	}
	node = func(depth int, dm bool, path string) any {
		if depth > 4 {
			return str(dm, path)
		}
		switch r.IntN(6) {
		case 0: // secret block
			return map[string]any{"kind": "secret", "children": kids(depth, true, path)}
		case 1: // hidden infobox property
			return map[string]any{"kind": "textField", "isHidden": true, "title": str(true, path+".title"),
				"content": []any{map[string]any{"text": str(true, path+".content[0].text")}}}
		case 2: // paragraph
			return map[string]any{"kind": "paragraph", "content": []any{map[string]any{"text": str(dm, path+".content[0].text")}}}
		case 3: // container
			kind := []string{"quote", "bulletList", "listItem"}[r.IntN(3)]
			return map[string]any{"kind": kind, "children": kids(depth, dm, path)}
		case 4: // mention
			return map[string]any{"kind": "paragraph", "content": []any{map[string]any{
				"mention": map[string]any{"resourceId": "abcd1234", "label": str(dm, path+".content[0].mention.label")}}}}
		default:
			return str(dm, path)
		}
	}
	var top []any
	n := 1 + r.IntN(6)
	for i := range n {
		top = append(top, node(0, false, fmt.Sprintf("$[%d]", i)))
	}
	if publicAt >= 0 { // the chosen public string was never generated
		top = append(top, "Zorblax!")
		found = fmt.Sprintf("$[%d]", n)
	}
	b, err := json.Marshal(top)
	if err != nil {
		panic(err)
	}
	return b, found
}

func FuzzGuardSecretOracle(f *testing.F) {
	for _, s := range []uint64{0, 1, 2, 42, 1 << 40} {
		f.Add(s, false)
		f.Add(s, true)
	}
	g := Open(writeTerms(f, sentinel+"\n"))
	f.Fuzz(func(t *testing.T, seed uint64, public bool) {
		doc, path := secretTree(seed, public)
		vs, err := g.Check(doc)
		if err != nil {
			t.Fatal(err)
		}
		if !public {
			if len(vs) != 0 {
				t.Fatalf("DM-only sentinel flagged: %v\n%s", vs, doc)
			}
			return
		}
		want := []Violation{{Term: sentinel, Path: path}}
		if !reflect.DeepEqual(vs, want) {
			t.Fatalf("got %v, want %v\n%s", vs, want, doc)
		}
	})
}

// oracleMatch restates the rules for the case the fuzzer can check
// independently: the normalized term words, joined by single spaces,
// appear in the normalized text with boundaries where the term's edges are
// word runes.
func oracleMatch(term, text string) bool {
	var words []string
	for _, w := range strings.Fields(term) {
		if n := Normalize(w); n != "" {
			words = append(words, n)
		}
	}
	if len(words) == 0 {
		return false
	}
	nt := []rune(strings.Join(words, " "))
	tx := []rune(Normalize(text))
	for i := 0; i+len(nt) <= len(tx); i++ {
		if string(tx[i:i+len(nt)]) != string(nt) {
			continue
		}
		if isWord(nt[0]) && i > 0 && isWord(tx[i-1]) {
			continue
		}
		e := i + len(nt)
		if isWord(nt[len(nt)-1]) && e < len(tx) && isWord(tx[e]) {
			continue
		}
		return true
	}
	return false
}

func FuzzGuardMatch(f *testing.F) {
	f.Add("Zorblax", "Zorblax's tower")
	f.Add("the Crimson Accord", "sign the crimson accord")
	f.Add("Ærendil", "ærendil")
	f.Add("#x", "a#x")
	f.Add("\u00ADk", "\u212A")
	f.Add("", "")
	f.Add("a b", "a\u00A0b")
	f.Fuzz(func(t *testing.T, term, text string) {
		got := Matches(term, text)
		if oracleMatch(term, text) && !got {
			t.Fatalf("Matches(%q, %q) = false; the oracle finds it", term, text)
		}
		// Check agrees with Matches on a one-string document.
		vs, err := check(newMatcher([]string{term}), map[string]string{"t": text})
		if err != nil {
			t.Fatal(err)
		}
		if (len(vs) > 0) != got {
			t.Fatalf("Check found %v; Matches = %v", vs, got)
		}
	})
}
