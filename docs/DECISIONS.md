# Decisions

Non-obvious choices, spec/guide conflicts and their resolutions. Numbered `D-NNN`;
never renumbered. Contract ambiguities are proposed in
[`SDK_CONTRACT.md`](SDK_CONTRACT.md#ambiguities-and-conflicts) and copied here once
approved.

## D-001 — Module path is `github.com/swiftj/golk`

- **Date:** 2026-10-23
- **Context:** CLAUDE.md had the placeholder `github.com/OWNER/legendkeeper-go`.
- **Decision:** the owner chose the GitHub project `github.com/swiftj/golk`. CLAUDE.md is updated to match.

## D-002 — Go 1.27, golangci-lint v2 plus standalone staticcheck

- **Date:** 2026-10-23
- **Decision:** `go 1.27` in go.mod (current stable is 1.27.3). `make lint` runs `gofmt -l`, `go vet` (with and without the `integration` tag), `staticcheck`, `golangci-lint` v2 (`.golangci.yml`) and `govulncheck`. staticcheck and govulncheck versions are pinned in the Makefile `tools` target.

## D-003 — HTML → Markdown conversion is a throwaway tool

- **Date:** 2026-10-23
- **Decision:** the guide pages were converted with `github.com/JohannesKaufmann/html-to-markdown/v2` from a scratch module outside the repo, so it is not a dependency. Only `<main>` is converted. `openapi.json` is stored byte-for-byte. See `docs/legendkeeper/FETCHED.md`.

## D-004 — Contract ambiguities A-01…A-15 resolved as proposed

- **Date:** 2026-10-23
- **Decision:** the owner approved every resolution in [`SDK_CONTRACT.md` § Ambiguities and conflicts](SDK_CONTRACT.md#ambiguities-and-conflicts) as written. Summary:

| ID | Topic | Resolution |
|---|---|---|
| A-01 | `Retry-After` > 8s | 429/503: no retry, return with `RetryAfter`. Other retryable cases: cap the wait at 8s and retry. |
| A-02 | `errors[]` statuses (guide vs spec) | Decode `errors` from any error body. Phase 2 probe. |
| A-03 | 413 | Handle plain text and JSON; `ErrPayloadTooLarge` matches both. No client-side size check in Phase 1. |
| A-04 | Unknown codes | Keep the raw code; `ErrorCode.Known()` reports false. |
| A-05 | `currentVersion` parsing | Phase 2 probe, then a tolerant parser. |
| A-06 | ETag form | Strip surrounding quotes, keep `W/` verbatim. Phase 2 probe. |
| A-07 | `Retry-After` format | Integer seconds or HTTP-date; anything else is absent. |
| A-08 | Pagination end | Continue only on a non-empty `nextCursor`; `hasMore` without a cursor → `ErrPaginationInconsistent`. |
| A-09 | Zero projects | `ErrNoProject`, not cached. |
| A-10 | Client id validation | None; `lk.NewID()` generates valid 24-char cuid2-format ids. |
| A-11 | Rate defaults | 250 req/min (burst 10), 100 writes/min (burst 5); longer tiers via 429 handling. |
| A-12 | Timeout vs. caller deadline | Per-attempt timeout has cause `ErrTimeout` and is retried; the caller's context is never retried. |
| A-13 | `parentId` sentinels | `lk.ParentRoot`, `lk.ParentTemplates`. |
| A-14 | Per-call project id | Wins; does not touch the cache. |
| A-15 | Header case | `Idempotency-Key` (canonical); no behavior impact. |

## D-005 — Code generation: stdlib types-only generator (`lkgen`), hand-written calls

- **Date:** 2026-10-23
- **Context:** the spec is OpenAPI **3.1.0**, with 42 `type: [T, "null"]` nullables, 34 `anyOf`, 6 `oneOf` (4 with a `kind` discriminator and an explicit mapping: `Block`/`VersionedBlock` ×28, `PropertyBlock`/`VersionedProperty` ×9), numeric `exclusiveMinimum`/`exclusiveMaximum` (3.1 form) and `allOf` (`ProjectDetail`, `ResourceWithAncestors`). Each option was prototyped in a scratch module against the **Resources + Projects** operations (12 ops) of the unmodified spec, sha256 `99109f62…3942b`.

### Results

| Criterion | A. oapi-codegen v2.8.0 (types + client) | B. ogen v1.24.0 | C1. oapi-codegen types only | C2. stdlib generator in repo (`internal/cmd/lkgen`) |
|---|---|---|---|---|
| Parses the spec as published | ✅ | ❌ fails on 3.1 numeric `exclusiveMinimum: 0` (`cannot unmarshal !!int into bool`) | ✅ | ✅ |
| 3.1 nullables | ✅ only with `nullable-type: true` (`nullable.Nullable[T]`). Default output is `*string` + `omitempty`, which **cannot send `{"parentId":null}`** | n/a | ✅ same as A | ✅ `Nullable[T]` from our own package; exact-JSON test passes (`{"parentId":null}`, `{"name":"x"}`, `{"tags":[]}`, `{}`) |
| `anyOf` | `json.RawMessage` wrapper + `As*/From*` helpers | ❌ "complex anyOf not implemented". With `ignore_not_implemented` it **skips 64 of 89 operations**, including `createResource`, `getResource`, `updateResource`, `moveResource`, `restoreResource`, `createManyResources` and every Blocks op | same as A | `json.RawMessage` for now; `Run`/blocks are hand-written in `lk/blocks` (Phase 2) per CLAUDE.md |
| `kind` discriminator | `union json.RawMessage` + `ValueByDiscriminator()` returning `interface{}`; unknown kind → error | n/a | same as A | Raw-preserving union (`Kind` + `Raw`), so **unknown kinds are kept, not rejected**; variant table emitted |
| `allOf` | ✅ merged | n/a | ✅ | ⚠ prototype emits `json.RawMessage`; flattening is ~40 lines, needed for `ProjectDetail` |
| `CGO_ENABLED=0` build | ✅ | ✅ (after spec patch) | ✅ | ✅ |
| Our transport owns retry/idempotency/rate limit | Possible: client takes an `HttpRequestDoer`, but it builds requests itself (its own `Idempotency-Key` param, its own URL building), so ownership is split | Client bakes in OpenTelemetry and its own error decoding | ✅ calls are hand-written | ✅ calls are hand-written |
| Dependency footprint (`go mod graph` lines) | 88: `oapi-codegen/runtime` drags gin, echo, iris, bytedance/sonic, logrus, testify… into the module graph | 174, incl. `go.opentelemetry.io/otel/*`, `go-faster/*` | 88 (same runtime, needed for union helpers) | **0** (stdlib only) |
| Stable regeneration (generate twice, `cmp`) | ✅ identical | n/a | ✅ identical | ✅ identical (sorted output, `go/format`) |
| Output size for the slice | 10,286 lines | — | 6,509 lines | 1,559 lines, 134 types |

### Decision: **C2** (approved by the owner 2026-10-23) — types only, from a small stdlib generator in the repo, with hand-written calls

- B is out: it can't parse the spec, and once patched it drops the very operations Phase 1 needs.
- A and C1 work, but the runtime's module graph puts about 80 unrelated modules (web frameworks, a JIT JSON library) under "every dependency must be justified". Their default nullable handling silently loses explicit `null`. Their union decoding rejects unknown kinds, which CLAUDE.md forbids (unknown kinds must round-trip).
- C2 adds zero dependencies, emits our own three-state types directly (so there's no translation layer for `Optional`/`Nullable`), keeps unknown union kinds, and is deterministic.
- **Cost:** we own about 400 lines of generator. That's bounded because the generator only emits wire types; operations, retries and pagination stay hand-written.
- **Known work before it is the real `make generate`:**
  - `allOf` flattening
  - integer enums
  - doc comments on enum constants
  - a golden test of generator output
  - a CI check that `make generate` leaves no diff
- **Layout:**
  - `internal/cmd/lkgen` holds the generator.
  - It writes `lk/internal/gen/types.go`.
  - The three-state types live in a leaf package, `lk/internal/opt`, which both `gen` and `lk` import, so there's no cycle.
  - `lk` re-exports them as generic type aliases (`type Optional[T any] = opt.Optional[T]`, Go ≥ 1.24), plus constructor funcs.
  - Public `lk` types are hand-written where the shape matters (request structs) and type aliases of generated types elsewhere.

## D-006 — Dependencies

- **Date:** 2026-10-23

| Module | Why | cgo |
|---|---|---|
| `golang.org/x/time` v0.16.0 (`rate`) | Client-side pacing, which CLAUDE.md mandates. Its `ReserveN(t, n)` takes an explicit time, so the limiter runs on the injected clock and tests never sleep. | None (pure Go) |

| `github.com/yuin/goldmark` v1.8.6 (added 2026-10-24, Phase 2b) | CommonMark + GFM parser for `lk/markdown` (LK_MARKDOWN.md §7). Writing a CommonMark-correct parser by hand would be large and error-prone, and the converter guards player-visible content. Only `lk/markdown` imports it, so `lk` callers who don't use Markdown don't link it. | None (pure Go, no dependencies of its own; `CGO_ENABLED=0 go build` checked) |

| `github.com/modelcontextprotocol/go-sdk` v1.8.0 (added 2026-10-24, Phase 3) | The official MCP Go SDK, which CLAUDE.md mandates for lk-mcp: stdio transport, typed tools with JSON Schema input validation and structured output, in-memory transports for tests. It brings `google/jsonschema-go`, `segmentio/encoding` + `segmentio/asm` (assembly, not cgo), `yosida95/uritemplate`, `golang-jwt/jwt`, `golang.org/x/oauth2`, `x/sync`, `x/sys` and `x/tools`; jwt and oauth2 serve its HTTP auth, which lk-mcp doesn't use. Only `cmd/lk-mcp` and `internal/mcpserver` import it. | None: `CGO_ENABLED=0` builds pass for every target |
| `golang.org/x/sys` v0.44.0 (indirect) | Raised from the SDK's v0.41.0 so govulncheck reports no required-module vulnerability (GO-2026-5024, Windows-only, never called). | None |

That is the whole module graph (`go mod graph`). UUIDs (idempotency keys) and cuid2-format IDs use `crypto/rand` instead of a dependency. The generator (`internal/cmd/lkgen`) uses only the standard library.

## D-007 — Core implementation choices

- **Date:** 2026-10-23
- **Public types are hand-written. Generated types stay internal.**
  - Responses map wire `Optional`/`Nullable` fields to plain Go values: `""` means absent and `time.Time{}` means unset. Times are converted to UTC.
  - Requests use `lk.Optional`/`lk.Nullable` for every optional field, so nothing is sent unless the caller set it.
  - `TestRes_C_WIRE_07_SpecDriftDetected` lists every wire field. A regenerated schema with a new field fails until the public type maps it, or the test names it as deferred:
    - `Document.MapImage` and `Document.Sources` are deferred to phase 4.
    - `ResourceDocumentParams.Blocks` is deferred to phase 2.
- **`Optional` never encodes `null`.** A set nil slice encodes as `[]` and a set nil map as `{}`, because only `Nullable` carries null. `Some[[]string](nil)` would otherwise send `"aliases":null` to a field the spec doesn't allow to be null.
- **Sentinels are the `ErrorCode` values.** `ErrorCode` implements `error`, and each `ErrX` is a constant alias of `CodeX`. `errors.Is(err, lk.ErrNotFound)` goes through `APIError.Is`, which compares codes, so there are no hand-maintained sentinel vars.
- **The per-attempt timeout runs on the injected clock.** `clock.AfterFunc` cancels the attempt with cause `ErrTimeout`. That's how A-12 tells a timeout from caller cancellation, and it lets tests fire a timeout without waiting.
- **Misuse fails before any request is sent, instead of being silently ignored:**
  - `WithIdempotencyKey` on a non-POST call returns `ErrIdempotencyKeyNotAllowed`.
  - `IfMatch` on an unversioned operation returns `ErrIfMatchNotAllowed`.
  - An empty path parameter is an error, so `Delete(ctx, "")` can never turn into a request on a different endpoint.
  - `Projects.Get("")` never falls back to the resolved project.
- **Escape-hatch headers (`Client.Do`) are passed through as written.** An `Idempotency-Key` header counts as the call's key for retry purposes only on POST, matching the server, which ignores it on other methods.
- **Delete takes a params struct** (`ResourceDeleteParams{Cascade: true}`) rather than a variadic or boolean flag, so a cascade is always visible at the call site.

## D-008 — Phase 2a contract resolutions (A-16 to A-32)

- **Date:** 2026-10-23
- **Status:** approved by the owner as proposed in [SDK_CONTRACT.md](SDK_CONTRACT.md#ambiguities-and-conflicts). Rows marked *probe* are confirmed or corrected by D-009.

| ID | Topic | Resolution |
|---|---|---|
| A-16 | Omitted fields on replace | The SDK never adds or strips fields. *Probe* records which fields persist. |
| A-17 | `plainText` | Match the server's `DocumentBlocks.text`, from *probe* golden output. |
| A-18 | `walk` paths | Pre-order; `blocks[i]`, `.children[j]`, `.rows[k]`. *Probe* against `errors[].path`. |
| A-19 | Row write `version` | Body if present, else `ETag`. *Probe*. |
| A-20 | Versioned deletes | 204, no version. |
| A-21 | `BlockPatch.view` | Not exposed (spec defect). |
| A-22 | Single-page envelopes | Single page; `hasMore: true` → `ErrPaginationInconsistent`. |
| A-23 | Upload headers | Send the returned headers exactly; add `Content-Type` only if missing; never API headers. *Probe*. |
| A-24 | Mention `name` echo | *Probe*; omit on write if rejected. |
| A-25 | Empty map block | `blocks.EmptyMap`. |
| A-26 | Row key length | No client check. |
| A-27 | `contentHash` | `UploadImage` always sends it. |
| A-28 | Unknown enum values | Decoded verbatim. |
| A-29 | Key reuse after 412 | Documented; generated keys unaffected. |
| A-30 | Client body limit | 1,048,576 bytes. |
| A-31 | Storage upload retries | None; no API rate-limit wait; per-attempt timeout applies. |
| A-32 | Unmodelled fields on known kinds | Kept from decode and re-encoded. |

## D-009 — Phase 2a probe observations

- **Date:** 2026-10-23
- **Source:** `lk/probe_integration_test.go` against the sandbox (`make test-integration ARGS='-run TestProbe -v'`). Everything the probes write is made up and deleted.

| ID | Observation | Consequence |
|---|---|---|
| A-02 | Inserting a block with a used id → 409 `DUPLICATE_ID` with `errors: [{"path":"block.id", …}]`. | The guide is right; `Errors` is decoded on any status (unchanged). |
| A-05 | 412 message: `Document version is <current>, If-Match was <sent>`. | `CurrentVersion` is parsed from `Document version is (\S+),`; anything else leaves it empty. |
| A-06 | Every ETag is **weak**: `W/"666d8c8987139620"`, while the body's `version` is `666d8c8987139620`. The server accepts bare, quoted and `W/`-quoted If-Match values alike. A 204 `deleteRow` sends a strong quoted ETag. | **Changed from D-004 (owner approved):** strip `W/` and quotes, so a version from an ETag equals the body's. |
| A-16 | `replaceBlocks` / `blocks.replace` without optional fields: image `title`, `isTitleHidden`, `layout`, `width` **are kept**; image `alt`, `fill`, paragraph `align`/`background`, layout column `width`/`background`, callout `icon`/`color` **are removed**. A `custom` callout sent without `icon`/`color` reads back as tone `info`. | The guide's "omitted field keeps its value" applies to the `layout`, `width` and title fields only. The SDK still sends exactly what it was given (read-modify-write is lossless); `Replace` doc comments state the rule. |
| A-17 | `text` is one line per text-bearing block, joined with `\n`, no trailing newline. Contributing: paragraph, heading, quote/list/table/callout/expand/layout/secret/textField children (recursively), task items, `code` (its `text`, newlines kept). A `break` run is `\n`. A mention is its `text`, else its `name`. Not contributing: empty paragraphs, `rule`, `keyValue` rows, `infobox` (including its `textField` rows), `meter`, `resourceLink`, `expand` titles, block titles. | `PlainText` reproduces this; golden file `lk/blocks/testdata/plaintext_golden.json`. |
| A-18 | Paths: `blocks[1].children[0].children[0].bogus`; infobox row child `blocks[0].rows[1].children[0].bogus`. A top-level `listItem` → `VALIDATION_ERROR` at `blocks[0].kind` (the top-level discriminator excludes `listItem`, `taskItem`, `tableRow`, `tableCell`, `layoutColumn`), not `INVALID_CONTENT`. | `Walk` paths as proposed. |
| A-19 | Row write bodies carry no `version`; the version is in the ETag (`W/"…"`), and equals the next `listRows.version`. | Row writes fill `Version` from the ETag (C-VER-04). |
| A-20 | `deleteRow` → 204 with an ETag. | The guide wins: deletes return no version. `CaptureResponse` exposes the ETag for callers who want it. |
| A-23 | `createImageUpload` returns `method: PUT`, an S3 URL and headers `content-type` and `x-amz-checksum-sha256`. A PUT with exactly those headers → 200; `createImage` → 201 with `width`/`height`. | As proposed; `Content-Type` is already in `headers`, so nothing is added in practice. |
| A-24 | Echoing blocks as read, including mention `name`, is accepted and leaves `text` unchanged. | Mentions are echoed as read; nothing is stripped. |
| — | Inside an infobox, a `textField` row's paragraph has no `id`. Rows of keyValue blocks and properties carry `rank`. | Ids are optional on decode everywhere. |
| C-CONTENT-03 | A `data:` image URL → 400 `VALIDATION_ERROR` at `blocks[0].url`. | Server-side rule confirmed; covered by the probe. |

## D-010 — Phase 2a implementation choices

- **Date:** 2026-10-23
- **The block types are the wire types** (`lk/blocks`).
  - Each of the 28 kinds is a hand-written struct with the spec's field names.
  - Required fields are plain; optional ones are `lk.Optional` with `omitzero`.
  - Block, property, run and filter types are not generated, and they don't map to generated structs. Duplicating 28 kinds as both wire and public types would add boilerplate and nothing else, and the sealed `Block` interface needs its own types anyway.
  - Drift is caught by `TestBlock_C_BLOCK_03_FieldsMatchSpec`, which compares every struct's JSON field set with its spec schema, including inline row and item objects and the three run shapes. A removed field fails the test (checked by mutation).
  - The generator still produces the simpler wire types (documents, assets, search, tags) and every `Op`.
- **Type names follow the spec: `ParagraphBlock`, `ImageProperty`, and so on.** That leaves the guide's builder names free: `P`, `H`, `Quote`, `Image`, `Callout`, `Layout`. The callout's `Tone` and the layout's `BlockLayout` keep their spec names too.
- **Nothing the SDK reads is dropped:**
  - An unknown kind decodes to `RawBlock` or `RawProperty`.
  - A known kind in a shape the SDK doesn't expect also decodes raw, rather than failing the whole page.
  - Fields the SDK doesn't model are kept and written back in name order after the modelled ones (A-32).
  - Unknown run shapes decode to `RawRun`.
  - Unknown enum values are kept as is.
  - Only JSON that isn't an object with a string `kind` is an error.
- **Read-only fields are echoed back.** A mention's `name` is sent back as read (D-009, A-24). The `version` of a versioned response is removed before the block is decoded, so it can't come back as an unmodelled field and be written into the page.
- **Builders take `...any` for text and children.** Strings, runs, blocks and `[]any` mix freely, as in the guide (`P("Hi ", Bold("there"))`). Anything else becomes its `fmt.Sprint` text, never a panic. Kinds that need children take a typed first argument.
- **Options are fields, not option bags.** Builders take only what the guide's positional arguments take; set everything else on the returned struct (`img.Fill = lk.Some(true)`). `Column(width, …)` treats a width of 0 or less as unset.
- **`Walk` visits `Node`s.** Infobox rows are properties, not blocks, so `Walk` and `FindBlock` work over a `Node` interface that both blocks and properties satisfy. The paths match the server's error paths (D-009, A-18).
- **Operations take a `DocRef{ResourceID, DocumentID}`.** Block and row ids follow positionally. This keeps calls readable while still sending the five path ids correctly.
- **`IfMatch` is split from version reporting.** Reads that report a version (`getBlocks`, `blocks.get`, row and infobox lists) fill it from the ETag but refuse `IfMatch`. Versioned writes do both. `assignBlockIds` reports a version and refuses `IfMatch` (C-CORE2-02). The escape hatch accepts `IfMatch`, since it can't know whether an endpoint is versioned.
- **Sidebar property creates drop `rank`** and send the placement as `before`/`after`, because a create with `rank` is a 400.
- **Union fields in patches are separate Go fields.** `BlockPatch.Rows`/`InfoboxRows` and `MeterItems`/`ResourceItems` both map to one wire field each; setting both fails before sending (`ErrPatchConflict`).
- **Storage uploads go through the same `http.Client`, without the SDK's headers.**
  - The request carries the returned headers plus a `Content-Type` only if they lack one. It has no `Authorization`, `Idempotency-Key`, `If-Match`, `Accept` or `User-Agent`; Go's default User-Agent is suppressed too.
  - It gets the per-attempt timeout, no retries and no rate-limit wait (A-31).
  - `UploadImage` detects the content type with `http.DetectContentType` and refuses anything outside the five accepted types before any request.
- **`Resource.Properties` is now `[]blocks.Property`.** This replaces Phase 1's `lk.RawProperty` placeholder.
- **`ResourceDocument.Blocks`** carries page content on `Resources.Create`, which closes Phase 1's deferred `ResourceDocumentParams.Blocks`.
- **Integration polling.** `TestIntegration_SearchAndTags` polls search for up to 30s with real sleeps, because the index may lag a write. That is the only real sleep in any test, and unit tests still never sleep.
- **Sandbox sweep covers assets.** `sandbox.Sweep` also deletes root assets named with the run prefix, and `sandbox.CleanupAsset` registers deletes.

## D-013 — Supported platforms: macOS and Linux only

- **Date:** 2026-10-24
- **Decision (owner):** lk-mcp is built and supported for `darwin/arm64`, `darwin/amd64`, `linux/amd64` and `linux/arm64` only. Linux covers server deployments such as AWS Lightsail. Windows is not a target: no Windows builds, no Windows-specific code paths, now or later.
- `make build` builds those four targets, and `make verify-static` fails on any other binary in `dist/`. CLAUDE.md's build section says the same.
- **Amended 2026-10-25 (owner, Phase 6 approval):** no Intel Mac support at all. `darwin/amd64` is dropped; the targets are `darwin/arm64`, `linux/amd64` and `linux/arm64`, and `verify-static` refuses any other binary.

## D-011 — lk-mcp tools and spoiler guard semantics (Phase 3)

- **Date:** 2026-10-24
- **Owner decisions (Phase 3 planning):**
  - The guard checks **all outgoing content** of a write, not only what the edit changed. A page that already holds a protected term outside DM-only content can't be written until that is fixed; the error names each term and where it is.
  - **DM-only content** is a `secret` block, a hidden document, a hidden resource (all of it), a hidden sidebar property and a hidden infobox row.
  - v1 tools are the read tools; create/update resource; edit page; sidebar properties; image upload; delete/restore resource; and document create/rename/hide.
  - Without a terms file, writes are **refused** unless `LK_MCP_GUARD=off` is set.
- **Tools are task-shaped.** Pages are read and written as LK Markdown (`read_page` gives the version; `edit_page` takes the whole page and that version and writes with `markdown.Replace`). Structured output carries ids, names and versions; page text is only in the text content, so a page isn't sent twice.
- **Read-only mode registers no write tools** rather than registering tools that refuse.

## D-012 — Guard details not covered by the docs

- **Date:** 2026-10-24
- **Hidden is not inherited.** The docs say a hidden entity is invisible to lower roles, but not that a hidden resource hides its children. The guard treats only an entity's own flag as hidden.
- **Generic visible-string walk.** Content is encoded to JSON and every string is checked, skipping objects with `"kind":"secret"` or `"isHidden":true`. This needs no per-kind list, covers lk-raw blocks and unmodelled fields, and fails toward checking more.
- **Only properties can be hidden.** `"isHidden":true` exempts an object only when its `kind` is a property kind (`textField`, `keyValue`, `image`, `meter`, `resourceCollection`, `resourceLink`, `embed`, `map`, `unsupported`): a sidebar property or an infobox row. On any other object (a run, a paragraph) it exempts nothing, so a field the API might ignore can't hide player-visible text from the guard. Hidden documents and resources are exempted by the tool, from the target's own flag.
- **Exemptions are positional** (red-team review): `"kind":"secret"` exempts an object only where it is a block, an element of a `blocks` or `children` array; `"isHidden":true` (with a property kind) only where it is a property, the value of `property` or an element of a `properties` or `rows` array. The checked value itself and its top-level elements may be either. Before this, lk-raw could put `"kind":"secret"` (or `"kind":"textField","isHidden":true`) on a text run, which the SDK preserves and sends, and the guard skipped that run's text.
- **Write tools run one at a time** (red-team review). The MCP SDK runs tool calls concurrently, and a guard decision rests on reads (a target's hidden flag, the content an unhide shows), so lk-mcp holds one write lock from a write tool's first read to its last write. Edits made outside lk-mcp (the LegendKeeper app) between its read and its write are not covered.
- **Content the guard can't read is not shown** (red-team review). Map, board and timeline documents have no content lk can read yet, so the guard used to check only their names when one was shown. Now, with the guard on, `update_document` refuses to unhide such a document, and unhiding or restoring a resource with one visible refuses (restore keeps that resource hidden). The hint says to review and show it in LegendKeeper, or hide that document first. Revisit in Phase 4, when lk can read them.
- **Duplicate JSON keys** (possible in lk-raw or unmodelled fields): an object is exempt only when every `kind` member is `secret` (or every `isHidden` is `true` and every `kind` is a property kind), so the guard is safe whether the server keeps the first or the last duplicate; every occurrence of a duplicated string member is checked.
- **Normalisation:** case is compared by Unicode simple folding, and format characters (soft hyphen, zero-width spaces and joiners) are dropped before matching. There is no NFC/NFKC normalisation (the standard library has none and no dependency is added), so decomposed accents and look-alike letters from other scripts evade the guard; it is a safety net for honest mistakes, not a defence against a DM deliberately disguising terms.
- **Whole-word matching, per string and per block text.** Each string is checked on its own, and each `content` array (a block's or row's runs) is also checked as the joined text players read, so a term split across runs (`Zor**blax**`, `the Crimson [Accord](url)`, across a line break) is caught and reported at the `content` path. A run without text (a mention) joins as a separator. Multi-word terms match across whitespace runs.
- **An empty terms file fails closed.** A rewrite that truncates the file before writing it (as `os.WriteFile` and many editors do) can be read as empty, which would let every write through; the race test caught exactly that. So a file with no terms is an error, and a file whose size or mtime changes while it is read is not used.
- **The terms file is re-read when it changes** (mtime and size), so a DM can add a term without restarting the server. It is never logged; logs give only the number of violations. A read is refused (the write fails, and the next Check tries again) when the file's stat differs before and after the read, or when a file that held terms now holds none: an editor rewriting the file in place truncates it first, and a Check in that window ran with no terms (red-team review). Emptying the terms file on purpose needs a restart.

## D-014 — Phase 4 probes and decisions (maps, timelines, calendars, templates)

- **Date:** 2026-10-24. Probes: `lk/probe4_integration_test.go` against the sandbox.
- **A-33:** feature, lane and event creates and updates return a weak `ETag` but no body `version`, and their operations take no `If-Match`. They are unversioned in the SDK; the ETag is ignored.
- **A-34:** `isHidden` on features and events round-trips on create and update; absent reads as false. Whether players see hidden pins and events isn't observable with a key: the guard checks them like public content until the owner confirms in the player view (X-GUARD-11).
- **A-35:** `saveAsTemplate` and `createFromTemplate` with `includeChildren` keep each copied resource's `isHidden` (a hidden child stays hidden in the template and in the copy). The template itself has `parentId` `templates`. So the guard checks a copy resource by resource, each by its own flag.
- **A-36:** a calendar's body `version` equals its `ETag` (weak form, `W/"…"`). A stale `If-Match` answers 412 `VERSION_MISMATCH` with "Calendar version is <current>, If-Match was <sent>"; `CurrentVersion` now parses that form too.
- **A-37:** `PixelToPos`/`PosToPixel` are linear across `bounds`: `lat = north + (y/height)(south − north)`, `lng = west + (x/width)(east − west)`, and the inverse. The spec fixes the frame (north and west 0 at the image's top-left, south negative, east positive); the guide gives no formula, and the server stores `pos` verbatim, so this is our derivation, tested by corners and round trips.
- **Owner decisions (Phase 4 planning):** SDK plus MCP tools; full calendar editing through MCP; templates are treated as player-visible (copies are checked); hidden features and events become DM-only only after the player-view check.
- **Spec findings (maps):**
  - **Conflict:** a feature's `parentId` is typed string-only on create and update, while its description says "null = root". The spec wins on shape, so `ParentID` is `Optional[string]` and the SDK can't move a feature back to the map root.
  - `visibilityRange` is write-only (in the create/update params, not the `Feature` schema); `isSynced`, named in the `iconColor` description, is not a field and isn't modelled.
  - `mapImage` comes back on document get, create and update, not on list or `getResource`; `sources` on list, get, create and update, not on `getResource`. A legacy tiled map has `assetId`, `url` and `tiles` null.
  - Read label sizes are free strings (app-drawn features read `small`/`medium`/`large`); writes are limited to the spec's `xs`–`3xl`.
  - The spec's `MapImage` description gives the pixel formula itself, identical to A-37's, so the helpers follow the spec. The sandbox confirmed `bounds` `[[-128,0],[0,256]]` for a 512×256 image and exact pixel round trips.
- **Spec findings (timelines, calendars, templates):**
  - **Conflict:** changing a built-in calendar answers 400 `KIND_MISMATCH` (spec and sandbox), while the guide's example catches it as `ValidationError`. The spec wins on shape: the SDK documents and tests `ErrKindMismatch`.
  - Events can read back with type `placeholder` or `invisible-placeholder`; `startDate` is always present, `endDate` and `iso` optional.
  - `listEvents` and `listLanes` on a chaining timeline include its sources'; `getLane` on a source's lane answers 404.
  - `listTemplates` returns the `resources.list` item shape.
  - An explicit `null` on an optional calendar field reads as unset and isn't written back.
- **lk-mcp map and template tools:**
  - A template's own name and the name given to a copy are always checked, even when the root resource is hidden (stricter than resources): a template list shows names.
  - `create_from_template` without a name checks the template's full name, which contains every word of the server's default (the name without "Template").
  - The template tree is walked with `Templates.All` grouped by `parentId`, the documented listing, rather than `Resources.List` with a template id as parent.
  - `set_map_feature` and `set_map_image` accept a `blank` document named explicitly, since the write converts it to a map; `read_map` refuses one.
  - `set_map_feature` checks the feature as it will be (the current feature with the changes laid over it) plus the map's name.
- **lk-mcp timeline and calendar tools:**
  - `set_timeline_event` requires `type` on create rather than defaulting it, so nothing is sent the caller didn't give; `type` isn't changeable on update.
  - A timeline's chained sources are checked even when the source document or resource is hidden, because what a chain shows players isn't documented (fail safe).
  - Setting a custom `calendar_id` on a visible timeline checks that calendar's spec, since its names render on the timeline.
  - The API has no single-calendar read, so `get_calendar` takes the calendar from the list. Dry runs of replace and delete warn when the version is stale.
  - A calendar `spec` is decoded into the lossless `lk.CalendarSpec`; required fields the caller leaves out go out as zero values (the API replaces the whole spec).
- **Red-team review (Phase 4):**
  - **A hidden timeline that a visible one chains is player-visible.** Chains are checked even through hidden sources (above), so `set_timeline_lane`, `set_timeline_event` and `update_document sources` on a hidden timeline now look for a visible timeline that chains it, directly or through other timelines, and check the write when one does. LegendKeeper has no reverse lookup: the scan lists the project's resources and the documents of each one holding a timeline. Only writes to hidden timelines pay for it, and a failed scan refuses the write.
  - **Showing a timeline checks its custom calendar.** A calendar's names render on every timeline using it, so showing a timeline (unhide, unhide or restore its resource, template copy) checks its calendar's spec too, found by the `calendarId` that `getResource` carries.
  - **A calendar spec is checked under a key.** As the guard's root value, a kept unmodelled `"kind":"secret"` (or a property kind with `"isHidden":true`) made the whole spec exempt. The API rejects those fields (`additionalProperties: false`), but a dry run passed them.
  - **A default copy name is checked as the server may form it.** The guide says only that the copy's name "drops the word Template". If that happens inside a word, `ZorblaxTemplate` copies as `Zorblax`, which whole-word matching on the template's name misses. Without a name, `create_from_template` also checks the name with every `template` (any case) removed, both outright and leaving a word break. This replaces the "covers every word" assumption above.
  - **Known limit: map embeds.** A `map` block or property on a visible page shows a map document, which may be hidden or in a hidden resource. Whether players then see that map's features is not documented (the API refuses non-admins an embed of a map hidden from them). Neither writing such a block nor `set_map_feature` on the embedded hidden map checks the map's features. Proposed general fix: one "what does this show" resolver for references (timeline chains, map blocks and properties) used by every write. A write to a hidden map or timeline checks when any visible page, property or timeline references it, and a write that adds a reference checks what it references. It needs a project-wide reference index, because maps are embedded in page blocks, and that is costly to build per write. Until then, keep spoiler maps out of embeds, or check the owner's player view as for X-GUARD-11.
  - **Known limit: derived text.** `formattedStart`/`formattedEnd` and `startDate` names are rendered by the server from the calendar. An event update in minutes checks the event's old formatted text, not the new one, which the server derives. The calendar itself is checked when it is created, replaced, set on a visible timeline or shown, so only a calendar edited in the app can bring a term in this way.

## D-015 — Phase 5 probes and decisions (boards)

- **Date:** 2026-10-25. Probes: `lk/probe5_integration_test.go` against the sandbox.
- **A-38:** every board read and write returns `version` in the body. The `ETag` carries the same value, sometimes weak (`W/"…"`) and sometimes strong, so the existing body-then-ETag rule (D-009) holds. A stale `If-Match` answers 412 `VERSION_MISMATCH` with "Document version is <current>, If-Match was <sent>", which `CurrentVersion` already parses.
- **A-39:** a `blank` document reads as an empty board (no pages, no shapes) with a `version`, and that version is accepted as `If-Match` on the first create. The create converts the document to `board` and makes its page (`page:page`).
- **A-40:** `listShapes` returns the same `version` as `getContent`.
- **Records are hand-written JSON types.** `BoardShape`, `BoardBinding`, `BoardAsset` and `BoardPage` carry JSON tags and keep `props` and `meta` as `map[string]json.RawMessage`, so values come back exactly as read. The generated wire types supply only the operations. The input types double as the decode target for lk-mcp's JSON tool arguments. Each record's `Input()` sends it back with every field; `typeName` and an empty `meta` are left out, since they are the defaults. A sandbox replace with a board's own records leaves it unchanged.
- **Client-side limit:** more than 500 records of one kind, or ids, fails with `ErrBoardBatchTooLarge` before any request (spec `maxItems`).
- **Nothing on a board is hidden.** tldraw shapes have no hidden flag, so the guard checks a visible board's whole content, and checks nothing on a hidden board or a board under a hidden resource.
- **lk-mcp board tools:** `read_board`, `add_board_shapes`, `update_board_shapes`, `delete_board_shapes` and `replace_board`. Every write requires `version` and sends it as `If-Match`. Records are passed as JSON objects and decoded strictly: an unknown field is refused before any request, and `typeName` may be left in, so records from `read_board raw=true` can be sent back. The destructive dry runs read the board, name what would go, and warn when `version` is stale.
- **Guarding boards:** the guard checks the outgoing records and the board's name, and skips them when the board document or its resource is hidden. Showing a board document (unhide, restore, template copy) reads and checks the whole board, and board documents are no longer refused. A shape's props may hold keys the guard exempts elsewhere (`blocks` with a `secret` block in a content-block shape, `rows`, `properties`). Whether the app hides those on a board isn't documented, so they are renamed before the check (`strictContent`) and exempt nothing.
- The version-mismatch hint now names `read_page`, `read_board` and `get_calendar`, not just pages.

## D-016 — Phase 6: Claude Desktop extension and Cowork plugin

- **Date:** 2026-10-25.
- **Owner decisions (Phase 6 approval):** no Intel Mac support (D-013 amended), so there is one extension, `dist/lk-mcp-darwin-arm64.mcpb`.
- **Two pieces.**
  - The **Claude Desktop extension** (`.mcpb`, manifest 0.3, `server.type: binary`) runs lk-mcp and asks for its settings. The API key is masked and stored by Desktop.
  - The **Cowork plugin** (`plugin/.claude-plugin/plugin.json` plus `skills/`) carries only the two skills. Cowork doesn't prompt for settings (a `${user_config.*}` server without defaults is skipped), so it has no safe place for the key. The server comes from the Desktop extension. Whether Cowork tasks see the Desktop extension's tools is for the owner to confirm in the app. If they don't, the fallback is a plugin `.mcp.json` whose key comes from a file outside the repo, not built until needed.
- **Settings map to existing variables:** api_key → `LEGENDKEEPER_API_KEY`, guard_file → `LK_MCP_GUARD_FILE`, spoiler_guard → `LK_MCP_GUARD`, read_only → `LK_MCP_READ_ONLY`. Desktop substitutes a boolean setting as `true`/`false`, so `LK_MCP_GUARD` now accepts those as well as `on`/`off`. Named extra projects aren't offered, because a manifest has a fixed set of settings.
- **Substitution, from the mcpb package source** (`dist/shared/config.js`, `getMcpConfigForManifest`): defaults are applied, then the user's values; booleans become `"true"`/`"false"`; a reference to a setting with neither is left in the string literally. So the optional terms file has `default: ""`, which lk-mcp ignores as unset. X-PLUG-01 checks that every referenced setting is required or defaulted, and X-PLUG-02 re-implements the substitution to check the resulting config.
- **Tooling:** `@anthropic-ai/mcpb` 2.1.2 (released 2025-12-04), run with `npx -y` from `make mcpb` to validate and pack. It is a build-only dev tool and not part of the Go module or the binary. The packed binary is the usual `CGO_ENABLED=0` darwin/arm64 build, and `verify-static` checks it inside the `.mcpb`.
- **Version:** `plugin/mcpb/manifest.json` holds the release version (semver, `0.1.0`). The plugin manifest must match it (X-PLUG-03). The binary's `--version` remains `git describe`.

## D-017 — No terms file means full DM access

- **Date:** 2026-10-25.
- **Owner decision:** "writing is a big part of the purpose of this MCP server and skill." Without a terms file, lk-mcp now allows every write, unchecked, as `LK_MCP_GUARD=off` does. This replaces D-011's "with no guard file, refuse writes". The guard is opt-in: set `LK_MCP_GUARD_FILE`, or pick a file in the Desktop extension.
- With a terms file, nothing changes: an empty file, or one that changes while it is being read, still fails closed. An empty file is something the owner set up, so it fails loudly rather than silently switching protection off.
- `list_projects` reports the guard as `on`, `off`, or `off (no terms file)`. The Desktop extension drops its separate "Spoiler guard" toggle, since leaving the terms file empty now does the same thing. `LK_MCP_GUARD` stays for environment-variable users. The extension and plugin version moves to 0.2.0.

## D-018 — Cowork is part of Claude Desktop

- **Date:** 2026-10-25.
- **Owner correction:** Cowork has been merged into Claude Desktop. It is a mode of the Desktop app, not a separate product, and it uses the extensions installed in Desktop.
- So the Desktop extension (`.mcpb`) is how lk-mcp reaches both chat and Cowork, with one set of settings and the API key stored once. The plugin zip adds only the skills.
- This supersedes D-016's open question ("whether Cowork tasks see the Desktop extension's tools"). The fallback described there, a plugin `.mcp.json` server with the key in a file, isn't planned.
- The docs now say "Claude Desktop (chat and Cowork)", and the plugin is called the skills plugin. Cowork's packaging rules from D-016 (no top-level `bin/`, no prompting for settings) still apply to the plugin.

## D-019 — Full Claude Desktop support: prompts, minimum version, why two files

- **Date:** 2026-10-25. Sources: https://claude.com/docs/plugins/platform-support, https://claude.com/docs/plugins/overview, https://code.claude.com/docs/en/plugins-reference, https://github.com/modelcontextprotocol/mcpb/blob/main/MANIFEST.md
- **No one-file install.** A plugin can name a bundled `.mcpb` as its `mcpServers` (it is extracted to `.mcpb-cache/`). But the platform-support table lists local MCP servers, `.mcpb` bundles included, as **ignored in Desktop chat**. In Cowork, a server that references `${user_config.*}` is "ignored when a referenced option has no default; Cowork doesn't prompt for values", and the API key can't have a default. The extension is the only Desktop path that prompts for settings and stores the key securely, so we keep two files: the `.mcpb` (server) and the plugin zip (skills).
- **Skills in chat:** the plugins overview says plugin skills apply in chat (web, desktop, mobile), Cowork and Claude Code. They are listed under Customize › Skills, and uploaded with Customize › Add › Upload plugin.
- **Minimum Desktop version:** `compatibility.claude_desktop` is a semver range ("Minimum Claude Desktop version required"). No source names the first Desktop version that supports manifest 0.3, and what Desktop does on a mismatch isn't documented. The floor is `>=3.6556.2`, the owner's installed version and the one the extension is tested with. Lower it only when an older version is shown to work.
- **MCP prompts:** lk-mcp offers `summarize_topic` (read) and `edit_page_safely`, `add_map_location` and `session_notes` (write, registered only when the write tools are). Each is one user message that names the tools in order. Whether Claude Desktop shows MCP prompts isn't documented; Claude Code lists them. The skills cover the same workflows, so nothing depends on Desktop showing prompts. X-PROMPT-03 checks every tool a prompt names exists.
- The extension and plugin are now version 0.3.0.

## D-020 — Deleting assets (`delete_asset`)

- **Date:** 2026-10-25. **Owner request:** after Claude Desktop replaced an image, it couldn't remove the old one ("the connector has no way to delete assets").
- **A-41, from the sandbox probe (`lk/probe6_integration_test.go`):**
  - After `assets.delete`, `assets.get` answers 404, and deleting again answers 404.
  - A page that already showed the image keeps the image block and its URL. The file URL still served (HTTP 200) right after the delete. The page can be rewritten unchanged with the URL in it.
  - Writing that URL to *another* page fails with 400 VALIDATION_ERROR ("Images must be LegendKeeper assets of this project").
  - How long the file stays served after a delete isn't documented. So the tool says pages keep the image, but doesn't promise for how long, and suggests replacing it on those pages first.
- **The tool:** `delete_asset {asset_id, dry_run}`, destructive and idempotent.
  - It reads the asset first, to name it. For a folder it counts the contents at any depth, capped at 1000, because deleting a folder deletes everything inside it.
  - The guard doesn't run, since a delete shows players nothing.
  - A key without the ADMIN role gets the API's 403 with the existing "an admin key may be needed" hint.
- The wiki skill now spells out how to replace an image: upload the new one, swap the URL with `edit_page`, then `delete_asset` the old one.

## D-021 — Phase 7: 100% API coverage in lk-mcp

- **Date:** 2026-10-25. **Owner request:** "I want our MCP server to have 100% support for everything that the API does." It was prompted by Claude Desktop being unable to reorder sidebar entries: `moveResource` takes `after`/`before`, but `update_resource` didn't expose them.
- **Audit:**
  - The SDK lacked only the five member and invite operations. They are now `Members` and `Invites` (C-MEM, C-INV).
  - lk-mcp lacked whole operations: block-level edits, rows, infobox properties, document delete, restore and assign-ids, batch create, asset folders and updates, members and invites.
  - Many request fields were missing too: positioning, icons, lock, banner, permissions, client ids, every document type, full width, tab rank, property kinds, event detail, polygons, visibility range.
  - Some read output was too thin to act on, such as `rank`.
- **Enforcement:**
  - `internal/mcpserver/coverage.go` maps every `operationId` to its tools, and every body and query field to the `tool.param` that carries it. `TestCoverage_X_COV_01` reads the spec and the live tool schemas, and fails on any unmapped operation or field, or any missing tool or parameter.
  - The only "internal" fields are `listProjects`'s `cursor`/`limit`, which the SDK pages itself when resolving a key's project.
  - `docs/API_COVERAGE.md` is generated from the table (`make coverage-doc`).
- **Design choices:**
  - Content is LK Markdown everywhere it is text: blocks (`insert_block`/`replace_block`), properties of any kind (`property_markdown`, the directive `get_resource` renders), and row values (inline).
  - `update_block` takes `patch` in the API's own field names, decoded strictly, with `rows`/`items` decoded by the block's kind.
  - One `set_row`/`delete_row` pair covers all three row targets (sidebar property, keyValue block, infobox property). One `set_property`/`delete_property` pair covers the sidebar and infoboxes (`document_id`+`block_id`).
  - Reordering lives in `update_resource` (`after`/`before`), which already moved resources. `list_resources` now shows `rank`, with a parent's children sorted by rank as plain strings (the guide's order).
  - Every block, row and infobox write sends the version it read as `If-Match`.
- **Spoiler guard** (only with a terms file; D-017):
  - Block writes are checked unless the page or resource is hidden, or the block is or lands inside a `secret` block. Moving a block out of a secret into the open checks it (X-GUARD-15).
  - Rows are checked unless their resource, page or property is hidden.
  - Asset folder names, and a visible asset's name and description, are checked (X-GUARD-16).
  - `restore_document` restores, hides, checks and shows again, like `restore_resource` (X-GUARD-14).
  - Raising the MEMBER role's access on a resource is treated like unhiding it. Lowering it is not checked. A resource with MEMBER access 0 is still not treated as DM-only, which is fail-safe (X-GUARD-13).
  - Members, invites, deletes, moves and reorders aren't checked.
- **Members and invites:**
  - `invite_member` is annotated open-world because it emails someone. It is the only tool that is.
  - The skills tell Claude to confirm with the user before inviting, removing or revoking.
  - The sandbox integration test invites a reserved, non-deliverable `@example.com` address and revokes it at once. `remove_member` is unit-tested only, since the sandbox has no throwaway member.
- The tool count goes from 40 to 60 (17 read, 43 write). The extension and plugin version is 0.4.0.

## D-022 — Icon glyphs

- **Problem:** Claude Desktop couldn't set any resource icon. Every glyph it tried was rejected with `VALIDATION_ERROR: Invalid icon name. Must be a valid FontAwesome icon.` The guide says only "a Font Awesome icon name, such as `crown` or `dog`".
- **Probe (sandbox, 2026-10-25)** on `iconGlyph`:
  - Accepted: bare, lowercase Font Awesome 6 names, Free and Pro (`crown`, `castle`, `swords`, `dagger`, `unicorn`, `shield-halved`, `house`).
  - Rejected: prefixes (`fa-crown`, `fas fa-crown`, `fa-solid fa-crown`), capitals (`Crown`), Font Awesome 5 names (`home`, `shield-alt`, `map-marked-alt`), and brand icons (`fort-awesome`, `wolf-pack-battalion`).
  - The 400 carries no field error, only that message.
- **Decision:**
  - **The directory.** `internal/fontawesome` embeds a directory of every Font Awesome 6 Free icon with a non-brand style: 1,402 icons with labels, categories, search terms and the older names each replaced.
    - It is generated by `internal/cmd/faicons` (`make icons`) from Font Awesome Free 6.7.2's own metadata (`icon-families.json`, `categories.yml`; MIT, see `internal/fontawesome/NOTICE.md`).
    - It is the canonical list. It is not re-checked against the sandbox, per the owner.
  - **Normalizing.** lk-mcp normalizes every `icon_glyph` before sending it: it lowercases it, drops Font Awesome class words and the `fa-` prefix, joins words with hyphens, and renames an older name to its current one using Font Awesome's alias data (597 names, e.g. `home` → `house`).
  - **No local allow-list.** A name outside the Free directory is still sent, since LegendKeeper also takes Pro names, so the server stays the judge.
  - **`find_icon`** (read-only, no API call) searches the directory by name, label, term and category, so an agent looks a name up instead of guessing. A rejected icon's hint (matched on the message, since there is no field) points to it.
  - **Suggestions.** `glyphSuggestions` is a short list for a world wiki, all from the Free directory. The wiki skill lists the same names.
- The extension and plugin version is 0.4.2.
