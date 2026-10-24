package mcpserver

import (
	"strings"
	"sync"
	"testing"

	"github.com/swiftj/golk/lk"
)

func TestProjects_X_PROJ_01_Resolve(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		arg     string
		want    string
		wantErr []string
	}{
		{name: "none configured", keys: nil, arg: "", wantErr: []string{"no LegendKeeper project is configured", "LEGENDKEEPER_API_KEY"}},
		{name: "none configured, named", keys: nil, arg: "wiki", wantErr: []string{"no LegendKeeper project is configured"}},
		{name: "default", keys: []string{"default", "second"}, arg: "", want: "default"},
		{name: "default by name", keys: []string{"default", "second"}, arg: "default", want: "default"},
		{name: "named", keys: []string{"default", "second"}, arg: "second", want: "second"},
		{name: "named, case-insensitive", keys: []string{"default", "second"}, arg: "SeCoNd", want: "second"},
		{name: "only project, not default", keys: []string{"sandbox"}, arg: "", want: "sandbox"},
		{name: "several without default", keys: []string{"beta", "alpha"}, arg: "", wantErr: []string{"several projects", "alpha, beta", "pass project"}},
		{name: "unknown", keys: []string{"default", "second"}, arg: "third", wantErr: []string{`unknown project "third"`, "default, second"}},
		{name: "unknown default", keys: []string{"sandbox"}, arg: "default", wantErr: []string{`unknown project "default"`, "sandbox"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{}
			for _, k := range tt.keys {
				cfg = cfg.WithKey(k, "made-up-key-"+k)
			}
			got, err := newProjects(cfg, nil).resolve(tt.arg)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("resolve(%q) = %q, want an error", tt.arg, got)
				}
				for _, w := range tt.wantErr {
					if !strings.Contains(err.Error(), w) {
						t.Errorf("error %q lacks %q", err, w)
					}
				}
				if strings.Contains(err.Error(), "made-up-key") {
					t.Errorf("error holds a key: %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("resolve(%q) = %q, %v; want %q", tt.arg, got, err, tt.want)
			}
		})
	}
}

func TestProjects_X_PROJ_02_OneClientPerProject(t *testing.T) {
	cfg := Config{}.WithKey(DefaultProject, "made-up-1").WithKey("second", "made-up-2")
	p := newProjects(cfg, []lk.Option{lk.WithRateLimiter(nil)})
	first, name, err := p.client("")
	if err != nil || name != DefaultProject {
		t.Fatalf("client(\"\") = %v, %q, %v", first, name, err)
	}
	var wg sync.WaitGroup
	got := make([]*lk.Client, 50)
	for i := range got {
		wg.Go(func() {
			arg := []string{"", "default", "DEFAULT"}[i%3]
			c, _, err := p.client(arg)
			if err != nil {
				t.Error(err)
			}
			got[i] = c
		})
	}
	wg.Wait()
	for i, c := range got {
		if c != first {
			t.Fatalf("call %d got another client", i)
		}
	}
	second, _, err := p.client("Second")
	if err != nil {
		t.Fatal(err)
	}
	if second == first {
		t.Error("two projects share a client")
	}
	if again, _, _ := p.client("second"); again != second {
		t.Error("the second project's client was not reused")
	}
	if len(p.clients) != 2 {
		t.Errorf("%d clients made, want 2", len(p.clients))
	}
}
