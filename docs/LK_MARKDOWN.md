# LK Markdown

LK Markdown is how `lk/markdown` turns a LegendKeeper page (`blocks.List`) into text and back. LegendKeeper's docs never mention Markdown ([DECISIONS.md](DECISIONS.md) D-003, Phase 2 research), so this format is ours. The owner chose its goals in Phase 2 planning:

1. **Lossless.** Blocks → Markdown → blocks gives the same blocks: same kinds, fields, ids and order. Content that Markdown can't express is carried in a form that comes back unchanged.
2. **Readable and editable.** Ordinary prose, headings, lists, quotes, code and simple tables are plain CommonMark + GFM, so a person or a model can edit them without learning anything new.
3. **Fail closed.** Input the converter doesn't fully understand is an error, never a silent guess. This matters most for `secret` blocks: a malformed secret must never turn into player-visible text.

Numbered requirements are `M-<AREA>-NN`. They become contract rows (phase 2b) once this document is approved.

---

## 1. API

```go
package markdown

// ToMarkdown renders a page.
func ToMarkdown(page blocks.List, opts ToOptions) (string, error)

// FromMarkdown parses LK Markdown into a page. When opts.Base is the page
// the Markdown was rendered from, every block the edit left unchanged comes
// back exactly as it was in Base, with its id and every field Markdown can't
// show (see §3).
func FromMarkdown(src string, opts FromOptions) (blocks.List, *Report, error)

type ToOptions struct {
	// IDs writes {#id} on every block that has one (§3.2). Off by default:
	// with Base, ids are restored without appearing in the text.
	IDs bool
}

type FromOptions struct {
	Base blocks.List
	// AllowRemoveOpaque permits an edit that drops an unsupported or raw
	// block from Base (§5, M-SAFE-04).
	AllowRemoveOpaque bool
}

// Report says what FromMarkdown did relative to Base, so a caller (the MCP
// server's dry_run) can show it before writing.
type Report struct {
	Kept, Changed, Added []string     // block ids or new-block paths
	Removed            []RemovedBlock // id, kind, and whether it was secret or opaque
}
```

- **M-API-01:** `FromMarkdown(ToMarkdown(p), {Base: p})` returns a list that encodes to exactly the JSON of `p`, for any `p` the blocks package can decode. That includes unknown kinds, unsupported blocks and unmodelled fields. Without `Base`, the result is the same apart from fields Markdown can't show (§3.3).
- **M-API-02:** `ToMarkdown` is deterministic. The same page always gives the same bytes, so equal renderings mean equal content (§3.1 depends on this).
- **M-API-03:** Neither function makes a request, logs, or panics on any input. Both are fuzzed.

---

## 2. The format

### 2.1 Standard blocks (CommonMark + GFM)

| Block | Markdown | Notes |
|---|---|---|
| paragraph | text | separated by a blank line |
| heading | `#` … `######` | `level` 1–6 |
| quote | `> ` lines | children are any blocks |
| code | fenced ```` ```lang ```` | `language` from the info string; the fence grows past any run of backticks in the text |
| rule | `---` | |
| bulletList / listItem | `- item` | first child is the item's paragraph; nested lists are indented |
| orderedList | `1. item` | `start` from the first number; `1` is omitted on read, as the server does |
| taskList / taskItem | `- [x] text` / `- [ ] text` | a nested taskList is indented under the item before it (§2.6) |
| table | GFM pipe table | only when representable (§2.5); otherwise `:::table` |
| image | a paragraph that is exactly one `![alt](url)` | an image anywhere else is an error (M-SAFE-05) |

Only these goldmark extensions are on: tables, strikethrough and task lists. Linkify, typographer, footnotes, definition lists and raw HTML are off. A bare URL stays text, and quotes stay as typed.

### 2.2 Inline text (runs)

| Run | Markdown |
|---|---|
| bold / italic / strike / code | `**x**`, `*x*`, `~~x~~`, `` `x` `` |
| link | `[x](url)` |
| underline, sub, sup | `[x]{underline}`, `[x]{sub}`, `[x]{sup}` (bracketed span) |
| color, highlight | `[x]{color=#EC4899}`, `[x]{highlight=#F59E0B}`; combinable: `[x]{underline color=#EC4899}` |
| line break | backslash at line end (`\` + newline) |
| mention without a label | `[Princess Bubblegum]{mention=pb1}`. The text is the read-only `name` and is ignored on parse. |
| mention with a label | `[PB]{mention=pb1 label}`; add `doc=d1` for `documentId` |

- **M-INL-01:** ToMarkdown escapes every character that would otherwise start Markdown syntax, `[`, `{` and `\` included, so text round-trips verbatim. Leading and trailing spaces inside a mark are moved outside it, as CommonMark requires.
- **M-INL-02:** Marks render in a fixed order: link outermost, then bold, italic, strike, span, code innermost. That makes renderings canonical after the server merges adjacent runs.
- **M-INL-03:** An unknown span attribute (`[x]{glow}`) is an error. Inline HTML is an error.

### 2.3 Directives for LegendKeeper-only blocks

A container directive is a fence of three or more colons with a name and attributes, then a body, then a closing fence. The closing `:::` closes the innermost open directive. A leaf directive is a single line starting with `::`.

```
:::callout tone=warning
Beware the **Lich**.
:::

:::callout tone=custom icon=dragon color=#aa0000
Here be dragons.
:::

:::expand title="Hidden details"
Body Markdown.
:::

:::layout
:::column width=50 background=#3B82F6
Candy Castle
:::
:::column width=50
Candy Village
:::
:::

:::secret
The Lich is not really gone.
:::

:::textfield title="Origin"
Got his powers from a magic mud puddle.
:::

:::keyvalue title=Stats display=grid
- Species :: Magic dog
- Plays :: **Video games**
:::

:::meter mode=bar title=Vitals
- HP :: 3/5 icon=heart color=#ff0000
:::

:::links title="See also"
- @finn1
- @jake1 doc=d9
:::

:::collection title=Residents isTitleHidden=true
- @finn1
:::

:::collection
```lk-filter
{"type":"filter","id":"f1","filterType":"tag","operator":"has_any","tags":["Kingdom"]}
```
:::

::embed provider=youtube url="https://www.youtube.com/watch?v=x"
::map resourceId=atlas documentId=atlasMap displayHeight=400 viewport="12.5,-3,2"
::map
```

Infobox rows are property directives. Their attributes include the envelope (`title`, `isTitleHidden`, `isHidden`):

```
:::infobox layout=wrapRight width=40
![Portrait](https://assets.legendkeeper.com/p.png){fill title=Portrait isTitleHidden=true}
:::keyvalue title=Stats
- Species :: Magic dog
:::
:::textfield title=Origin
Rolled in a magic mud puddle.
:::
:::
```

- **M-DIR-01:** Attribute names are the spec's JSON field names (`isTitleHidden`, `displayHeight`, `layout`, `width`, …), so the mapping is mechanical and complete. Values are bare tokens or double-quoted strings with `\"` and `\\` escapes. Booleans are `true`/`false`; a bare flag means `true`.
- **M-DIR-02:** Every optional field of a kind can be written as an attribute. A field not written is unset, never a default.
- **M-DIR-03:** Rows (`keyvalue`) are `- key :: value`, where the value is inline LK Markdown. Meter items are `- name :: current/max` plus attributes. Link and collection items are `- @resourceId` plus an optional `doc=`.
- **M-DIR-04:** Block attributes for kinds that use standard syntax (`align`, `background`, quote `color`, image fields) go in an attribute set right after the block: `{align=center}` at the end of a heading line, or a `{…}` line directly under a paragraph or quote.

### 2.4 Opaque content

| Content | Markdown |
|---|---|
| `unsupported` block | `::unsupported id=u1` |
| unknown kind (`RawBlock`) | ```` ```lk-raw ```` fence holding the block's JSON |
| known kind with unmodelled fields, or anything §2.1–2.3 can't express exactly | ```` ```lk-raw ```` fence holding that block's JSON |
| unsupported or raw infobox row | `::unsupported id=…` or `lk-raw`, inside the infobox |

- **M-OPQ-01:** `lk-raw` is a safety valve. Any block whose Markdown form wouldn't parse back to the identical block is rendered as `lk-raw` instead. A test renders every block shape the package can decode and asserts this.
- **M-OPQ-02:** An `lk-raw` fence must hold one JSON object with a string `kind`. Anything else is an error.

### 2.5 Tables

A table renders as a GFM pipe table only when:
- the first row is all header cells and the other rows have none;
- there are no `colspan`/`rowspan` values;
- every cell holds exactly one paragraph;
- every row has the same number of cells.

Otherwise it renders as directives:

```
:::table
:::row
:::cell header colspan=2
Name
:::
:::
:::
```

### 2.6 Task lists

A taskList holds taskItems and nested taskLists. A nested taskList directly after an item is written indented under that item. One at the start of a taskList, with no item before it, renders as `lk-raw` (M-OPQ-01).

---

## 3. Ids and fields Markdown doesn't show

### 3.1 Base matching (the default)

Ids, ranks, row ids, unmodelled fields and legacy values aren't written in the Markdown. `FromMarkdown` gets them back from `Base`:

1. Parse the Markdown into blocks.
2. Pair every parsed block that has an explicit id (§3.2) with the base block of that id, wherever it is in `Base` (a move). The rest of each sibling list is matched against the base siblings left over with a longest-common-subsequence diff over their canonical renderings (M-API-02).
3. A parsed block whose rendering equals its matched base block **is replaced by the base block, verbatim**, keeping its id and every field. A parsed block left unmatched whose rendering equals a still-unmatched base sibling anywhere in the same list (a reorder) is matched to it the same way.
4. Inside a diff hunk, the *i*-th remaining parsed block of a kind and the *i*-th remaining base block of that kind are **changed**:
   - The parsed block keeps the base block's id.
   - It also keeps the base fields Markdown can't show: the id, a row's rank, and the ids and ranks of key-value rows (matched by key), resource items (by resource and document id) and meter items (by name).
   - Its children and infobox rows are matched recursively by the same rules.
   - A changed block that comes out byte-identical to its base block counts as kept.
5. Any other parsed block is **added**, with no id; the server assigns one.
6. Any unmatched base block is **removed** (see M-SAFE-04).

Details:

- Blocks and infobox rows are never matched with each other, even when their kinds are the same.
- A base block that renders opaque (`::unsupported`, `lk-raw`) is never paired as changed: it is either matched verbatim or by its id, or it is removed. An `unsupported` block matched by id always comes back as the base block; for an infobox row only its envelope (title, visibility) comes from the Markdown. A parsed `lk-raw` block can't take an id, so it is never paired as changed either.
- A verbatim match also needs the explicit ids inside the parsed block to sit on the same nodes in the base block; otherwise the pair is changed and matched recursively.
- The diff matches the common prefix and suffix first, then runs Myers' algorithm with a bounded edit distance (about 2²⁴ steps and 4 MiB per list). Past the bound the rest of the list is one hunk, and reorder matching (step 3) still keeps every unchanged block.
- `Report` lists, in document order: `Kept`, the ids of verbatim base blocks (the top of each verbatim subtree); `Changed`, the ids of base blocks edited in place, including containers whose descendants changed; `Added`, the paths (as `blocks.Walk` writes them, in the result) of the tops of new subtrees; and `Removed`, the tops of removed base subtrees.

- **M-ID-01:** An unchanged block comes back byte-identical to the base block.
- **M-ID-02:** Editing one paragraph changes only that paragraph. Its siblings and its container keep their ids.
- **M-ID-03:** Matching never moves an id onto a block of another kind. It never uses the same base block twice.

### 3.2 Explicit ids (optional)

`ToOptions.IDs` writes `{#id}`:
- after a heading's text;
- on a `{…}` line under a paragraph;
- in a directive's attributes;
- after a row or list item.

An explicit id in the Markdown wins over base matching. The same id twice is an error (M-SAFE-06), and so is an id whose base block is of another kind, or a block where the base has an infobox row (`*IDError`). An id not in `Base` is kept as written, and the server decides; for `::unsupported` it is an error (M-SAFE-06). Explicit ids are for tools that need stable handles across separate reads; normal editing doesn't need them.

### 3.3 Without Base

Every parsed block is new. The server assigns ids, so a full-page replace of an existing page without `Base` changes every id. The `lk.Documents` helper in §6 always passes the page it read.

---

## 4. Plain-text parity

- **M-TXT-01:** The text a reader sees in the Markdown for a block is the text `blocks.PlainText` gives for it. Markup and attributes don't count. A test checks the two derivations agree on the golden fixture.

---

## 5. Fail-closed rules

`FromMarkdown` returns an error, with a line number and the reason, and returns no blocks, for each of these:

- **M-SAFE-01:** An unknown directive name, an unknown attribute, a duplicate attribute, or a value the field can't hold (wrong type, enum value outside the spec). Enum values the SDK doesn't know are accepted only inside `lk-raw`.
- **M-SAFE-02:** An unclosed container directive, or a stray closing fence. In particular, a `:::secret` that is never closed doesn't let its text run into the rest of the page.
- **M-SAFE-03:** Raw HTML (block or inline), a link reference definition, or a footnote.
- **M-SAFE-04:** An edit that removes an `unsupported` or `lk-raw` block found in `Base`, at any depth (inside a removed or changed container, or as an infobox row), unless `AllowRemoveOpaque` is set. The error is a `*RemoveError` listing every such block; it matches `ErrRemovesOpaque` with `errors.Is`. Removing that content deletes something the editor couldn't see. Removed `secret` blocks are allowed, but `Report.Removed` lists them so the caller can confirm.
- **M-SAFE-05:** An image that isn't alone in its paragraph, or an image inside a heading or table cell. The block model has no inline images.
- **M-SAFE-06:** An `::unsupported` id that isn't in `Base` (the server would answer `UNSUPPORTED_KIND`), or the same block id twice. Also an `::unsupported` with no id. These come from base matching, so the error is an `*IDError` with the block's path rather than a line number.
- **M-SAFE-07:** Input over 1 MiB or nested deeper than 64 levels (the server's own limit), rejected before parsing.

The converter never checks what the server checks: colours, widths, callout icon and tone pairing, image URL ownership. The server's 400 is the authority, as with the builders (C-BUILD-06).

---

## 6. Read-edit-write helpers

The MCP server needs read → edit → write with the safe defaults built in. The helpers live in `lk/markdown`, which imports `lk`; `lk` itself never imports goldmark, so SDK callers who don't use Markdown don't pull it in.

```go
// Get reads a page as LK Markdown, with the version and blocks it was
// rendered from (the Base for Replace).
func Get(ctx context.Context, c *lk.Client, doc lk.DocRef, opts ...lk.CallOption) (*Page, error)

type Page struct {
	Markdown string
	Version  string
	Blocks   blocks.List
}

// Replace parses md against base.Blocks, then replaces the page with
// IfMatch(base.Version). DryRun returns the parsed blocks and Report
// without writing.
func Replace(ctx context.Context, c *lk.Client, doc lk.DocRef, base *Page, md string, ro ReplaceOptions, opts ...lk.CallOption) (*lk.DocumentBlocks, *Report, error)
```

- **M-API-04:** `Replace` always sends `IfMatch(base.Version)`. An edit to a page that changed meanwhile fails with `ErrVersionMismatch` instead of overwriting someone's work.
- **M-API-05:** `ReplaceOptions.DryRun` sends no request.

---

## 7. Dependency

- `github.com/yuin/goldmark` v1.8.6: CommonMark-compliant, pure Go, with no dependencies of its own (checked with `go mod graph`), and builds with `CGO_ENABLED=0`. The `:::` directives, `::` leaf lines, bracketed spans and `{…}` attribute sets are custom goldmark block and inline parsers inside `lk/markdown`. It's recorded in DECISIONS.md D-006 when added.

---

## 8. Open questions for review

| # | Question | Proposal |
|---|---|---|
| Q1 | How ids survive editing | Base matching (§3.1) by default, with explicit `{#id}` optional. Writing every id inline makes lists and tables unreadable (every list item, cell and row has ids). |
| Q2 | Tables with ids or spans | GFM when representable (§2.5) using Base for ids; otherwise `:::table`. |
| Q3 | Mention syntax | `[Name]{mention=id}` / `[Label]{mention=id label}` (§2.2). The guide's `lk://` URLs are link marks, not mentions, so they're kept as plain links. |
| Q4 | Removing opaque content | Error unless `AllowRemoveOpaque` (M-SAFE-04). Secrets may be removed but are reported. |
| Q5 | `lk-raw` fences | Exposing JSON in Markdown is a little ugly but strictly lossless, and in practice it appears only for unknown kinds, unmodelled fields and unusual shapes. The Phase 3 spoiler guard must treat an `lk-raw` fence holding a `secret` as hidden content, not prose. |

## 9. Implementation plan (after approval)

1. Add the contract rows (M-* → phase `2b`) and record the dependency in DECISIONS.md D-006, then commit.
2. Write `lk/markdown`:
   - `render.go`: blocks → Markdown, canonical form, the `lk-raw` safety valve.
   - `parse.go`: goldmark with the extensions above, plus the custom parsers for directives, spans and attribute sets.
   - `merge.go`: base matching (§3.1) and the `Report`.
   - `errors.go`: `*SyntaxError{Line, Reason}`.
3. Tests:
   - golden files for each construct;
   - a round trip of every block kind and of the sandbox golden page;
   - the base-matching edit cases (M-ID-01..03);
   - every fail-closed rule;
   - `FuzzFromMarkdown`, which must never panic, and whose output must render and re-parse to the same blocks;
   - `FuzzRoundTrip`, which builds blocks from fuzz input and checks Markdown round-trips them.
4. Sandbox integration:
   - read a written page as Markdown, edit one paragraph, write it back;
   - check that the ids of untouched blocks are unchanged and an `unsupported` echo is preserved.
5. `make lint test test-race test-integration build verify-static`, then commit and push.
