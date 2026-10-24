package blocks

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// roundTrip checks that whatever decodes re-encodes to JSON that decodes and
// encodes to the same bytes again: after one pass the encoding is stable,
// and decoding never panics.
func roundTrip[T any](t *testing.T, data []byte) {
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return
	}
	once, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encode after decode: %v\ninput %s", err, data)
	}
	var w T
	if err := json.Unmarshal(once, &w); err != nil {
		t.Fatalf("re-decode: %v\nencoded %s", err, once)
	}
	twice, err := json.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(once, twice) {
		t.Fatalf("unstable encoding\nonce  %s\ntwice %s", once, twice)
	}
}

func seedGolden(f *testing.F) {
	data, err := os.ReadFile("testdata/plaintext_golden.json")
	if err != nil {
		f.Fatal(err)
	}
	var g struct {
		Blocks json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(data, &g); err != nil {
		f.Fatal(err)
	}
	f.Add([]byte(g.Blocks))
}

func FuzzDecodeBlocks(f *testing.F) {
	seedGolden(f)
	for _, s := range []string{
		`[]`, `null`, `[{"kind":"hologram","x":1}]`, `[{"kind":"unsupported","id":"u"}]`,
		`[{"kind":"paragraph","content":[{"text":"a","x":1}],"y":[1]}]`,
		`[{"kind":"quote","children":[{"kind":"quote","children":[]}]}]`,
		`[{"kind":"infobox","rows":[{"kind":"textField","children":[{"kind":"rule"}]},{"kind":"z"}]}]`,
		`[{"kind":"resourceCollection","filter":{"type":"filter","id":"f","filterType":"composite","operator":"and","filters":[{"type":"filter","id":"g","filterType":"text","value":"x"}]}}]`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) { roundTrip[List](t, data) })
}

func FuzzDecodeRuns(f *testing.F) {
	for _, s := range []string{
		`[{"text":"a","bold":true}]`, `[{"mention":{"resourceId":"r","name":"n"},"color":"#000000"}]`,
		`[{"break":true}]`, `[{"break":true,"x":1}]`, `[{"mention":null}]`, `[{}]`, `[1,"x",null]`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) { roundTrip[Runs](t, data) })
}

func FuzzDecodeProperties(f *testing.F) {
	for _, s := range []string{
		`[{"kind":"keyValue","id":"k","rank":"a","rows":[{"key":"k","text":"v"}]}]`,
		`[{"kind":"unsupported","id":"u","isHidden":true}]`, `[{"kind":"chart"}]`,
		`[{"kind":"map","viewport":{"center":[1,2],"zoom":3}}]`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) { roundTrip[Properties](t, data) })
}

func FuzzDecodeFilter(f *testing.F) {
	for _, s := range []string{
		`{"type":"filter","id":"f","filterType":"field","field":"parentId","operator":"equals","value":"$self"}`,
		`{"type":"filter","id":"f","filterType":"field","operator":"in","value":[1,"a",null]}`,
		`{"type":"filter","id":"f","filterType":"composite","operator":"not","filters":[{"type":"filter","id":"g","filterType":"tag","operator":"has_all","tags":[]}]}`,
		`{"id":"f","filterType":"custom","params":{"a":1}}`,
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) { roundTrip[Filter](t, data) })
}
