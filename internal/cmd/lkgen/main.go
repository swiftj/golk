// Command lkgen generates Go wire types from the LegendKeeper OpenAPI 3.1 spec.
//
// It emits types only: one Go type per schema reachable from the selected
// operations, plus a table of those operations' methods and paths. Requests,
// retries and pagination are written by hand in package lk.
//
// Mapping rules:
//   - A required, non-nullable property is a plain field.
//   - An optional property is opt.Optional[T], tagged omitzero.
//   - A nullable property (type [T, "null"], or anyOf [T, null]) is
//     opt.Nullable[T], tagged omitzero.
//   - A oneOf with a discriminator is a raw-preserving union: it keeps the
//     original JSON and exposes the discriminator value, so an unknown kind
//     round-trips unchanged.
//   - allOf is flattened into one struct.
//   - Any other oneOf/anyOf is json.RawMessage, unless every branch maps to the
//     same Go type.
//
// Output is sorted and gofmt'ed, so regenerating from the same spec is
// byte-for-byte stable.
//
// Usage:
//
//	lkgen -spec docs/legendkeeper/openapi.json -tags Resources,Projects -out lk/internal/gen/types.go
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"slices"
	"sort"
	"strings"
	"unicode"
)

const optImport = "github.com/swiftj/golk/lk/internal/opt"

func main() {
	specPath := flag.String("spec", "docs/legendkeeper/openapi.json", "OpenAPI spec path")
	tags := flag.String("tags", "", "comma-separated operation tags to generate")
	outPath := flag.String("out", "", "output file")
	pkg := flag.String("pkg", "gen", "package name")
	flag.Parse()

	raw, err := os.ReadFile(*specPath)
	if err != nil {
		fail(err)
	}
	src, err := Generate(raw, strings.Split(*tags, ","), *pkg)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*outPath, src, 0o644); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "lkgen:", err)
	os.Exit(1)
}

type schema struct {
	Ref                  string             `json:"$ref"`
	Type                 json.RawMessage    `json:"type"`
	Format               string             `json:"format"`
	Enum                 []json.RawMessage  `json:"enum"`
	Properties           map[string]*schema `json:"properties"`
	Required             []string           `json:"required"`
	Items                *schema            `json:"items"`
	AdditionalProperties json.RawMessage    `json:"additionalProperties"`
	OneOf                []*schema          `json:"oneOf"`
	AnyOf                []*schema          `json:"anyOf"`
	AllOf                []*schema          `json:"allOf"`
	Discriminator        *struct {
		PropertyName string            `json:"propertyName"`
		Mapping      map[string]string `json:"mapping"`
	} `json:"discriminator"`
	Description string `json:"description"`
}

// types returns the declared non-null JSON types and whether "null" is one of them.
func (s *schema) types() (ts []string, nullable bool) {
	if len(s.Type) == 0 {
		return nil, false
	}
	var one string
	if json.Unmarshal(s.Type, &one) == nil {
		if one == "null" {
			return nil, true
		}
		return []string{one}, false
	}
	var many []string
	_ = json.Unmarshal(s.Type, &many)
	for _, t := range many {
		if t == "null" {
			nullable = true
		} else {
			ts = append(ts, t)
		}
	}
	return ts, nullable
}

func (s *schema) isNull() bool {
	ts, n := s.types()
	return n && len(ts) == 0
}

func (s *schema) additional() *schema {
	if len(s.AdditionalProperties) == 0 || s.AdditionalProperties[0] != '{' {
		return nil
	}
	var ap schema
	if json.Unmarshal(s.AdditionalProperties, &ap) != nil {
		return nil
	}
	return &ap
}

type spec struct {
	Paths      map[string]map[string]json.RawMessage `json:"paths"`
	Components struct {
		Schemas map[string]*schema `json:"schemas"`
	} `json:"components"`
}

type operation struct {
	OperationID string   `json:"operationId"`
	Tags        []string `json:"tags"`
	RequestBody *struct {
		Content map[string]struct {
			Schema *schema `json:"schema"`
		} `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content map[string]struct {
			Schema *schema `json:"schema"`
		} `json:"content"`
	} `json:"responses"`
}

type opEntry struct{ id, method, path string }

type generator struct {
	spec     *spec
	needed   map[string]bool
	decls    map[string]string
	usesOpt  bool
	usesTime bool
}

// Generate returns the gofmt'ed Go source for the operations with the given tags.
func Generate(raw []byte, tags []string, pkg string) ([]byte, error) {
	var sp spec
	if err := json.Unmarshal(raw, &sp); err != nil {
		return nil, fmt.Errorf("parse spec: %w", err)
	}
	g := &generator{spec: &sp, needed: map[string]bool{}, decls: map[string]string{}}

	var ops []opEntry
	for path, methods := range sp.Paths {
		for method, rawOp := range methods {
			if method == "parameters" {
				continue
			}
			var op operation
			if err := json.Unmarshal(rawOp, &op); err != nil {
				return nil, fmt.Errorf("parse %s %s: %w", method, path, err)
			}
			if len(op.Tags) == 0 || !slices.Contains(tags, op.Tags[0]) {
				continue
			}
			ops = append(ops, opEntry{op.OperationID, strings.ToUpper(method), path})
			if op.RequestBody != nil {
				for _, c := range op.RequestBody.Content {
					g.collect(c.Schema)
				}
			}
			for _, r := range op.Responses {
				for _, c := range r.Content {
					g.collect(c.Schema)
				}
			}
		}
	}
	if len(ops) == 0 {
		return nil, errors.New("no operations match the given tags")
	}
	sort.Slice(ops, func(i, j int) bool { return ops[i].id < ops[j].id })

	names := sortedKeys(g.needed)
	for _, n := range names {
		s := sp.Components.Schemas[n]
		if s == nil {
			return nil, fmt.Errorf("unresolved schema %q", n)
		}
		g.named(goName(n), s)
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "// Code generated by lkgen from docs/legendkeeper/openapi.json. DO NOT EDIT.\n\n")
	fmt.Fprintf(&b, "package %s\n\nimport (\n\t\"encoding/json\"\n", pkg)
	if g.usesTime {
		b.WriteString("\t\"time\"\n")
	}
	if g.usesOpt {
		fmt.Fprintf(&b, "\n\t%q\n", optImport)
	}
	b.WriteString(")\n\n")
	b.WriteString("// Op is one API operation's method and path template.\ntype Op struct {\n\tID     string\n\tMethod string\n\tPath   string\n}\n\n")
	b.WriteString("// Operations generated in this file.\nvar (\n")
	for _, o := range ops {
		fmt.Fprintf(&b, "\tOp%s = Op{ID: %q, Method: %q, Path: %q}\n", goName(o.id), o.id, o.method, o.path)
	}
	b.WriteString(")\n\n")
	for _, n := range sortedKeys(g.decls) {
		b.WriteString(g.decls[n])
		b.WriteString("\n")
	}
	src, err := format.Source(b.Bytes())
	if err != nil {
		return nil, fmt.Errorf("gofmt generated code: %w", err)
	}
	return src, nil
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func (g *generator) resolve(s *schema) *schema {
	if s != nil && s.Ref != "" {
		return g.spec.Components.Schemas[refName(s.Ref)]
	}
	return s
}

func (g *generator) collect(s *schema) {
	if s == nil {
		return
	}
	if s.Ref != "" {
		n := refName(s.Ref)
		if g.needed[n] {
			return
		}
		g.needed[n] = true
		g.collect(g.spec.Components.Schemas[n])
		return
	}
	for _, p := range s.Properties {
		g.collect(p)
	}
	g.collect(s.Items)
	for _, l := range [][]*schema{s.OneOf, s.AnyOf, s.AllOf} {
		for _, c := range l {
			g.collect(c)
		}
	}
	g.collect(s.additional())
}

// flatten merges an allOf into one object schema.
func (g *generator) flatten(s *schema) *schema {
	out := &schema{Description: s.Description, Properties: map[string]*schema{}}
	for _, part := range s.AllOf {
		p := g.resolve(part)
		if len(p.AllOf) > 0 {
			p = g.flatten(p)
		}
		for k, v := range p.Properties {
			out.Properties[k] = v
		}
		out.Required = append(out.Required, p.Required...)
	}
	out.Type = json.RawMessage(`"object"`)
	return out
}

// named emits a top-level declaration for s.
func (g *generator) named(name string, s *schema) {
	if _, ok := g.decls[name]; ok {
		return
	}
	g.decls[name] = "" // reserve against recursion
	if len(s.AllOf) > 0 {
		s = g.flatten(s)
	}
	var b strings.Builder
	if s.Description != "" {
		writeComment(&b, "", name+" "+s.Description)
	}
	ts, _ := s.types()
	switch {
	case s.Discriminator != nil && len(s.OneOf) > 0:
		g.union(&b, name, s)
	case len(s.OneOf) > 0 || len(s.AnyOf) > 0:
		fmt.Fprintf(&b, "type %s = %s\n", name, g.typeOf(name, s))
	case (len(ts) == 1 && ts[0] == "object") || (len(ts) == 0 && len(s.Properties) > 0):
		if len(s.Properties) == 0 {
			fmt.Fprintf(&b, "type %s %s\n", name, g.typeOf(name, s))
			break
		}
		g.object(&b, name, s)
	case len(ts) == 1 && ts[0] == "string" && len(s.Enum) > 0:
		g.enum(&b, name, s)
	default:
		fmt.Fprintf(&b, "type %s %s\n", name, g.typeOf(name+"Item", s))
	}
	g.decls[name] = b.String()
}

func (g *generator) object(b *strings.Builder, name string, s *schema) {
	fmt.Fprintf(b, "type %s struct {\n", name)
	for _, p := range sortedKeys(s.Properties) {
		ps := s.Properties[p]
		field := goName(p)
		t := g.typeOf(name+field, ps)
		tag := p
		switch {
		case g.nullable(ps):
			g.usesOpt = true
			t = "opt.Nullable[" + t + "]"
			tag += ",omitzero"
		case !slices.Contains(s.Required, p):
			g.usesOpt = true
			t = "opt.Optional[" + t + "]"
			tag += ",omitzero"
		}
		if d := g.resolve(ps).Description; d != "" && ps.Ref == "" {
			writeComment(b, "\t", field+" "+d)
		}
		fmt.Fprintf(b, "\t%s %s `json:%q`\n", field, t, tag)
	}
	b.WriteString("}\n")
}

func (g *generator) enum(b *strings.Builder, name string, s *schema) {
	fmt.Fprintf(b, "type %s string\n\nconst (\n", name)
	for _, e := range s.Enum {
		var v string
		if json.Unmarshal(e, &v) != nil {
			continue
		}
		fmt.Fprintf(b, "\t%s%s %s = %q\n", name, goName(v), name, v)
	}
	b.WriteString(")\n")
}

// union emits a discriminated union that keeps its raw JSON and exposes the
// discriminator value. Decoding never fails on an unknown discriminator value.
func (g *generator) union(b *strings.Builder, name string, s *schema) {
	prop := s.Discriminator.PropertyName
	field := goName(prop)
	fmt.Fprintf(b, "type %s struct {\n\t// %s is the %q discriminator value.\n\t%s string\n\t// Raw is the original JSON object.\n\tRaw json.RawMessage\n}\n\n", name, field, prop, field)
	fmt.Fprintf(b, "// UnmarshalJSON records the discriminator and keeps the raw JSON.\nfunc (u *%s) UnmarshalJSON(b []byte) error {\n\tvar d struct {\n\t\tV string `json:%q`\n\t}\n\tif err := json.Unmarshal(b, &d); err != nil {\n\t\treturn err\n\t}\n\tu.%s, u.Raw = d.V, append(json.RawMessage(nil), b...)\n\treturn nil\n}\n\n", name, prop, field)
	fmt.Fprintf(b, "// MarshalJSON returns the raw JSON unchanged.\nfunc (u %s) MarshalJSON() ([]byte, error) {\n\tif u.Raw == nil {\n\t\treturn []byte(\"null\"), nil\n\t}\n\treturn u.Raw, nil\n}\n\n", name)
	fmt.Fprintf(b, "// %sVariants maps each %q value to its variant type name.\nvar %sVariants = map[string]string{\n", name, prop, name)
	for _, k := range sortedKeys(s.Discriminator.Mapping) {
		fmt.Fprintf(b, "\t%q: %q,\n", k, goName(refName(s.Discriminator.Mapping[k])))
	}
	b.WriteString("}\n")
}

func (g *generator) nullable(s *schema) bool {
	if _, n := s.types(); n {
		return true
	}
	for _, c := range s.AnyOf {
		if c.isNull() {
			return true
		}
	}
	return false
}

// typeOf returns the Go type for s, emitting named declarations for inline
// objects and enums under ctx.
func (g *generator) typeOf(ctx string, s *schema) string {
	if s.Ref != "" {
		return goName(refName(s.Ref))
	}
	if len(s.AllOf) > 0 {
		g.named(ctx, s)
		return ctx
	}
	if branches := s.AnyOf; len(branches) > 0 || len(s.OneOf) > 0 {
		if len(branches) == 0 {
			branches = s.OneOf
		}
		var nonNull []*schema
		for _, c := range branches {
			if !c.isNull() {
				nonNull = append(nonNull, c)
			}
		}
		if len(nonNull) == 1 {
			return g.typeOf(ctx, nonNull[0])
		}
		// Collapse branches that are all the same primitive Go type.
		first := ""
		for i, c := range nonNull {
			t := g.primitive(c)
			if t == "" || (i > 0 && t != first) {
				return "json.RawMessage"
			}
			first = t
		}
		return first
	}
	ts, _ := s.types()
	if len(ts) != 1 {
		return "json.RawMessage"
	}
	switch ts[0] {
	case "string":
		if s.Format == "date-time" {
			g.usesTime = true
			return "time.Time"
		}
		if len(s.Enum) > 0 {
			g.named(ctx, s)
			return ctx
		}
		return "string"
	case "integer":
		return "int64"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	case "array":
		if s.Items == nil {
			return "[]json.RawMessage"
		}
		return "[]" + g.typeOf(ctx+"Item", s.Items)
	case "object":
		if len(s.Properties) == 0 {
			if ap := s.additional(); ap != nil {
				return "map[string]" + g.typeOf(ctx+"Value", ap)
			}
			return "json.RawMessage"
		}
		g.named(ctx, s)
		return ctx
	}
	return "json.RawMessage"
}

// primitive returns the Go type of a non-enum primitive schema, or "".
func (g *generator) primitive(s *schema) string {
	s = g.resolve(s)
	ts, _ := s.types()
	if len(ts) != 1 || len(s.Enum) > 0 || s.Format == "date-time" {
		return ""
	}
	switch ts[0] {
	case "string":
		return "string"
	case "integer":
		return "int64"
	case "number":
		return "float64"
	case "boolean":
		return "bool"
	}
	return ""
}

func refName(ref string) string { return ref[strings.LastIndex(ref, "/")+1:] }

var initialisms = map[string]string{
	"api": "API", "css": "CSS", "html": "HTML", "http": "HTTP", "https": "HTTPS",
	"id": "ID", "ids": "IDs", "json": "JSON", "uri": "URI", "url": "URL", "uuid": "UUID",
}

// goName converts a JSON name such as "parentId" or "wrap-left" to "ParentID" or "WrapLeft".
func goName(s string) string {
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range s {
		switch {
		case !unicode.IsLetter(r) && !unicode.IsDigit(r):
			flush()
		case unicode.IsUpper(r) && len(cur) > 0 && !unicode.IsUpper(cur[len(cur)-1]):
			flush()
			cur = append(cur, r)
		default:
			cur = append(cur, r)
		}
	}
	flush()
	var b strings.Builder
	for _, w := range words {
		if up, ok := initialisms[strings.ToLower(w)]; ok {
			b.WriteString(up)
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]))
		b.WriteString(w[1:])
	}
	out := b.String()
	if out == "" || unicode.IsDigit(rune(out[0])) {
		out = "V" + out
	}
	return out
}

func writeComment(b *strings.Builder, indent, text string) {
	line := indent + "//"
	for w := range strings.FieldsSeq(text) {
		if len(line)+1+len(w) > 100 && line != indent+"//" {
			b.WriteString(line)
			b.WriteString("\n")
			line = indent + "//"
		}
		line += " " + w
	}
	b.WriteString(line)
	b.WriteString("\n")
}
