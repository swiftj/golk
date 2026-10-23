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
