package guard

import (
	"strings"
	"unicode"
)

// Matching rules (X-GUARD-02, D-012):
//
//   - Both the term and the text are normalized rune by rune: each rune is
//     replaced by the smallest rune of its Unicode simple-folding orbit
//     (so a and A, Æ and æ, K and the Kelvin sign all compare equal; this
//     is strings.EqualFold's equivalence), and format characters (Unicode
//     category Cf: soft hyphen, zero-width space and joiners, BOM) are
//     dropped, because they are invisible to the reader.
//   - A term is split into words on Unicode white space. Its words must
//     appear in order in the text, separated by one or more Unicode
//     white-space runes (newlines, tabs, no-break spaces, ...).
//   - Word runes are letters and digits (unicode.IsLetter, unicode.IsDigit).
//     Everything else, including '_', apostrophes, punctuation and
//     combining marks, is a boundary. When the term's first rune is a word
//     rune, the rune before the match must be the string start or a
//     non-word rune; likewise for the term's last rune and the rune after
//     the match. So "Zorblax" matches "Zorblax's", "(Zorblax)", "Zorblax."
//     and "Zorblax_2" but not "Zorblaxian" or "AZorblax".
//   - Matching is per string; a term split across two strings is not found.
//   - A term that normalizes to nothing is ignored, and terms that
//     normalize to the same words are reported once, as first written.

// term is one protected term, normalized.
type term struct {
	raw   string
	words [][]rune
	first rune // words[0][0]
}

// matcher holds the normalized terms of one terms-file load. It is
// immutable and shared between concurrent checks.
type matcher struct {
	terms []term
	ascii [128][]int     // term indexes by first rune, first rune < 128
	other map[rune][]int // term indexes by first rune, otherwise
}

func newMatcher(raw []string) *matcher {
	m := &matcher{other: map[rune][]int{}}
	seen := map[string]bool{}
	for _, r := range raw {
		var words [][]rune
		for _, w := range strings.Fields(r) {
			if f := normalize(nil, w); len(f) > 0 {
				words = append(words, f)
			}
		}
		if len(words) == 0 {
			continue
		}
		key := keyOf(words)
		if seen[key] {
			continue
		}
		seen[key] = true
		i := len(m.terms)
		t := term{raw: r, words: words, first: words[0][0]}
		m.terms = append(m.terms, t)
		if t.first < 128 {
			m.ascii[t.first] = append(m.ascii[t.first], i)
		} else {
			m.other[t.first] = append(m.other[t.first], i)
		}
	}
	return m
}

func keyOf(words [][]rune) string {
	var b strings.Builder
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(string(w))
	}
	return b.String()
}

// fold maps r to the smallest rune of its simple-folding orbit.
func fold(r rune) rune {
	if r < 0x80 {
		if 'a' <= r && r <= 'z' {
			return r - 'a' + 'A'
		}
		return r
	}
	min := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < min {
			min = f
		}
	}
	return min
}

// normalize appends the normalized runes of s to dst.
func normalize(dst []rune, s string) []rune {
	for _, r := range s {
		if r >= 0x80 && unicode.Is(unicode.Cf, r) {
			continue
		}
		dst = append(dst, fold(r))
	}
	return dst
}

func isWord(r rune) bool {
	if r < 0x80 {
		return 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9'
	}
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func isSpace(r rune) bool {
	if r < 0x80 {
		return r == ' ' || '\t' <= r && r <= '\r'
	}
	return unicode.IsSpace(r)
}

// scan reports, in term order, the index of every term found in the
// normalized text. hit is scratch space of len(m.terms), all false on
// entry and on return.
func (m *matcher) scan(text []rune, hit []bool, out []int) []int {
	found := 0
	for i, r := range text {
		var cands []int
		if r < 128 {
			cands = m.ascii[r]
		} else {
			cands = m.other[r]
		}
		if len(cands) == 0 {
			continue
		}
		startOK := i == 0 || !isWord(text[i-1])
		for _, ti := range cands {
			if hit[ti] {
				continue
			}
			t := &m.terms[ti]
			if isWord(t.first) && !startOK {
				continue
			}
			if t.matchAt(text, i) {
				hit[ti] = true
				found++
			}
		}
		if found == len(m.terms) {
			break
		}
	}
	for ti := range hit {
		if hit[ti] {
			out = append(out, ti)
			hit[ti] = false
		}
	}
	return out
}

// matchAt reports whether t occurs in text starting at i, with a boundary
// after it when its last rune is a word rune.
func (t *term) matchAt(text []rune, i int) bool {
	j := i
	for wi, w := range t.words {
		if wi > 0 {
			k := j
			for k < len(text) && isSpace(text[k]) {
				k++
			}
			if k == j {
				return false
			}
			j = k
		}
		if len(text)-j < len(w) {
			return false
		}
		for _, r := range w {
			if text[j] != r {
				return false
			}
			j++
		}
	}
	last := t.words[len(t.words)-1]
	if isWord(last[len(last)-1]) && j < len(text) && isWord(text[j]) {
		return false
	}
	return true
}

// Matches reports whether term occurs in text under the guard's matching
// rules (see the comment at the top of match.go). It is the single-string
// form of what Check does, for tests and for tools that want to explain a
// refusal.
func Matches(term, text string) bool {
	m := newMatcher([]string{term})
	if len(m.terms) == 0 {
		return false
	}
	return len(m.scan(normalize(nil, text), make([]bool, 1), nil)) > 0
}

// Normalize returns s under the guard's normalization: simple case folding
// to a canonical rune per orbit, with format (Cf) characters removed.
func Normalize(s string) string { return string(normalize(nil, s)) }
