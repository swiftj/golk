# golk: LegendKeeper for Go and Claude

**golk** connects [LegendKeeper](https://www.legendkeeper.com), the worldbuilding wiki, to Go programs and to Claude. It has three parts:

| Part | What it is | For |
|---|---|---|
| [`lk`](#the-lk-go-sdk) | A Go SDK for the LegendKeeper v3 REST API: retries, idempotency, versioned writes, pagination, rate limiting, typed blocks, and a lossless Markdown converter | Go developers |
| [`lk-mcp`](#lk-mcp-the-mcp-server) | An MCP server with 60 task-shaped tools and 4 prompts, covering 100% of the LegendKeeper API, built on `lk` | Claude Desktop (chat and Cowork), Claude Code and any MCP client |
| [Claude Desktop extension and plugin](#install-in-claude-desktop) | A one-click Claude Desktop extension (`.mcpb`), plus a plugin with two skills | People who want Claude to read and write their wiki |

It was built for a DM who publishes a D&D world wiki that **players read**. So it puts correctness, data safety, and keeping DM-only content away from players ahead of speed:

- Versioned writes never overwrite an edit made in the app since Claude read the page.
- Every write tool has a dry run.
- An optional **spoiler guard** refuses any write that would show a protected term to players.

> This project is not affiliated with or endorsed by LegendKeeper.

---

## Contents

- [Quick start (Claude Desktop)](#quick-start-claude-desktop)
- [Install in Claude Desktop](#install-in-claude-desktop)
- [What Claude can do](#what-claude-can-do)
- [lk-mcp, the MCP server](#lk-mcp-the-mcp-server)
- [The spoiler guard](#the-spoiler-guard)
- [LK Markdown](#lk-markdown)
- [The lk Go SDK](#the-lk-go-sdk)
- [Building from source](#building-from-source)
- [Testing](#testing)
- [Repository layout](#repository-layout)
- [Design documents](#design-documents)
- [Supported platforms](#supported-platforms)
- [License](#license)

---

## Quick start (Claude Desktop)

You need an Apple Silicon Mac with Claude Desktop 3.6556.2 or later, a LegendKeeper project and an API key for it. Use a test project first.

**Download** `lk-mcp-darwin-arm64.mcpb` and `legendkeeper-plugin.zip` from the [latest release](https://github.com/swiftj/golk/releases/latest), or build them yourself:

```sh
git clone https://github.com/swiftj/golk.git
cd golk
make mcpb plugin        # needs Go 1.27+ and Node (npx) for the packer
```

1. Double-click `lk-mcp-darwin-arm64.mcpb` and enter your API key.
2. In a new chat, ask: **"List my LegendKeeper projects."**
3. Recommended: in Claude Desktop, open **Customize** and choose **Add › Upload plugin**, then pick `legendkeeper-plugin.zip`. This adds the two skills to every chat and every Cowork task. Cowork is part of Claude Desktop, so it uses the same extension, with nothing else to set up.

---

## Install in Claude Desktop

Cowork is part of Claude Desktop, so everything below covers both chat and Cowork. The integration has two pieces, installed separately. (Why not one file? A plugin can carry an `.mcpb`, but Claude Desktop ignores a local server inside a plugin in chat. Cowork ignores it too when a setting has no default, as the API key doesn't. The extension is what gets prompted for settings. See [D-019](docs/DECISIONS.md).)

| Piece | File | Contains |
|---|---|---|
| Claude Desktop extension | `dist/lk-mcp-darwin-arm64.mcpb` | The `lk-mcp` server binary and its settings. This is what gives Claude the LegendKeeper tools. |
| Plugin (skills) | `dist/legendkeeper-plugin.zip` | Two skills that teach Claude to use the tools well: `legendkeeper-wiki` (pages, LK Markdown, hidden content, the guard) and `legendkeeper-worldbuilding` (maps, timelines, calendars, templates, boards). It holds no server and no key. |

### 1. Get them

Download both from the [releases page](https://github.com/swiftj/golk/releases), and check them with `shasum -a 256 -c SHA256SUMS`. Or build them:


```sh
make mcpb     # dist/lk-mcp-darwin-arm64.mcpb  (runs `npx -y @anthropic-ai/mcpb@2.1.2` to validate and pack)
make plugin   # dist/legendkeeper-plugin.zip
```

### 2. Install the Desktop extension

1. Open it in one of three ways:
   - double-click `dist/lk-mcp-darwin-arm64.mcpb`
   - drag it onto the Claude Desktop window
   - in Claude Desktop, open **Settings › Extensions › Advanced settings › Install Extension**
2. Fill in the settings:

   | Setting | Required | Meaning |
   |---|---|---|
   | **LegendKeeper API key** | yes | The key of the one project Claude should work in. Claude Desktop stores it masked, and lk-mcp never writes it to a file or a log. |
   | **Spoiler terms file** | no | A text file of protected terms, one per line. **Leave it empty for full DM access**, so every write is allowed. Set it to turn on the [spoiler guard](#the-spoiler-guard). |
   | **Read-only** | no | Offer Claude only the read tools. |

3. Check that the extension is switched on in **Settings › Extensions**.
4. Start a new chat and ask **"List my LegendKeeper projects."** The answer names your project and ends with a line such as `read-only: false; spoiler guard: off (no terms file)`.

To change a setting later, open **Settings › Extensions › LegendKeeper**. To upgrade, install a newer `.mcpb` over the old one; your settings are kept.

### 3. Install the plugin (skills)

In Claude Desktop, open **Customize** and choose **Add › Upload plugin**, then pick `dist/legendkeeper-plugin.zip`. Plugin skills apply to ordinary chats as well as Cowork tasks. You can see them under **Customize › Skills**.

### Using it in Cowork

Cowork runs inside Claude Desktop and uses the extensions installed there, so the LegendKeeper tools and your settings carry over. Start a Cowork task and ask, for example, *"List my LegendKeeper projects, then add a paragraph to the Tree Fort page."* If a task doesn't show the LegendKeeper tools, check that the extension is switched on in **Settings › Extensions**, then start a new task.

### Other MCP clients (Claude Code, any stdio client)

Run the binary directly and pass settings as environment variables (see [Configuration](#configuration)). For example, in a client's JSON config:

```json
{
  "mcpServers": {
    "legendkeeper": {
      "command": "/path/to/golk/dist/lk-mcp-darwin-arm64",
      "env": { "LEGENDKEEPER_API_KEY": "<your key>" }
    }
  }
}
```

Build the plain binaries with `make build`. On Linux, use `dist/lk-mcp-linux-amd64` or `dist/lk-mcp-linux-arm64`.

---

## What Claude can do

Once the extension is installed, you can ask for things like:

- *"Find everything about the Candy Kingdom and summarise it."*
- *"Create a page for Princess Bubblegum under Characters, with an infobox for species and title."*
- *"Add a secret section to the Ice King's page saying he's really Simon Petrikov."*
- *"Hide the Lich's page from players."*, or *"Make the Mushroom War page visible to players."*
- *"Put a pin on the Ooo map at pixel 512, 300 named Tree Fort, linked to the Tree Fort page."*
- *"Add a timeline event 'The Mushroom War' in year −1000 on the History lane."*
- *"Save the Kingdom page as a template, then create Fire Kingdom from it."*
- *"On the Rivalries board, add the Fire Kingdom and an arrow from it to the Candy Kingdom."*
- *"Sort the children of Atlas alphabetically."*
- *"Tick off 'Find the Enchiridion' on the Quests page."*
- *"Add a Vitals meter to Finn's sidebar with HP 3 of 5."*
- *"Who are the members of this wiki?"*

How it stays safe:

- **Reads first.** Claude reads a page, gets its `version`, and edits with that version. If you changed the page in LegendKeeper since, the write is refused and Claude re-reads it.
- **Dry runs.** Every write tool takes `dry_run`, which describes the change without making it. Deletes, replaces and unhides are worth previewing.
- **Nothing silently dropped.** Content that Markdown can't express (`lk-raw` and `::unsupported` blocks) is kept. Deleting it needs explicit permission (`allow_remove_opaque`).

---

## lk-mcp, the MCP server

`lk-mcp` is a single static binary that speaks MCP over stdio. stdout carries only protocol messages, and logs go to stderr. Logs never contain API keys, and page content is never logged at INFO.

### Tools

Every tool carries MCP annotations (read-only, destructive, idempotent), and every write tool takes `dry_run`. Every tool takes an optional `project` when several projects are configured. Output is concise text with ids, names and versions, plus structured data. Errors include the LegendKeeper error code, the request ID and a hint.

| Area | Read tools | Write tools |
|---|---|---|
| Projects and search | `list_projects`, `search`, `list_tags` | |
| Resources (wiki entries) | `list_resources` (in sidebar order, with ranks), `get_resource` | `create_resource` (with a page, or 1–5 documents of any type), `create_resources` (up to 100 at once), `update_resource` (rename, tags, aliases, icon, lock, banner, permissions, move, **reorder** with `after`/`before`, hide or show), `delete_resource`, `restore_resource` |
| Documents (tabs) | (in `get_resource`) | `create_document` (page, map, time, board, blank), `update_document` (rename, hide or show, tab order, full width, timeline calendar and chained sources), `delete_document`, `restore_document` |
| Pages | `read_page` (as LK Markdown, with its version) | `edit_page` (the whole page) |
| Blocks | `read_block` | `insert_block`, `replace_block`, `update_block`, `move_block`, `delete_block`, `assign_block_ids` |
| Properties and infoboxes | (in `get_resource`, `read_block`) | `set_property` (any kind, sidebar or infobox), `delete_property`, `set_row`, `delete_row` |
| Assets | `list_assets`, `get_asset` | `upload_image` (from a local file), `create_asset_folder`, `update_asset`, `delete_asset` |
| Maps | `read_map` | `set_map_feature` (pins, labels, regions, paths, folders; place by lat/lng or by image pixel), `delete_map_feature`, `set_map_image` |
| Timelines | `read_timeline` | `set_timeline_lane`, `delete_timeline_lane`, `set_timeline_event`, `delete_timeline_event` |
| Calendars | `list_calendars`, `get_calendar` | `create_calendar`, `replace_calendar`, `delete_calendar` |
| Templates | `list_templates` | `save_as_template`, `create_from_template` |
| Boards (tldraw canvases) | `read_board` | `add_board_shapes`, `update_board_shapes`, `delete_board_shapes`, `replace_board` |
| Members and invites | `list_members`, `list_invites` | `invite_member` (emails someone), `remove_member`, `revoke_invite` |

That's 17 read tools and 43 write tools. In read-only mode the write tools aren't registered at all.

**100% API coverage.** Every one of the 89 operations of the LegendKeeper v3 API, and every request field, is reachable through these tools. [`docs/API_COVERAGE.md`](docs/API_COVERAGE.md) maps each operation and field to the tool parameter that carries it. A test checks that table against the API spec and the live tool schemas, so a gap fails the build.

### Prompts

lk-mcp also offers four MCP prompts. These are ready-made requests that walk Claude through a multi-step job in the right order:

| Prompt | Arguments | Does |
|---|---|---|
| `summarize_topic` | `topic` | Finds and summarizes what the wiki says about a topic, marking what's DM-only. It only reads. |
| `edit_page_safely` | `page`, `change` | Reads the page, makes only that change, previews it with a dry run, then writes with the version it read |
| `add_map_location` | `map`, `place`, `where` (optional) | Places a pin on a map, linked to the place's page if one exists |
| `session_notes` | `notes`, `session` (optional) | Turns rough session notes into a session page plus updates to the pages they touch, proposing the changes before making any |

The three write prompts exist only when the write tools do. Clients that support MCP prompts, such as Claude Code, list them for you. Whether Claude Desktop shows MCP prompts isn't documented, so the skills cover the same workflows.

The extension declares a minimum Claude Desktop version of 3.6556.2 (`compatibility.claude_desktop` in the manifest). That's the version it was tested with, because no documentation names an earlier one that supports manifest 0.3.

### Configuration

All settings come from the environment. The Desktop extension sets them for you.

| Variable | Meaning |
|---|---|
| `LEGENDKEEPER_API_KEY` | The key of the `default` project |
| `LEGENDKEEPER_API_KEY_<NAME>` | Adds a project named `<name>` (lower-case letters, digits, `_`). Tools take `project: "<name>"`. |
| `LEGENDKEEPER_BASE_URL` | The API host (default `https://www.legendkeeper.com`) |
| `LK_MCP_READ_ONLY` | `true` registers only the read tools |
| `LK_MCP_GUARD_FILE` | The spoiler-guard terms file. Unset means no guard: every write is allowed. |
| `LK_MCP_GUARD` | `off` (or `false`) disables the guard even with a terms file; `on` (or `true`) is the default |
| `LK_MCP_LOG_LEVEL` | `debug`, `info` (default), `warn` or `error` |

Empty values count as unset, and a malformed value stops lk-mcp at startup with an error. `lk-mcp --version` prints the version.

### What "hidden" means

LegendKeeper content is **DM-only** when it is any of these:
- a hidden resource (all of it)
- a hidden document (tab)
- a hidden sidebar property or infobox row
- inside a `:::secret` block

"Hidden" is each item's own flag: a resource does not inherit its parent's flag, and a document does not inherit its resource's.

---

## The spoiler guard

The guard is **optional**. Without a terms file, Claude has full DM access, and `list_projects` reports `spoiler guard: off (no terms file)`.

With a terms file (`LK_MCP_GUARD_FILE`, or the extension's **Spoiler terms file** setting), every write is checked before it is sent. A write that would put a protected term anywhere players can see it is refused, and the error lists each term and where it was found.

**The terms file**
- Plain text with one term per line. A multi-word term matches across line breaks and extra spaces.
- Keep it **outside the wiki and out of any repository**.
- Matching ignores case and invisible formatting characters, and matches whole words only.
- The file is reloaded when it changes. An empty file, or a file that changes while it is being read, refuses writes (fails closed) rather than switching protection off.

**What is checked:** everything the write would show players. That includes page blocks, names, tags, aliases, image URLs, sidebar properties, map features, timeline lanes and events, calendar specs, board shapes and template copies.
- Unhiding or restoring content checks everything it would reveal.
- Content that is DM-only (see [What "hidden" means](#what-hidden-means)) may hold any term.
- Hidden map pins and hidden timeline events are still checked as if public. The API can't show what players see, so this stays the safe default until the owner confirms otherwise (`hiddenFeaturesConfirmed` in the code).

The guard can't see edits you make in the LegendKeeper app, or text the server derives itself (such as formatted dates). See [`docs/DECISIONS.md`](docs/DECISIONS.md) D-011, D-014 and D-017 for its exact rules and known limits.

---

## LK Markdown

`read_page` and `edit_page` exchange pages as **LK Markdown**: CommonMark plus GFM (tables, task lists, strikethrough), with directives for LegendKeeper's own blocks. It is **lossless**. A page read and written back unchanged is byte-for-byte the same, and anything Markdown can't express is kept as an `lk-raw` JSON fence.

```markdown
# The Ice King

:::callout tone=warning
Beware the **crown**.
:::

:::secret
He is really Simon Petrikov.
:::

:::keyvalue title=Stats
- Species :: Human (mostly)
- Title :: King of the Ice Kingdom
:::

See also [Gunter]{mention=gunter1}.
```

Edits are matched to the base page by content, so block ids, unmodelled fields and comments survive an edit. The converter fails closed:
- raw HTML is refused
- a stray `::` line is refused
- a code fence that could swallow a `:::secret` is refused
- deleting opaque content needs explicit permission

The full spec is in [`docs/LK_MARKDOWN.md`](docs/LK_MARKDOWN.md).

---

## The lk Go SDK

```sh
go get github.com/swiftj/golk/lk
```

`lk` matches the official TypeScript SDK (`@legendkeeper/sdk`) in *behaviour*, but is idiomatic Go in *shape*.

```go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/markdown"
)

func main() {
	ctx := context.Background()
	c, err := lk.NewFromEnv() // LEGENDKEEPER_API_KEY, LEGENDKEEPER_BASE_URL
	if err != nil {
		log.Fatal(err)
	}

	p, err := c.Project(ctx) // resolved once and cached
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("connected to", p.Name)

	// Every list is an iterator; pagination is handled for you.
	for r, err := range c.Resources.All(ctx, lk.ResourceListParams{}) {
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(r.Name, r.ID)
	}

	// Create a resource, then edit its first page as Markdown.
	r, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: "The Tree Fort"})
	if err != nil {
		log.Fatal(err)
	}
	doc := lk.DocRef{ResourceID: r.ID, DocumentID: r.Documents[0].ID}
	page, err := markdown.Get(ctx, c, doc)
	if err != nil {
		log.Fatal(err)
	}
	_, _, err = markdown.Replace(ctx, c, doc, page, "Home of **Finn** and **Jake**.\n", markdown.ReplaceOptions{})
	if errors.Is(err, lk.ErrVersionMismatch) {
		log.Fatal("someone edited the page since it was read; read it again")
	} else if err != nil {
		log.Fatal(err)
	}
}
```

**Client and options**
- `lk.New(apiKey, opts...)`. `New` never reads the environment; `lk.NewFromEnv(opts...)` does.
- `WithBaseURL`, `WithHTTPClient`, `WithProjectID`, `WithTimeout` (default 30s per attempt), `WithMaxRetries` (default 2), `WithRateLimits` / `WithRateLimiter`, `WithoutIdempotencyKeys`, `WithLogger` (`*slog.Logger`), `WithUserAgent`.
- Per call: `lk.IfMatch(version)`, `lk.WithIdempotencyKey(key)`, `lk.ForProject(id)`, `lk.CaptureResponse(&meta)`.

**Behaviour** (every rule is numbered in [`docs/SDK_CONTRACT.md`](docs/SDK_CONTRACT.md) and named by a test):
- **Project:** resolved once from the credential and cached. A key that reaches several projects is an error that asks for an explicit ID.
- **Retries:** GET, PUT, PATCH and DELETE retry on 429, 502, 503, 504, timeouts and connection failures, with exponential backoff and jitter.
  - `Retry-After` is honoured. Above 8s on a 429 or 503, the error is returned with `RetryAfter` set instead of retrying.
  - Every POST carries an `Idempotency-Key`, reused on each retry, so POSTs retry safely too, including on 409 `IN_PROGRESS`.
  - Never retried: context cancellation, 500, or any other 4xx.
- **Versioned writes:** `IfMatch` sends `If-Match`, and a mismatch is `lk.ErrVersionMismatch`, with the server's current version in `APIError.CurrentVersion`. When a body lacks a version, it is taken from the `ETag`.
- **Errors:** `*lk.APIError` carries `Status`, `Code`, `Message`, `Errors`, `RequestID`, `RetryAfter` and `CurrentVersion`. Branch on the code with `errors.Is(err, lk.ErrNotFound)` and similar, never on the message. `*lk.ConnectionError` wraps transport failures.
- **Three-state fields:** `lk.Optional[T]` (omitted or set) and `lk.Nullable[T]` (omitted, `null` or set). Fields you don't set are never sent, and tests assert the exact JSON.
- **Rate limits:** requests are paced client-side, below 120 writes/min and 300 requests/min per key. Use `Resources.CreateMany` for bulk creates.
- **Blocks:** `lk/blocks` holds a sealed `Block` interface with builders (`blocks.P`, `blocks.H`, `blocks.Callout`, `blocks.KeyValue`…) and readers (`Walk`, `FindBlock`, `PlainText`).
  - A kind the SDK doesn't know decodes into a raw block that keeps its JSON.
  - `unsupported` blocks keep their ids.
  - Unmodelled fields round-trip.

**Namespaces:** `Projects`, `Resources` (with templates), `Documents`, `Blocks`, `Properties`, `Search`, `Tags`, `Assets` (with image upload), `Features` (map features), `Timelines` (lanes and events), `Calendars`, `Templates`, `Boards`, `Members` and `Invites`: every operation of the API. There are map helpers too: `lk.PixelToPos` and `lk.PosToPixel`. `lk/markdown` has `ToMarkdown`, `FromMarkdown`, `Get` and `Replace` (with `DryRun`).

Runnable ports of the examples in LegendKeeper's developer guide are in [`examples/`](examples/). They are integration tests against a sandbox project.

---

## Building from source

Requirements:
- Go 1.27 or later
- `make`
- Node.js, for `npx`, only to pack the Desktop extension
- `golangci-lint`, `staticcheck` and `govulncheck` for `make lint`; `make tools` installs pinned versions

| Command | Does |
|---|---|
| `make build` | Cross-compiles `dist/lk-mcp-darwin-arm64`, `dist/lk-mcp-linux-amd64` and `dist/lk-mcp-linux-arm64` |
| `make verify-static` | Checks every binary (and the one inside the `.mcpb`) was built with `CGO_ENABLED=0`, and that Linux binaries are statically linked |
| `make mcpb` | Packs the Claude Desktop extension `dist/lk-mcp-darwin-arm64.mcpb` |
| `make plugin` | Zips the skills plugin `dist/legendkeeper-plugin.zip` |
| `make lint` | `gofmt`, `go vet`, `staticcheck`, `golangci-lint`, `govulncheck` |
| `make test` / `make test-race` | Unit tests, without and with the race detector |
| `make test-integration` | Integration tests against a sandbox project (see below) |
| `make generate` | Regenerates `lk/internal/gen` from `docs/legendkeeper/openapi.json` |

Every release binary is static and CGO-free: no cgo dependency is allowed. The release version of the extension and plugin is in `plugin/mcpb/manifest.json`, and `plugin/.claude-plugin/plugin.json` must match it.

---

## Testing

- **Unit tests** run against `httptest` fakes and assert each request exactly: method, path, query, `Authorization`, `If-Match`, `Idempotency-Key` and the body JSON. Retry tests use an injected clock and random source, so they never sleep.
- **Fuzz tests** cover block JSON decoding and the Markdown converter.
- **Contract coverage:** a test fails the build if any numbered requirement in `docs/SDK_CONTRACT.md` has no test naming it.
- **Integration tests** (`-tags integration`) run against a **sandbox** LegendKeeper project:

  ```sh
  export LK_SANDBOX_API_KEY=...            # a key for a throwaway project
  export LK_SANDBOX_PROJECT_NAME="..."     # that project's exact name
  make test-integration
  ```

  They read the key's project first and **abort unless its name equals `LK_SANDBOX_PROJECT_NAME`**, so they can never touch a real wiki. Every test deletes what it creates, all tests share one rate limiter, and a final sweep fails the run if anything was left behind. They never read `LEGENDKEEPER_API_KEY`.

---

## Repository layout

```
cmd/lk-mcp/            MCP server entrypoint
lk/                    public SDK: client, options, errors, retry, pagination, namespaces
lk/internal/gen/       generated from openapi.json (never hand-edited; `make generate`)
lk/blocks/             block types, builders, readers
lk/markdown/           LK Markdown <-> blocks, Get/Replace
internal/mcpserver/    MCP tools, config, output, errors
internal/mcpserver/guard/  the spoiler guard
internal/sandbox/      guarded sandbox client for integration tests
internal/cmd/lkgen/    the code generator
examples/              ports of the guide's examples (integration-tagged)
plugin/                Desktop extension manifest, skills plugin
docs/                  sources of truth, contract, decisions, LK Markdown spec
scripts/               build verification
```

---

## Design documents

| Document | What it holds |
|---|---|
| [`docs/SDK_CONTRACT.md`](docs/SDK_CONTRACT.md) | Every numbered behavioural requirement (`C-*` SDK, `M-*` Markdown, `X-*` MCP), each tested |
| [`docs/DECISIONS.md`](docs/DECISIONS.md) | Every non-obvious choice, sandbox probe result and conflict between the LegendKeeper spec and guide (D-001 to D-021) |
| [`docs/API_COVERAGE.md`](docs/API_COVERAGE.md) | Every API operation and request field, and the tool parameter that carries it |
| [`docs/LK_MARKDOWN.md`](docs/LK_MARKDOWN.md) | The LK Markdown format and its fail-closed rules |
| [`docs/legendkeeper/`](docs/legendkeeper/) | LegendKeeper's public API docs, the build's sources of truth (see [NOTICE](docs/legendkeeper/NOTICE.md)) |
| [`plugin/README.md`](plugin/README.md) | Short install guide for the extension and plugin |
| [`CLAUDE.md`](CLAUDE.md) | The working rules this project was built under with Claude Code |

---

## Supported platforms

| Platform | Binary | Claude Desktop extension |
|---|---|---|
| macOS, Apple Silicon | `lk-mcp-darwin-arm64` | `lk-mcp-darwin-arm64.mcpb` |
| Linux x86-64 (e.g. servers, AWS Lightsail) | `lk-mcp-linux-amd64` | — (no Claude Desktop for Linux) |
| Linux ARM64 | `lk-mcp-linux-arm64` | — |

Windows and Intel Macs are deliberately not supported.

---

## License

The code is under the [MIT License](LICENSE). The files in `docs/legendkeeper/` are copies of LegendKeeper's public developer documentation. They are not covered by this license and remain LegendKeeper's; see [NOTICE](docs/legendkeeper/NOTICE.md).
