package markdown

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/swiftj/golk/lk/blocks"
	"github.com/yuin/goldmark/util"
)

// Enum values the directives and attribute sets accept (M-SAFE-01). Values
// outside these lists are kept only inside lk-raw.
var (
	aligns    = []blocks.Align{blocks.AlignLeft, blocks.AlignCenter, blocks.AlignRight}
	layouts   = []blocks.BlockLayout{blocks.LayoutDefault, blocks.LayoutWrapLeft, blocks.LayoutWrapRight, blocks.LayoutCenter, blocks.LayoutAlignStart, blocks.LayoutAlignEnd}
	tones     = []blocks.Tone{blocks.ToneInfo, blocks.ToneNote, blocks.ToneSuccess, blocks.ToneWarning, blocks.ToneError, blocks.ToneCustom}
	providers = []blocks.EmbedProvider{blocks.EmbedYouTube, blocks.EmbedSpotify, blocks.EmbedSoundCloud}
	displays  = []blocks.KeyValueDisplay{blocks.DisplayList, blocks.DisplayGrid, blocks.DisplayInline}
	modes     = []blocks.MeterMode{blocks.MeterBar, blocks.MeterWheel, blocks.MeterGauge180, blocks.MeterGauge270, blocks.MeterPool, blocks.MeterRating}
)

// decodeText resolves backslash escapes and entity references in raw inline
// text, as CommonMark does: an escaped '&' does not start a reference.
func decodeText(raw []byte) string {
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '\\' && i+1 < len(raw) && util.IsPunct(raw[i+1]) {
			b.WriteByte(raw[i+1])
			i++
			continue
		}
		if c == '&' {
			if s, n := entityAt(raw[i:]); n > 0 {
				b.WriteString(s)
				i += n - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// entityAt decodes an entity or numeric character reference at the start of
// b, returning its text and length, or n == 0.
func entityAt(b []byte) (string, int) {
	if len(b) < 3 || b[0] != '&' {
		return "", 0
	}
	if b[1] == '#' {
		i, base, maxDigits := 2, 10, 7
		if i < len(b) && (b[i] == 'x' || b[i] == 'X') {
			i, base, maxDigits = 3, 16, 6
		}
		j := i
		for j < len(b) && (base == 10 && util.IsNumeric(b[j]) || base == 16 && util.IsHexDecimal(b[j])) {
			j++
		}
		if j == i || j-i > maxDigits || j >= len(b) || b[j] != ';' {
			return "", 0
		}
		v, err := strconv.ParseUint(string(b[i:j]), base, 32)
		if err != nil {
			return "", 0
		}
		return string(util.ToValidRune(rune(v))), j + 1
	}
	j := 1
	for j < len(b) && util.IsAlphaNumeric(b[j]) {
		j++
	}
	if j == 1 || j >= len(b) || b[j] != ';' {
		return "", 0
	}
	e, ok := util.LookUpHTML5EntityByName(string(b[1:j]))
	if !ok {
		return "", 0
	}
	return string(e.Characters), j + 1
}

// textOK reports whether s can be written as Markdown text: valid UTF-8 and
// no control character other than a tab.
func textOK(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 && c != '\t' || c == 0x7f {
			return false
		}
	}
	return true
}

// escCtx says where escaped text sits.
type escCtx struct {
	lineStart bool // first thing on a line: block syntax could start here
	lineEnd   bool // last thing on a line: trailing whitespace is trimmed
	heading   bool // inside a heading: '#' could close it
	colons    bool // a row key or meter name: "::" separates the value
	bang      bool // a '[' follows: a trailing '!' would start an image
}

func isEdgeSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\v' || c == '\f' }

// escapeText writes s so that it parses back as exactly s (M-INL-01).
func escapeText(s string, ctx escCtx) string {
	var b strings.Builder
	lead := 0
	if ctx.lineStart {
		for lead < len(s) && isEdgeSpace(s[lead]) {
			lead++
		}
	}
	trail := len(s)
	if ctx.lineEnd {
		for trail > lead && isEdgeSpace(s[trail-1]) {
			trail--
		}
	}
	for i := range lead {
		b.WriteString("&#" + strconv.Itoa(int(s[i])) + ";")
	}
	digitsEnd := -1
	if ctx.lineStart && lead == 0 {
		j := 0
		for j < len(s) && s[j] >= '0' && s[j] <= '9' {
			j++
		}
		if j > 0 && j < len(s) && (s[j] == '.' || s[j] == ')') {
			digitsEnd = j
		}
	}
	for i := lead; i < trail; i++ {
		c := s[i]
		switch {
		case strings.IndexByte("\\`*_~[]{}<|", c) >= 0,
			c == '&' && i+1 < len(s) && (s[i+1] == '#' || isNameStart(s[i+1])),
			c == '#' && ctx.heading,
			c == ':' && ctx.colons,
			c == '!' && ctx.bang && i == len(s)-1,
			i == digitsEnd,
			i == 0 && ctx.lineStart && strings.IndexByte("#>-+=:", c) >= 0:
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	for i := trail; i < len(s); i++ {
		if i >= lead {
			b.WriteString("&#" + strconv.Itoa(int(s[i])) + ";")
		}
	}
	return b.String()
}

// codeSpan writes s as a code span. In a table cell a '|' is escaped, which
// GFM removes again inside code.
func codeSpan(s string, cell bool) string {
	run, longest := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	if cell {
		s = strings.ReplaceAll(s, "|", `\|`)
	}
	fence := strings.Repeat("`", longest+1)
	pad := s[0] == '`' || s[len(s)-1] == '`' ||
		s[0] == ' ' && s[len(s)-1] == ' ' && strings.Trim(s, " ") != ""
	if pad {
		return fence + " " + s + " " + fence
	}
	return fence + s + fence
}

// linkDest writes a link or image destination. ok is false for one that
// can't be written (a control character).
func linkDest(u string) (string, bool) {
	if !textOK(u) || strings.ContainsRune(u, '\t') {
		return "", false
	}
	if u == "" {
		return "<>", true
	}
	pointy := strings.ContainsRune(u, ' ')
	var b strings.Builder
	if pointy {
		b.WriteByte('<')
	}
	for i := 0; i < len(u); i++ {
		c := u[i]
		esc := strings.IndexByte(`<>\|`, c) >= 0 ||
			c == '&' && i+1 < len(u) && (u[i+1] == '#' || isNameStart(u[i+1])) ||
			!pointy && (c == '(' || c == ')')
		if esc {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	if pointy {
		b.WriteByte('>')
	}
	return b.String(), true
}
