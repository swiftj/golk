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
