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

## D-005 — Code generation approach (PROPOSED, awaiting owner decision)

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

### Recommendation: **C2 — types only, from a small stdlib generator in the repo, with hand-written calls**

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
