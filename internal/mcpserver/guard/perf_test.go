package guard

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/lk/blocks"
)

// perfFixture builds about 1 MiB of page (paragraphs, every tenth a
// secret) and a terms file of 200 made-up terms, one of which appears once
// in public text.
func perfFixture(tb testing.TB) (*Guard, blocks.List) {
	tb.Helper()
	var terms strings.Builder
	for i := range 200 {
		if i%5 == 0 {
			fmt.Fprintf(&terms, "the Vorn%03d Accord\n", i)
		} else {
			fmt.Fprintf(&terms, "Qelthar%03d\n", i)
		}
	}
	g := Open(writeTerms(tb, terms.String()))
	sentence := "The caravan left Mirevale at dawn, carrying salt, iron and rumours of the north. "
	var l blocks.List
	size := 0
	for i := 0; size < 1<<20; i++ {
		text := strings.Repeat(sentence, 8)
		if i == 501 { // a public paragraph
			text += "Qelthar007"
		}
		if i%10 == 0 {
			l = append(l, blocks.Secret(blocks.P(text+" Qelthar001")))
		} else {
			l = append(l, blocks.P(text))
		}
		size += len(text)
	}
	return g, l
}

func TestGuard_Perf_1MiB_200Terms(t *testing.T) {
	g, l := perfFixture(t)
	if _, err := g.Check(l); err != nil { // load the terms file
		t.Fatal(err)
	}
	start := time.Now()
	vs, err := g.Check(l)
	el := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Term != "Qelthar007" {
		t.Fatalf("violations = %v", vs)
	}
	t.Logf("1 MiB, 200 terms: %v", el)
	if bound := 100 * time.Millisecond * slowdown; el > bound {
		t.Fatalf("check took %v, want < %v", el, bound)
	}
}

func BenchmarkGuardCheck1MiB(b *testing.B) {
	g, l := perfFixture(b)
	if _, err := g.Check(l); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(1 << 20)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := g.Check(l); err != nil {
			b.Fatal(err)
		}
	}
}
