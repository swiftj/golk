package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The checked-in lk/internal/gen/types.go must be exactly what lkgen produces
// from the checked-in spec, so a hand edit or a stale regeneration fails here.
func TestGeneratedFileIsCurrent(t *testing.T) {
	spec, err := os.ReadFile("../../../docs/legendkeeper/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../../lk/internal/gen/types.go")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Generate(spec, genTags(t), "gen")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("lk/internal/gen/types.go is stale or hand-edited; run make generate")
	}
	again, err := Generate(spec, genTags(t), "gen")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, again) {
		t.Fatal("generation is not deterministic")
	}
}

// genTags reads GEN_TAGS from the Makefile so the test and make generate agree.
func genTags(t *testing.T) []string {
	t.Helper()
	mk, err := os.ReadFile("../../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(mk)) {
		if v, ok := strings.CutPrefix(line, "GEN_TAGS"); ok {
			_, v, _ = strings.Cut(v, ":=")
			return strings.Split(strings.TrimSpace(v), ",")
		}
	}
	t.Fatal("GEN_TAGS not found in Makefile")
	return nil
}

const miniSpec = `{
  "openapi": "3.1.0",
  "paths": {
    "/things": {
      "post": {
        "operationId": "createThing",
        "tags": ["Things"],
        "requestBody": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/ThingParams"}}}},
        "responses": {"201": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/ThingDetail"}}}}}
      }
    },
    "/other": {
      "get": {"operationId": "other", "tags": ["Other"], "responses": {"200": {"content": {"application/json": {"schema": {"$ref": "#/components/schemas/Unused"}}}}}}
    }
  },
  "components": {"schemas": {
    "ThingParams": {"type": "object", "required": ["name"], "properties": {
      "name": {"type": "string"},
      "parentId": {"type": ["string", "null"]},
      "size": {"type": "string", "enum": ["small", "x-large"]},
      "when": {"type": "string", "format": "date-time"},
      "either": {"anyOf": [{"type": "string", "maxLength": 3}, {"type": "string", "pattern": "^a"}]},
      "maybe": {"anyOf": [{"$ref": "#/components/schemas/Unit"}, {"type": "null"}]},
      "body": {"$ref": "#/components/schemas/Shape"}
    }},
    "Unit": {"type": "object", "properties": {"n": {"type": "integer"}}},
    "Shape": {"oneOf": [{"$ref": "#/components/schemas/Circle"}], "discriminator": {"propertyName": "kind", "mapping": {"circle": "#/components/schemas/Circle"}}},
    "Circle": {"type": "object", "required": ["kind"], "properties": {"kind": {"type": "string", "enum": ["circle"]}}},
    "ThingDetail": {"allOf": [{"$ref": "#/components/schemas/Unit"}, {"type": "object", "required": ["id"], "properties": {"id": {"type": "string"}}}]},
    "Unused": {"type": "object", "properties": {"x": {"type": "string"}}}
  }}
}`

func TestGenerateMappingRules(t *testing.T) {
	src, err := Generate([]byte(miniSpec), []string{"Things"}, "gen")
	if err != nil {
		t.Fatal(err)
	}
	// Compare with whitespace collapsed so gofmt alignment doesn't matter.
	squash := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	out := squash(string(src))
	for _, want := range []string{
		`OpCreateThing = Op{ID: "createThing", Method: "POST", Path: "/things"}`,
		"Name     string", // required
		"opt.Nullable[string]        `json:\"parentId,omitzero\"`", // type [T, null]
		"opt.Optional[ThingParamsSize]",                            // inline enum
		`ThingParamsSizeXLarge ThingParamsSize = "x-large"`,        // enum constant naming
		"opt.Optional[time.Time]",                                  // date-time
		"opt.Optional[string]        `json:\"either,omitzero\"`",   // same-type anyOf collapses
		"opt.Nullable[Unit]",                                       // anyOf [ref, null]
		"type Shape struct",                                        // discriminated union
		`"circle": "Circle"`,                                       // variant table
		"type ThingDetail struct",                                  // allOf flattened
		"ID string `json:\"id\"`",
		"N opt.Optional[int64] `json:\"n,omitzero\"`",
	} {
		if !strings.Contains(out, squash(want)) {
			t.Errorf("generated code lacks %q\n%s", want, src)
		}
	}
	if strings.Contains(out, "Unused") || strings.Contains(out, "OpOther") {
		t.Error("generated a schema or operation outside the selected tags")
	}
}

func TestGenerateNoMatchingTags(t *testing.T) {
	if _, err := Generate([]byte(miniSpec), []string{"Nope"}, "gen"); err == nil {
		t.Fatal("want error for tags that match no operation")
	}
}

func TestGoName(t *testing.T) {
	for in, want := range map[string]string{
		"parentId":       "ParentID",
		"coverImageUrl":  "CoverImageURL",
		"wrap-left":      "WrapLeft",
		"x-large":        "XLarge",
		"homeResourceId": "HomeResourceID",
		"listResources":  "ListResources",
		"2col":           "V2col",
		"ids":            "IDs",
		"isPublic":       "IsPublic",
	} {
		if got := goName(in); got != want {
			t.Errorf("goName(%q) = %q, want %q", in, got, want)
		}
	}
}
