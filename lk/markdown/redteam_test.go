package markdown

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/lk/blocks"
)

// Red-team tests: attacks on secret handling, content preservation and the
// fail-closed rules. The sentinel is a made-up word that appears only inside
// secrets in the input; the oracle checks that it never reaches any field of
// a non-secret part of the result.

const sentinel = "Zqxlich"

// publicJSON is the JSON of l with every secret subtree removed (blocks
// and lk-raw secrets alike: both decode to *blocks.SecretBlock).
func publicJSON(t testing.TB, l blocks.List) string {
	t.Helper()
	data, err := json.Marshal(l)
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(dropSecrets(tree))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func dropSecrets(v any) any {
	switch t := v.(type) {
	case []any:
		var out []any
		for _, e := range t {
			if m, ok := e.(map[string]any); ok && m["kind"] == "secret" {
				continue
			}
			out = append(out, dropSecrets(e))
		}
		return out
	case map[string]any:
		for k, e := range t {
			t[k] = dropSecrets(e)
		}
		return t
	}
	return v
}

func leaks(t testing.TB, l blocks.List) bool {
	t.Helper()
	return strings.Contains(publicJSON(t, l), sentinel)
}

// secretPage is a page with the sentinel inside secrets at every place a
// secret can sit, between public blocks that use fences, lists and quotes.
func secretPage(t testing.TB) blocks.List {
	t.Helper()
	return listOf(t, `[
 {"kind":"heading","id":"h1","level":1,"content":[{"text":"Public"}]},
 {"kind":"code","id":"c1","language":"js","text":"let a = 1"},
 {"kind":"secret","id":"s1","children":[
   {"kind":"paragraph","id":"s1p","content":[{"text":"The `+sentinel+` lives."}]},
   {"kind":"code","id":"s1c","text":"`+sentinel+` code"}]},
 {"kind":"paragraph","id":"p1","content":[{"text":"Between"}]},
 {"kind":"code","id":"c2","text":"x = 2"},
 {"kind":"quote","id":"q1","children":[
   {"kind":"secret","id":"s2","children":[{"kind":"paragraph","id":"s2p","content":[{"text":"`+sentinel+` in quote"}]}]}]},
 {"kind":"bulletList","id":"l1","children":[
   {"kind":"listItem","id":"li1","children":[
     {"kind":"paragraph","id":"li1p","content":[{"text":"item"}]},
     {"kind":"secret","id":"s3","children":[{"kind":"paragraph","id":"s3p","content":[{"text":"`+sentinel+` in list"}]}]}]}]},
 {"kind":"callout","id":"co1","tone":"info","children":[
   {"kind":"secret","id":"s4","children":[{"kind":"paragraph","id":"s4p","content":[{"text":"`+sentinel+` in callout"}]}]}]},
 {"kind":"infobox","id":"ib1","rows":[
   {"kind":"textField","id":"tf1","rank":"a","title":"Notes","children":[
     {"kind":"secret","id":"s5","children":[{"kind":"paragraph","id":"s5p","content":[{"text":"`+sentinel+` in infobox"}]}]}]}]},
 {"kind":"layout","id":"lay","children":[
   {"kind":"layoutColumn","id":"col1","children":[
     {"kind":"secret","id":"s6","children":[{"kind":"paragraph","id":"s6p","content":[{"text":"`+sentinel+` in column"}]}]}]}]},
 {"kind":"code","id":"c3","text":"y = 3"},
 {"kind":"paragraph","id":"p2","content":[{"text":"End"}]}
]`)
}

// TestRedteam_M_SAFE_02_SecretPageRoundTrips checks the fixture itself.
func TestRedteam_M_SAFE_02_SecretPageRoundTrips(t *testing.T) {
	page := secretPage(t)
	if leaks(t, page) {
		t.Fatal("fixture leaks")
	}
	md := roundTrip(t, page)
	if hasRaw(md) {
		t.Fatalf("fixture renders lk-raw:\n%s", md)
	}
}

// TestRedteam_M_SAFE_02_FenceSwallowsSecret: a code fence that is never
// closed before a secret (an editor dropped its closing line) must not turn
// the secret into a public code block. Before the fix this parsed, with the
// secret's text in a visible code block.
func TestRedteam_M_SAFE_02_FenceSwallowsSecret(t *testing.T) {
	for _, src := range []string{
		"```js\ncode\n\n:::secret\n" + sentinel + "\n:::\n\n```py\nx\n```\n",
		"~~~\n:::secret\n" + sentinel + "\n:::\n~~~\n",
		"```\n::::secret layout=center\n" + sentinel + "\n::::\n```\n",
		"> ```\n> :::secret\n> " + sentinel + "\n> :::\n> ```\n",
		"- ```\n  :::secret\n  " + sentinel + "\n  :::\n  ```\n",
		"```\n   :::secret\n" + sentinel + "\n:::\n```\n",
		"```\n:::Secret\n" + sentinel + "\n:::\n```\n",
	} {
		got, err := parse(src)
		var se *SyntaxError
		if !errors.As(err, &se) || got != nil {
			t.Errorf("%q: want a *SyntaxError, got %v %s", src, err, mustJSON(t, got))
		}
	}
	// The edit that causes it: drop the closing fence of the first code
	// block of a rendered page.
	page := secretPage(t)
	md, err := ToMarkdown(page, ToOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(md, "\n")
	for i, l := range lines {
		if l == "let a = 1" {
			lines = append(lines[:i+1], lines[i+2:]...) // drop the closing ```
			break
		}
	}
	edited := strings.Join(lines, "\n")
	got, _, err := FromMarkdown(edited, FromOptions{Base: page})
	if err == nil && leaks(t, got) {
		t.Fatalf("secret text in a public block:\n%s\n%s", edited, publicJSON(t, got))
	}
	if err == nil {
		t.Fatalf("want an error for\n%s", edited)
	}
}

// A code block whose text holds a secret fence line still round-trips: it
// renders as lk-raw.
func TestRedteam_M_SAFE_02_SecretFenceTextInCode(t *testing.T) {
	page := listOf(t, `[{"kind":"code","id":"c","text":"Example:\n:::secret\nhidden\n:::"}]`)
	md := roundTrip(t, page)
	if !hasRaw(md) {
		t.Fatalf("want lk-raw:\n%s", md)
	}
}

// TestRedteam_M_SAFE_06_UnsupportedLeavesSecret: an ::unsupported line
// moved out of a secret would publish content the editor can't see (and a
// spoiler guard can't check). It is an *IDError.
func TestRedteam_M_SAFE_06_UnsupportedLeavesSecret(t *testing.T) {
	base := listOf(t, `[
 {"kind":"secret","id":"s1","children":[
   {"kind":"unsupported","id":"u1"},
   {"kind":"paragraph","id":"p1","content":[{"text":"x"}]}]},
 {"kind":"infobox","id":"ib","rows":[{"kind":"textField","id":"tf","rank":"a","children":[
   {"kind":"secret","id":"s2","children":[{"kind":"unsupported","id":"u2"}]}]}]}
]`)
	md, err := ToMarkdown(base, ToOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := FromMarkdown(md, FromOptions{Base: base}); err != nil {
		t.Fatalf("unchanged: %v", err)
	}
	for name, src := range map[string]string{
		"moved out":            "::unsupported id=u1\n\n:::secret\nx\n:::\n\n:::infobox\n:::textfield\n:::secret\n::unsupported id=u2\n:::\n:::\n:::\n",
		"secret unwrapped":     "::unsupported id=u1\n\nx\n\n:::infobox\n:::textfield\n:::secret\n::unsupported id=u2\n:::\n:::\n:::\n",
		"row secret unwrapped": ":::secret\n::unsupported id=u1\n\nx\n:::\n\n:::infobox\n:::textfield\n::unsupported id=u2\n:::\n:::\n",
		"into a callout":       ":::callout tone=info\n::unsupported id=u1\n:::\n\n:::secret\nx\n:::\n\n:::infobox\n:::textfield\n:::secret\n::unsupported id=u2\n:::\n:::\n:::\n",
		"lk-raw form":          "```lk-raw\n{\"kind\":\"unsupported\",\"id\":\"u1\"}\n```\n\n:::secret\nx\n:::\n\n:::infobox\n:::textfield\n:::secret\n::unsupported id=u2\n:::\n:::\n:::\n",
	} {
		_, _, err := FromMarkdown(src, FromOptions{Base: base, AllowRemoveOpaque: true})
		var ie *IDError
		if !errors.As(err, &ie) {
			t.Errorf("%s: want an *IDError, got %v", name, err)
		}
	}
	// Moving one into a secret, or within secrets, is fine.
	in := ":::secret\n::unsupported id=u1\n:::\n\n:::secret\n:::secret\nx\n:::\n:::\n\n:::infobox\n:::textfield\n:::secret\n::unsupported id=u2\n:::\n:::\n:::\n"
	if _, _, err := FromMarkdown(in, FromOptions{Base: base}); err != nil {
		t.Errorf("move within secrets: %v", err)
	}
}

// FuzzRedteamSecretTemplate: arbitrary Markdown around a secret whose first
// line holds the sentinel. If it parses, the sentinel is only in secrets.
func FuzzRedteamSecretTemplate(f *testing.F) {
	for _, s := range [][4]string{
		{"Intro", ":::secret", "more", ":::"},
		{"```", ":::secret", "", ":::\n```"},
		{"> a", "> :::secret", "> b", "> :::"},
		{"- a", "  :::secret", "  b", "  :::"},
		{"", "::::secret layout=center", "x", "::::"},
		{"~~~~", ":::secret", "~~~~", ":::"},
		{"| a |\n| --- |", ":::secret", "| b |", ":::"},
		{":::callout tone=info", ":::secret", ":::", ":::"},
		{"", ":::secret", "```\n:::\n```", ":::"},
		{"[a](", ":::secret", ")", ":::"},
		{"", ":::secret", "", ":::\n:::"},
		{"", "- :::secret", "  x", "  :::"},
		{":::infobox\n:::textfield", ":::secret", "x", ":::\n:::\n:::"},
		{":::table\n:::row\n:::cell", ":::secret", "x", ":::\n:::\n:::\n:::"},
		{":::keyvalue\n- a ::", ":::secret", "x", ":::\n:::"},
	} {
		f.Add(s[0], s[1], s[2], s[3])
	}
	f.Fuzz(func(t *testing.T, pre, open, body, post string) {
		pre = strings.ReplaceAll(pre, sentinel, "")
		open = strings.ReplaceAll(open, sentinel, "")
		body = strings.ReplaceAll(body, sentinel, "")
		post = strings.ReplaceAll(post, sentinel, "")
		if strings.ContainsAny(open, "\n\r") || !secretOpener(open) {
			return // only an attempted secret makes the sentinel secret
		}
		// The sentinel line is the line right after the opener, with the
		// opener's own prefix (quote markers, list indentation).
		prefix := open[:len(open)-len(strings.TrimLeft(open, " >"))]
		src := pre + "\n" + open + "\n" + prefix + sentinel + " " + body + "\n" + post + "\n"
		got, _, err := FromMarkdown(src, FromOptions{})
		if err != nil {
			return
		}
		if leaks(t, got) {
			t.Fatalf("sentinel outside a secret\ninput %q\npublic %s", src, publicJSON(t, got))
		}
	})
}

// FuzzRedteamSecretEdit: an edit of a rendered page that only deletes lines
// and inserts lines without the sentinel can't move the sentinel out of a
// secret, except by deleting a secret's opening line (unwrapping it), which
// Report.Removed must then list as a removed secret, or by deleting a
// closing line and inserting one earlier (moving it), which the Report
// must show as a change to a secret. Spotting text that left a secret is
// the spoiler guard's job; the Report only says which secrets changed.
func FuzzRedteamSecretEdit(f *testing.F) {
	f.Add(uint16(3), uint16(0), uint16(0), uint16(0), "")
	f.Add(uint16(4), uint16(0), uint16(0), uint16(2), "```")
	f.Add(uint16(0), uint16(0), uint16(0), uint16(9), ":::")
	f.Add(uint16(12), uint16(14), uint16(0), uint16(5), "    ")
	f.Add(uint16(1), uint16(2), uint16(3), uint16(4), ":::secret")
	f.Add(uint16(0), uint16(0), uint16(0), uint16(6), "> ")
	f.Add(uint16(0), uint16(0), uint16(0), uint16(6), "<div>")
	page := secretPage(f)
	md, err := ToMarkdown(page, ToOptions{})
	if err != nil {
		f.Fatal(err)
	}
	base := strings.Split(strings.TrimSuffix(md, "\n"), "\n")
	f.Fuzz(func(t *testing.T, d1, d2, d3, at uint16, ins string) {
		ins = strings.ReplaceAll(ins, sentinel, "")
		lines := append([]string(nil), base...)
		unwrapped, closer := false, false
		for _, d := range []uint16{d1, d2, d3} {
			if d == 0 || len(lines) == 0 {
				continue
			}
			i := int(d-1) % len(lines)
			if strings.Contains(lines[i], sentinel) {
				continue // deleting the sentinel can't leak it
			}
			unwrapped = unwrapped || secretOpener(lines[i])
			closer = closer || isClosingFence([]byte(strings.TrimLeft(lines[i], " \t>")))
			lines = append(lines[:i], lines[i+1:]...)
		}
		if ins != "" {
			i := int(at) % (len(lines) + 1)
			lines = append(lines[:i], append(strings.Split(ins, "\n"), lines[i:]...)...)
		}
		src := strings.Join(lines, "\n") + "\n"
		for _, b := range []blocks.List{page, nil} {
			got, rep, err := FromMarkdown(src, FromOptions{Base: b, AllowRemoveOpaque: true})
			if err != nil {
				continue
			}
			if unwrapped {
				if b != nil && leaks(t, got) && !removedSecret(rep) {
					t.Fatalf("secret unwrapped without a removed secret in the Report\ninput:\n%s\nreport %+v", src, rep)
				}
				continue
			}
			if closer {
				if b != nil && leaks(t, got) && !secretTouched(rep, b) {
					t.Fatalf("secret shrunk without a changed secret in the Report\ninput:\n%s\nreport %+v", src, rep)
				}
				continue
			}
			if leaks(t, got) {
				t.Fatalf("sentinel outside a secret\ninput:\n%s\npublic %s", src, publicJSON(t, got))
			}
		}
	})
}

// TestRedteam_M_OPQ_02_KindMatchedExactly: an lk-raw object's kind is its
// exact "kind" key, as the server reads it. encoding/json's case-insensitive
// matching made {"kind":"secret","Kind":"hologram"} an unknown-kind raw
// block here, sent verbatim, which the server reads as a secret; and an
// object holding only "Kind" decoded although it has no kind.
func TestRedteam_M_OPQ_02_KindMatchedExactly(t *testing.T) {
	for _, c := range []struct {
		json string
		want blocks.Kind
	}{
		{`{"kind":"secret","Kind":"hologram","children":[{"kind":"paragraph","content":[{"text":"x"}]}]}`, blocks.KindSecret},
		{`{"Kind":"secret","kind":"paragraph","content":[{"text":"x"}]}`, blocks.KindParagraph},
		{`{"kind":"paragraph","KIND":"secret","content":[{"text":"x"}]}`, blocks.KindParagraph},
	} {
		got, err := parse("```lk-raw\n" + c.json + "\n```\n")
		if err != nil {
			t.Fatalf("%s: %v", c.json, err)
		}
		if len(got) != 1 || got[0].Kind() != c.want {
			t.Errorf("%s: got %s, want kind %s", c.json, mustJSON(t, got), c.want)
		}
	}
	if _, err := parse("```lk-raw\n{\"Kind\":\"secret\",\"children\":[]}\n```\n"); err == nil {
		t.Error("an object without an exact kind key parsed")
	}
}

// secretOpener reports whether line, after quote markers, list markers and
// indentation, is two or more colons and then "secret" in any case.
func secretOpener(line string) bool {
	l := strings.TrimLeft(line, " \t>-")
	n := len(l) - len(strings.TrimLeft(l, ":"))
	return n >= 2 && strings.HasPrefix(strings.ToLower(strings.TrimLeft(l[n:], " \t")), "secret")
}

func removedSecret(rep *Report) bool {
	for _, r := range rep.Removed {
		if r.Secret {
			return true
		}
	}
	return false
}

// secretTouched reports whether rep lists a secret of base as changed or
// removed.
func secretTouched(rep *Report, base blocks.List) bool {
	secrets := map[string]bool{}
	blocks.Walk(base, func(_ string, n blocks.Node) bool {
		if n.Kind() == blocks.KindSecret {
			secrets[n.NodeID()] = true
		}
		return true
	})
	for _, id := range rep.Changed {
		if secrets[id] {
			return true
		}
	}
	return removedSecret(rep)
}

// TestRedteam_M_SAFE_02_TwoColonSecret: "::" then a name after spaces is a
// mistyped directive fence. It used to parse as visible text, so ":: secret"
// published the text after it.
func TestRedteam_M_SAFE_02_TwoColonSecret(t *testing.T) {
	for _, src := range []string{
		":: secret\n" + sentinel + "\n:::\n",
		":: secret\n" + sentinel + "\n",
		"::\tSecret\n" + sentinel + "\n",
		"Intro\n:: secret\n",
		"> :: secret\n> " + sentinel + "\n",
		"- :: secret\n  " + sentinel + "\n",
	} {
		got, err := parse(src)
		var se *SyntaxError
		if !errors.As(err, &se) || got != nil {
			t.Errorf("%q: want a *SyntaxError, got %v %s", src, err, mustJSON(t, got))
		}
	}
	// Still fine: an empty key-value key, text that merely contains "::",
	// and the rendering of a paragraph starting ":: x".
	for _, src := range []string{
		":::keyvalue\n- :: secret\n:::\n",
		"a :: secret\n",
		"\\:: secret\n",
		"::\n",
		":: 1\n",
	} {
		if _, err := parse(src); err != nil {
			t.Errorf("%q: %v", src, err)
		}
	}
	roundTrip(t, listOf(t, `[{"kind":"paragraph","content":[{"text":":: secret"}]},{"kind":"keyValue","rows":[{"key":"","content":[{"text":":: secret"}]}]}]`))
}

// TestRedteam_M_API_03_QuadraticMarks: goldmark's link and emphasis
// parsing is quadratic in the markers of one paragraph; "[a](" or "*a_"
// repeated to 1 MiB took minutes. Such a paragraph or heading is now an
// error, found before inline parsing.
func TestRedteam_M_API_03_QuadraticMarks(t *testing.T) {
	for _, unit := range []string{"[a](", "![a](", "*a_", "_a*\n", "[a](<", "~a*"} {
		n := maxInput / len(unit)
		heading := "# " + strings.Repeat(strings.TrimSuffix(unit, "\n"), n-1)
		for _, src := range []string{strings.Repeat(unit, n), heading} {
			start := time.Now()
			got, _, err := FromMarkdown(src, FromOptions{})
			var se *SyntaxError
			if !errors.As(err, &se) || got != nil {
				t.Errorf("%q: want a *SyntaxError, got %v", unit, err)
			}
			if d := time.Since(start); d > 3*time.Second*slowdown {
				t.Errorf("%q: %v", unit, d)
			}
		}
	}
	// Many paragraphs just under the bound stay bounded too.
	para := strings.Repeat("*a_", maxMarks/2) + "\n\n"
	start := time.Now()
	_, _, _ = FromMarkdown(strings.Repeat(para, maxInput/len(para)), FromOptions{})
	if d := time.Since(start); d > 10*time.Second*slowdown {
		t.Errorf("paragraphs under the bound: %v", d)
	}
	// Escaped text doesn't count, and a paragraph or heading with more
	// marks than that still round-trips, as lk-raw.
	roundTrip(t, listOf(t, `[{"kind":"paragraph","content":[{"text":"`+strings.Repeat("_*[~", maxMarks)+`"}]}]`))
	runs := strings.TrimSuffix(strings.Repeat(`{"text":"a","bold":true},{"text":" "},`, maxMarks+1), ",")
	page := listOf(t, `[{"kind":"paragraph","content":[`+runs+`]},{"kind":"heading","level":1,"content":[`+runs+`]}]`)
	if md := roundTrip(t, page); strings.Count(md, "lk-raw") != 2 {
		t.Fatalf("want two lk-raw blocks:\n%.300s", md)
	}
}
