package markdown

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/swiftj/golk/lk/internal/opt"
)

// attr is one attribute of a directive, an attribute set or an item line:
// name=value, a bare flag, an #id or an @resource (LK_MARKDOWN.md §2.3).
type attr struct {
	name  string // "#" for an id, "@" for a resource
	value string
	flag  bool // a bare name, which means true
}

const (
	attrID       = "#"
	attrResource = "@"
)

func isNameStart(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isNameChar(c byte) bool  { return isNameStart(c) || c >= '0' && c <= '9' }

// isBare reports whether c can appear in a bare (unquoted) value as the
// renderer writes it.
func isBare(c byte) bool {
	return isNameChar(c) || strings.IndexByte("_.:/#%+,;@!?&=~*'$^-", c) >= 0
}

// bareInput is what the parser accepts in a bare value: the bare set and
// any non-ASCII byte.
func bareInput(c byte) bool { return c >= 0x80 || isBare(c) }

// scanAttrs reads attributes from s[i:]. In a set (inSet) they end at '}',
// which must come before the end of the line; otherwise they run to the
// end of the line. It returns the index after the last byte read, or a
// reason.
func scanAttrs(s []byte, i int, inSet, allowAt bool) ([]attr, int, string) {
	if !inSet {
		if nl := slices.Index(s[i:], '\n'); nl >= 0 {
			s = s[:i+nl]
		}
	}
	var out []attr
	for {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
			i++
		}
		if i == len(s) || s[i] == '\n' {
			if inSet {
				return nil, i, "unclosed attribute set"
			}
			return out, i, ""
		}
		if inSet && s[i] == '}' {
			return out, i + 1, ""
		}
		var a attr
		var reason string
		switch c := s[i]; {
		case c == '#':
			a.name = attrID
			a.value, i, reason = scanValue(s, i+1)
		case c == '@' && allowAt:
			a.name = attrResource
			a.value, i, reason = scanValue(s, i+1)
		case isNameStart(c):
			j := i
			for j < len(s) && isNameChar(s[j]) {
				j++
			}
			a.name = string(s[i:j])
			i = j
			if i < len(s) && s[i] == '=' {
				a.value, i, reason = scanValue(s, i+1)
			} else {
				a.flag = true
			}
		default:
			return nil, i, fmt.Sprintf("unexpected %q in attributes", rune(c))
		}
		if reason != "" {
			return nil, i, reason
		}
		if i < len(s) && s[i] != ' ' && s[i] != '\t' && s[i] != '\n' && (!inSet || s[i] != '}') {
			return nil, i, fmt.Sprintf("unexpected %q after attribute %s", rune(s[i]), a.name)
		}
		out = append(out, a)
	}
}

// scanValue reads a bare or double-quoted value at s[i:]. In a quoted value
// a backslash escapes any ASCII punctuation.
func scanValue(s []byte, i int) (string, int, string) {
	if i < len(s) && s[i] == '"' {
		var b strings.Builder
		for i++; i < len(s); i++ {
			c := s[i]
			switch c {
			case '"':
				return b.String(), i + 1, ""
			case '\\':
				if i+1 < len(s) && isASCIIPunct(s[i+1]) {
					b.WriteByte(s[i+1])
					i++
					continue
				}
				return "", i, "invalid escape in quoted value"
			case '\n', '\r':
				return "", i, "unclosed quoted value"
			}
			b.WriteByte(c)
		}
		return "", i, "unclosed quoted value"
	}
	j := i
	for j < len(s) && bareInput(s[j]) {
		j++
	}
	if j == i {
		return "", i, "missing attribute value"
	}
	return string(s[i:j]), j, ""
}

// scanAttrSet reads an attribute set {…} at the start of b, which must end
// on the same line. ok is false when b does not start with one.
func scanAttrSet(b []byte) (attrs []attr, n int, ok bool) {
	if len(b) == 0 || b[0] != '{' {
		return nil, 0, false
	}
	attrs, n, reason := scanAttrs(b, 1, true, false)
	return attrs, n, reason == ""
}

func isASCIIPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

// fmtValue writes v as a bare token when it can and quoted otherwise. ok is
// false for a value an attribute can't hold (a control character other
// than a tab).
func fmtValue(v string) (string, bool) {
	bare := v != ""
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c < 0x20 && c != '\t' || c == 0x7f {
			return "", false
		}
		if !isBare(c) {
			bare = false
		}
	}
	if bare {
		return v, true
	}
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		if c := v[i]; c == '"' || c == '\\' || c == '|' {
			b.WriteByte('\\')
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('"')
	return b.String(), true
}

func fmtFloat(f float64) (string, bool) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", false
	}
	return strconv.FormatFloat(f, 'g', -1, 64), true
}

// attrW writes attributes in the order they are added.
type attrW struct {
	parts []string
	ok    bool
}

func newAttrW() *attrW { return &attrW{ok: true} }

func (w *attrW) raw(s string) { w.parts = append(w.parts, s) }

func (w *attrW) value(prefix, v string) {
	f, ok := fmtValue(v)
	if !ok {
		w.ok = false
		return
	}
	w.raw(prefix + f)
}

// id writes #id when on and the id is set.
func (w *attrW) id(on bool, id string) {
	if on && id != "" {
		w.value(attrID, id)
	}
}

func (w *attrW) str(name, v string) { w.value(name+"=", v) }

func (w *attrW) optStr(name string, o opt.Optional[string]) {
	if v, ok := o.Get(); ok {
		w.str(name, v)
	}
}

func (w *attrW) boolean(name string, v bool) {
	if v {
		w.raw(name)
	} else {
		w.raw(name + "=false")
	}
}

func (w *attrW) optBool(name string, o opt.Optional[bool]) {
	if v, ok := o.Get(); ok {
		w.boolean(name, v)
	}
}

func (w *attrW) optInt(name string, o opt.Optional[int]) {
	if v, ok := o.Get(); ok {
		w.raw(name + "=" + strconv.Itoa(v))
	}
}

func (w *attrW) float(name string, v float64) {
	f, ok := fmtFloat(v)
	if !ok {
		w.ok = false
		return
	}
	w.raw(name + "=" + f)
}

func (w *attrW) optFloat(name string, o opt.Optional[float64]) {
	if v, ok := o.Get(); ok {
		w.float(name, v)
	}
}

// enum writes a required enum value, which must be one of valid.
func enum[T ~string](w *attrW, name string, v T, valid []T) {
	if !slices.Contains(valid, v) {
		w.ok = false
		return
	}
	w.str(name, string(v))
}

func optEnum[T ~string](w *attrW, name string, o opt.Optional[T], valid []T) {
	if v, ok := o.Get(); ok {
		enum(w, name, v, valid)
	}
}

func (w *attrW) String() string { return strings.Join(w.parts, " ") }

// attrR reads the attributes of one construct and rejects unknown,
// duplicate and malformed ones (M-SAFE-01).
type attrR struct {
	c    *conv
	off  int // source offset, for the error line
	what string
	list []attr
	used []bool
}

func (c *conv) attrReader(list []attr, off int, what string) (*attrR, error) {
	a := &attrR{c: c, off: off, what: what, list: list, used: make([]bool, len(list))}
	for i, x := range list {
		for _, y := range list[:i] {
			if x.name == y.name {
				return nil, c.errf(off, "duplicate attribute %s on %s", showName(x.name), what)
			}
		}
	}
	return a, nil
}

func showName(n string) string {
	switch n {
	case attrID:
		return "#id"
	case attrResource:
		return "@resource"
	}
	return n
}

func (a *attrR) find(name string) (attr, bool) {
	for i, x := range a.list {
		if x.name == name {
			a.used[i] = true
			return x, true
		}
	}
	return attr{}, false
}

func (a *attrR) errf(format string, args ...any) error {
	return a.c.errf(a.off, "%s: %s", a.what, fmt.Sprintf(format, args...))
}

// id returns the #id attribute, or "".
func (a *attrR) id() string {
	x, _ := a.find(attrID)
	return x.value
}

func (a *attrR) str(name string) (opt.Optional[string], error) {
	x, ok := a.find(name)
	if !ok {
		return opt.Optional[string]{}, nil
	}
	if x.flag {
		return opt.Optional[string]{}, a.errf("attribute %s needs a value", name)
	}
	return opt.Some(x.value), nil
}

func (a *attrR) reqStr(name string) (string, error) {
	v, err := a.str(name)
	if err != nil {
		return "", err
	}
	s, ok := v.Get()
	if !ok {
		return "", a.errf("missing attribute %s", name)
	}
	return s, nil
}

func (a *attrR) boolean(name string) (opt.Optional[bool], error) {
	x, ok := a.find(name)
	switch {
	case !ok:
		return opt.Optional[bool]{}, nil
	case x.flag || x.value == "true":
		return opt.Some(true), nil
	case x.value == "false":
		return opt.Some(false), nil
	}
	return opt.Optional[bool]{}, a.errf("attribute %s must be true or false, not %q", name, x.value)
}

var intRE = regexp.MustCompile(`^-?[0-9]+$`)

func (a *attrR) integer(name string) (opt.Optional[int], error) {
	v, err := a.str(name)
	if s, ok := v.Get(); err == nil && ok {
		n, perr := strconv.Atoi(s)
		if !intRE.MatchString(s) || perr != nil {
			return v2none[int](), a.errf("attribute %s must be an integer, not %q", name, s)
		}
		return opt.Some(n), nil
	}
	return v2none[int](), err
}

var floatRE = regexp.MustCompile(`^-?([0-9]+(\.[0-9]*)?|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// parseFloat reads a finite decimal number.
func parseFloat(s string) (float64, bool) {
	if !floatRE.MatchString(s) {
		return 0, false
	}
	f, err := strconv.ParseFloat(s, 64)
	return f, err == nil && !math.IsInf(f, 0)
}

func (a *attrR) float(name string) (opt.Optional[float64], error) {
	v, err := a.str(name)
	if s, ok := v.Get(); err == nil && ok {
		f, ok := parseFloat(s)
		if !ok {
			return v2none[float64](), a.errf("attribute %s must be a number, not %q", name, s)
		}
		return opt.Some(f), nil
	}
	return v2none[float64](), err
}

func v2none[T any]() opt.Optional[T] { return opt.Optional[T]{} }

// readEnum reads an enum attribute, which must be one of valid.
func readEnum[T ~string](a *attrR, name string, valid []T) (opt.Optional[T], error) {
	v, err := a.str(name)
	if s, ok := v.Get(); err == nil && ok {
		if !slices.Contains(valid, T(s)) {
			return opt.Optional[T]{}, a.errf("attribute %s: %q is not one of %s", name, s, joinEnum(valid))
		}
		return opt.Some(T(s)), nil
	}
	return opt.Optional[T]{}, err
}

func reqEnum[T ~string](a *attrR, name string, valid []T) (T, error) {
	v, err := readEnum(a, name, valid)
	if err != nil {
		return "", err
	}
	t, ok := v.Get()
	if !ok {
		return "", a.errf("missing attribute %s", name)
	}
	return t, nil
}

func joinEnum[T ~string](valid []T) string {
	s := make([]string, len(valid))
	for i, v := range valid {
		s[i] = string(v)
	}
	return strings.Join(s, ", ")
}

// done fails on any attribute not read.
func (a *attrR) done() error {
	for i, x := range a.list {
		if !a.used[i] {
			return a.errf("unknown attribute %s", showName(x.name))
		}
	}
	return nil
}
