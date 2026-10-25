package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Prompts are ready-made requests a user picks from the client (in Claude
// Desktop, from the chat box), for jobs that take several tools in a set
// order. Each one is a single user message naming the tools to use. Write
// prompts exist only when the write tools do.

type promptDef struct {
	name, title, description string
	args                     []*mcp.PromptArgument
	write                    bool
	text                     func(map[string]string) string
}

func arg(name, title, description string, required bool) *mcp.PromptArgument {
	return &mcp.PromptArgument{Name: name, Title: title, Description: description, Required: required}
}

// optional returns " <prefix><v>" when v is set, for optional arguments.
func optional(prefix, v string) string {
	if v = strings.TrimSpace(v); v == "" {
		return ""
	}
	return " " + prefix + v
}

var prompts = []promptDef{
	{
		name: "summarize_topic", title: "Summarize a topic",
		description: "Find everything the wiki says about a topic and summarize it, noting what is hidden from players.",
		args:        []*mcp.PromptArgument{arg("topic", "Topic", "a person, place, faction, event or idea", true)},
		text: func(a map[string]string) string {
			return fmt.Sprintf(`Summarize what my LegendKeeper wiki says about %q.

1. Use search (and list_resources or get_resource as needed) to find the relevant resources.
2. Read the most relevant pages with read_page.
3. Give a short summary with the resource names and ids you used. Mark which facts are DM-only (hidden resources or documents, or :::secret blocks) and which players can see.

Only read; don't change anything.`, a["topic"])
		},
	},
	{
		name: "edit_page_safely", title: "Edit a page safely", write: true,
		description: "Make one change to a wiki page: read it, preview the edit with a dry run, then write it with the version read.",
		args: []*mcp.PromptArgument{
			arg("page", "Page", "the page's name (or resource id)", true),
			arg("change", "Change", "what to change, in your own words", true),
		},
		text: func(a map[string]string) string {
			return fmt.Sprintf(`Edit the LegendKeeper page %q: %s

1. Find the page with search, then read it with read_page and keep its version.
2. Change only what I asked. Keep every other line exactly as it was, including :::secret blocks, lk-raw fences and ::unsupported lines.
3. Call edit_page with base_version and dry_run: true, and show me what it would change.
4. If that looks right, call edit_page again without dry_run. If it says the page changed since it was read, read it again and redo the change.
5. If the spoiler guard refuses the write, tell me which term it found and where. Don't reword the text to get past it.`, a["page"], a["change"])
		},
	},
	{
		name: "add_map_location", title: "Add a location to a map", write: true,
		description: "Place a pin for a location on a map, linked to its wiki page when there is one.",
		args: []*mcp.PromptArgument{
			arg("map", "Map", "the resource whose map to use", true),
			arg("place", "Place", "the location's name", true),
			arg("where", "Where", "where it goes: a pixel [x, y] on the map image, or a description", false),
		},
		text: func(a map[string]string) string {
			return fmt.Sprintf(`Add %q to the map of %q in LegendKeeper.%s

1. Find the map's resource with search, then read the map with read_map (note its image size and the existing features).
2. Find a page named %q with search. If one exists, link the pin to it with uri lk://resources/<id>.
3. Work out the position: a pixel [x, y] counted from the image's top-left corner, or pos [lat, lng]. If I didn't say where, ask me.
4. Call set_map_feature with type pin, dry_run: true first, then for real.`, a["place"], a["map"], optional("Where: ", a["where"]), a["place"])
		},
	},
	{
		name: "session_notes", title: "Write session notes", write: true,
		description: "Turn notes from a game session into wiki updates: a session page, plus changes to the pages it touches.",
		args: []*mcp.PromptArgument{
			arg("notes", "Notes", "your session notes, as rough as you like", true),
			arg("session", "Session", "the session's name or number, e.g. Session 12", false),
		},
		text: func(a map[string]string) string {
			return fmt.Sprintf(`Write these game-session notes into my LegendKeeper wiki.%s

Notes:
"""
%s
"""

1. Search for the people, places and things the notes mention, and read their pages.
2. Propose the changes before making any: a new page for the session (under a Sessions resource if there is one), and the additions to existing pages. Put anything the players shouldn't know in :::secret blocks.
3. When I agree, make each change: create_resource with page_markdown for new pages, and read_page then edit_page (with its version) for existing ones. Use dry_run first for edits to existing pages.
4. Finish with a list of the pages you created or changed, with their ids.`, optional("Session: ", a["session"]), a["notes"])
		},
	},
}

func (s *server) addPrompts(ms *mcp.Server) {
	for _, p := range prompts {
		if p.write && s.cfg.ReadOnly {
			continue
		}
		ms.AddPrompt(&mcp.Prompt{Name: p.name, Title: p.title, Description: p.description, Arguments: p.args},
			func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
				in := req.Params.Arguments
				for _, a := range p.args {
					if a.Required && strings.TrimSpace(in[a.Name]) == "" {
						return nil, fmt.Errorf("prompt %s needs %s", p.name, a.Name)
					}
				}
				return &mcp.GetPromptResult{
					Description: p.description,
					Messages:    []*mcp.PromptMessage{{Role: "user", Content: &mcp.TextContent{Text: p.text(in)}}},
				}, nil
			})
	}
}
