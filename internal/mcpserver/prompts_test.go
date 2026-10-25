package mcpserver

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func promptNames(t *testing.T, h *harness) []string {
	t.Helper()
	res, err := h.cs.ListPrompts(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range res.Prompts {
		if p.Title == "" || p.Description == "" {
			t.Errorf("prompt %s lacks a title or description", p.Name)
		}
		names = append(names, p.Name)
	}
	slices.Sort(names)
	return names
}

// X-PROMPT-01: the server offers its prompts; the write ones only when the
// write tools exist.
func TestPrompts_X_PROMPT_01_Listed(t *testing.T) {
	if got := promptNames(t, newHarness(t)); !slices.Equal(got, []string{"add_map_location", "edit_page_safely", "session_notes", "summarize_topic"}) {
		t.Errorf("prompts = %v", got)
	}
	if got := promptNames(t, newHarness(t, readOnlyMode())); !slices.Equal(got, []string{"summarize_topic"}) {
		t.Errorf("read-only prompts = %v", got)
	}
}

func getPrompt(t *testing.T, h *harness, name string, args map[string]string) (*mcp.GetPromptResult, error) {
	t.Helper()
	return h.cs.GetPrompt(t.Context(), &mcp.GetPromptParams{Name: name, Arguments: args})
}

// X-PROMPT-02: a prompt is one user message with the arguments filled in;
// a missing required argument is an error; optional ones may be left out.
func TestPrompts_X_PROMPT_02_Get(t *testing.T) {
	h := newHarness(t)
	r, err := getPrompt(t, h, "edit_page_safely", map[string]string{"page": "Candy Kingdom", "change": "add that the castle has a moat"})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Messages) != 1 || r.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v", r.Messages)
	}
	text := r.Messages[0].Content.(*mcp.TextContent).Text
	for _, want := range []string{`"Candy Kingdom": add that the castle has a moat`, "dry_run: true", "base_version"} {
		if !strings.Contains(text, want) {
			t.Errorf("text lacks %q:\n%s", want, text)
		}
	}
	if _, err := getPrompt(t, h, "edit_page_safely", map[string]string{"page": "Candy Kingdom"}); err == nil || !strings.Contains(err.Error(), "needs change") {
		t.Errorf("missing argument: err = %v", err)
	}
	r, err = getPrompt(t, h, "session_notes", map[string]string{"notes": "Finn met the Lich."})
	if err != nil {
		t.Fatal(err)
	}
	if text := r.Messages[0].Content.(*mcp.TextContent).Text; strings.Contains(text, "Session:") || !strings.Contains(text, "Finn met the Lich.") {
		t.Errorf("text:\n%s", text)
	}
	if _, err := getPrompt(t, newHarness(t, readOnlyMode()), "session_notes", map[string]string{"notes": "x"}); err == nil {
		t.Error("a write prompt works in read-only mode")
	}
}

// X-PROMPT-03: every tool a prompt names exists.
func TestPrompts_X_PROMPT_03_NameRealTools(t *testing.T) {
	h := newHarness(t)
	res, err := h.cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]bool{}
	for _, tool := range res.Tools {
		tools[tool.Name] = true
	}
	toolRef := regexp.MustCompile(`\b((?:list|get|read|search|create|update|delete|restore|set|edit|upload|save|replace|add)_[a-z_]+)\b`)
	for _, p := range prompts {
		args := map[string]string{}
		for _, a := range p.args {
			args[a.Name] = "x"
		}
		text := p.text(args)
		refs := toolRef.FindAllString(text, -1)
		if len(refs) == 0 && !strings.Contains(text, "search") {
			t.Errorf("prompt %s names no tool", p.name)
		}
		for _, ref := range refs {
			if !tools[ref] {
				t.Errorf("prompt %s names %s, which lk-mcp doesn't have", p.name, ref)
			}
		}
	}
}
