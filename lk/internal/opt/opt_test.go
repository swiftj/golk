package opt

import (
	"encoding/json"
	"testing"
)

type body struct {
	O Optional[string]         `json:"o,omitzero"`
	N Nullable[string]         `json:"n,omitzero"`
	S Optional[[]int]          `json:"s,omitzero"`
	M Optional[map[string]int] `json:"m,omitzero"`
}

func TestEncode(t *testing.T) {
	for _, tc := range []struct {
		in   body
		want string
	}{
		{body{}, `{}`},
		{body{O: Some("")}, `{"o":""}`},
		{body{N: Null[string]()}, `{"n":null}`},
		{body{N: Value("x")}, `{"n":"x"}`},
		{body{S: Some[[]int](nil), M: Some[map[string]int](nil)}, `{"s":[],"m":{}}`},
		{body{S: Some([]int{1})}, `{"s":[1]}`},
	} {
		b, err := json.Marshal(tc.in)
		if err != nil || string(b) != tc.want {
			t.Errorf("got %s %v, want %s", b, err, tc.want)
		}
	}
}

func TestDecode(t *testing.T) {
	var b body
	if err := json.Unmarshal([]byte(`{"o":"x","n":null}`), &b); err != nil {
		t.Fatal(err)
	}
	if v, ok := b.O.Get(); !ok || v != "x" || !b.N.IsNull() || b.N.IsSet() || b.S.IsSet() {
		t.Fatalf("%+v", b)
	}
	// Optional has no null state: null decodes as unset.
	b = body{}
	_ = json.Unmarshal([]byte(`{"o":null,"n":"y"}`), &b)
	if b.O.IsSet() || b.N.Or("") != "y" {
		t.Fatalf("%+v", b)
	}
}
