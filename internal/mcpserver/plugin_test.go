package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The Claude Desktop extension and Cowork plugin in plugin/ (Phase 6,
// DECISIONS.md D-016).

const pluginDir = "../../plugin"

type mcpbManifest struct {
	ManifestVersion string `json:"manifest_version"`
	Name            string `json:"name"`
	Version         string `json:"version"`
	Server          struct {
		Type       string `json:"type"`
		EntryPoint string `json:"entry_point"`
		MCPConfig  struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
		} `json:"mcp_config"`
	} `json:"server"`
	Compatibility struct {
		Platforms []string `json:"platforms"`
	} `json:"compatibility"`
	UserConfig map[string]struct {
		Type      string `json:"type"`
		Required  bool   `json:"required"`
		Sensitive bool   `json:"sensitive"`
		Default   any    `json:"default"`
	} `json:"user_config"`
}

func readManifest(t *testing.T) mcpbManifest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(pluginDir, "mcpb", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m mcpbManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

var userConfigRef = regexp.MustCompile(`\$\{user_config\.([a-z_]+)\}`)

// substitute does what Claude Desktop does to the manifest's env (the mcpb
// package's getMcpConfigForManifest): defaults, then the user's values;
// booleans become "true"/"false"; a reference with neither stays as
// written.
func substitute(t *testing.T, m mcpbManifest, user map[string]any) []string {
	t.Helper()
	vals := map[string]string{}
	for k, uc := range m.UserConfig {
		if uc.Default != nil {
			vals[k] = configString(uc.Default)
		}
	}
	for k, v := range user {
		vals[k] = configString(v)
	}
	var env []string
	for k, v := range m.Server.MCPConfig.Env {
		env = append(env, k+"="+userConfigRef.ReplaceAllStringFunc(v, func(ref string) string {
			if s, ok := vals[userConfigRef.FindStringSubmatch(ref)[1]]; ok {
				return s
			}
			return ref
		}))
	}
	slices.Sort(env)
	return env
}

func configString(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "true"
		}
		return "false"
	case string:
		return x
	}
	b, _ := json.Marshal(v)
	return string(b)
}

// X-PLUG-01: the extension runs the bundled binary on macOS, takes the key
// as a required, masked setting, and passes only variables lk-mcp reads.
// Every setting it references is required or has a default, so Desktop
// never passes a literal ${user_config.…}.
func TestPlugin_X_PLUG_01_Manifest(t *testing.T) {
	m := readManifest(t)
	if m.ManifestVersion != "0.3" || m.Server.Type != "binary" || m.Server.EntryPoint != "server/lk-mcp" ||
		m.Server.MCPConfig.Command != "${__dirname}/server/lk-mcp" || len(m.Server.MCPConfig.Args) != 0 {
		t.Errorf("server = %+v (manifest %s)", m.Server, m.ManifestVersion)
	}
	if !slices.Equal(m.Compatibility.Platforms, []string{"darwin"}) {
		t.Errorf("platforms = %v, want darwin only (D-013)", m.Compatibility.Platforms)
	}
	if k := m.UserConfig["api_key"]; k.Type != "string" || !k.Required || !k.Sensitive {
		t.Errorf("api_key = %+v, want a required, sensitive string", k)
	}
	read := map[string]bool{"LEGENDKEEPER_API_KEY": true, "LK_MCP_GUARD_FILE": true, "LK_MCP_GUARD": true, "LK_MCP_READ_ONLY": true}
	for k, v := range m.Server.MCPConfig.Env {
		if !read[k] {
			t.Errorf("env %s is not one lk-mcp reads", k)
		}
		for _, ref := range userConfigRef.FindAllStringSubmatch(v, -1) {
			uc, ok := m.UserConfig[ref[1]]
			if !ok {
				t.Errorf("env %s references undeclared setting %s", k, ref[1])
			} else if !uc.Required && uc.Default == nil {
				t.Errorf("setting %s is optional without a default: Desktop would pass %q literally", ref[1], ref[0])
			}
		}
	}
	for _, env := range substitute(t, m, map[string]any{"api_key": "made-up-key"}) {
		if strings.Contains(env, "${") {
			t.Errorf("unsubstituted %s", env)
		}
	}
}

// X-PLUG-02: each combination of settings, as Desktop substitutes it, is
// the intended config.
func TestPlugin_X_PLUG_02_SettingsToConfig(t *testing.T) {
	m := readManifest(t)
	for name, tc := range map[string]struct {
		user               map[string]any
		guardOff, readOnly bool
		guardFile          string
	}{
		"defaults":       {user: map[string]any{"api_key": "made-up-key"}},
		"terms file":     {user: map[string]any{"api_key": "made-up-key", "guard_file": "/elsewhere/terms.txt"}, guardFile: "/elsewhere/terms.txt"},
		"guard off":      {user: map[string]any{"api_key": "made-up-key", "spoiler_guard": false}, guardOff: true},
		"read-only":      {user: map[string]any{"api_key": "made-up-key", "read_only": true}, readOnly: true},
		"read-only, off": {user: map[string]any{"api_key": "made-up-key", "read_only": false, "spoiler_guard": true}},
	} {
		t.Run(name, func(t *testing.T) {
			c, err := ConfigFromEnv(substitute(t, m, tc.user))
			if err != nil {
				t.Fatal(err)
			}
			if c.GuardOff != tc.guardOff || c.ReadOnly != tc.readOnly || c.GuardFile != tc.guardFile ||
				!slices.Equal(c.Projects(), []string{DefaultProject}) {
				t.Errorf("config = %v, want guardOff=%t readOnly=%t file=%q", c, tc.guardOff, tc.readOnly, tc.guardFile)
			}
		})
	}
}

type skillDoc struct {
	dir, name, description, body string
}

func readSkills(t *testing.T) []skillDoc {
	t.Helper()
	dirs, err := filepath.Glob(filepath.Join(pluginDir, "skills", "*"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no skills: %v", err)
	}
	var out []skillDoc
	for _, d := range dirs {
		data, err := os.ReadFile(filepath.Join(d, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		front, body, ok := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		if !ok || !strings.HasPrefix(string(data), "---\n") {
			t.Fatalf("%s: no frontmatter", d)
		}
		s := skillDoc{dir: filepath.Base(d), body: body}
		for _, line := range strings.Split(front, "\n") {
			k, v, _ := strings.Cut(line, ": ")
			switch k {
			case "name":
				s.name = v
			case "description":
				s.description = v
			default:
				t.Errorf("%s: unexpected frontmatter line %q", d, line)
			}
		}
		out = append(out, s)
	}
	return out
}

var skillName = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// X-PLUG-03: skills have valid frontmatter, and the plugin's version is the
// extension's.
func TestPlugin_X_PLUG_03_Skills(t *testing.T) {
	for _, s := range readSkills(t) {
		if s.name != s.dir || len(s.name) > 64 || !skillName.MatchString(s.name) {
			t.Errorf("skill %s: name %q must match its folder, in lower case and hyphens, at most 64 characters", s.dir, s.name)
		}
		if s.description == "" || len(s.description) > 1024 {
			t.Errorf("skill %s: description of %d characters, want 1-1024", s.dir, len(s.description))
		}
		if n := strings.Count(s.body, "\n"); n > 200 {
			t.Errorf("skill %s: %d lines, want at most 200", s.dir, n)
		}
	}
	data, err := os.ReadFile(filepath.Join(pluginDir, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct{ Name, Version string }
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if m := readManifest(t); p.Name != "legendkeeper" || p.Version != m.Version {
		t.Errorf("plugin %+v; its version must equal the extension's %s", p, m.Version)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "bin")); err == nil {
		t.Error("plugin/bin exists; Cowork refuses a plugin with a top-level bin/")
	}
}

// X-PLUG-04: every tool a skill names in backticks exists, so renaming a
// tool fails here until the skills follow.
func TestPlugin_X_PLUG_04_SkillsNameRealTools(t *testing.T) {
	h := newHarness(t)
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]bool{}
	for _, tool := range res.Tools {
		tools[tool.Name] = true
	}
	// A tool-like name: snake_case with a verb we use as a prefix.
	toolRef := regexp.MustCompile("`((?:list|get|read|search|create|update|delete|restore|set|edit|upload|save|replace|add)(?:_[a-z]+)*)")
	seen := 0
	for _, s := range readSkills(t) {
		for _, m := range toolRef.FindAllStringSubmatch(s.body, -1) {
			seen++
			if !tools[m[1]] {
				t.Errorf("skill %s names %s, which lk-mcp doesn't have", s.dir, m[1])
			}
		}
	}
	if seen < 30 {
		t.Errorf("found only %d tool names in the skills; is the pattern still right?", seen)
	}
}
