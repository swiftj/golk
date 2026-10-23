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
