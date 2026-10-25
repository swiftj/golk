# CLAUDE.md — legendkeeper-go

## What this repo is

Three deliverables, built in this order:

1. **`lk`** — a Go SDK for the LegendKeeper v3 REST API. It matches the official TypeScript SDK (`@legendkeeper/sdk`) in *behavior*, but is idiomatic Go in *shape*.
2. **`lk-mcp`** — an MCP server for LegendKeeper, built on `lk` and the official MCP Go SDK.
3. **A Claude plugin** — skills plus the `lk-mcp` binary, for Claude Desktop and Cowork (later phase).

The owner uses this to publish a D&D world wiki that **players read**. Correctness, data safety and never leaking DM-only content matter more than speed.

## Sources of truth

Build only against LegendKeeper's public documentation, stored in the repo:

| File | Source |
|---|---|
| `docs/legendkeeper/openapi.json` | https://www.legendkeeper.com/api/v3/openapi.json |
| `docs/legendkeeper/sdk-guide.md` | https://www.legendkeeper.com/developers/guide/ |
| `docs/legendkeeper/style-guide.md` | https://www.legendkeeper.com/developers/style/ |
| `docs/legendkeeper/quickstart.md` | https://www.legendkeeper.com/developers/quickstart/ |
| `docs/legendkeeper/FETCHED.md` | Fetch date, spec `info.version`, sha256 of `openapi.json` |

Rules:

- **Never guess** an endpoint, field, enum value or behavior. If the docs don't say, stop and ask. If asking isn't possible, write an integration probe test against the sandbox and record what you observe in `docs/DECISIONS.md`.
- **When the docs disagree:** the spec wins for request/response *shape*, and the guide wins for client *behavior*. Record every conflict in `docs/DECISIONS.md`.
- **`docs/SDK_CONTRACT.md`** holds the numbered behavioral requirements extracted from the guide. Every requirement has at least one test that names its ID.

## Layout

```
cmd/lk-mcp/            MCP server entrypoint
lk/                    public SDK: client, options, errors, retry, pagination, namespaces
lk/internal/gen/       generated from openapi.json — NEVER hand-edit; regenerate with `make generate`
lk/blocks/             block types, builders, readers (Walk, FindBlock, PlainText)
lk/markdown/           Markdown <-> block conversion (phase 2)
internal/mcpserver/    tools, spoiler guard, config (phase 3)
internal/testutil/     fakes, fake clock, sandbox helpers
examples/              Go ports of the guide's examples (integration-tagged)
docs/                  sources of truth, SDK_CONTRACT.md, DECISIONS.md
plugin/                plugin bundle (phase 6)
```

Module path: `github.com/swiftj/golk`.

## Build: static, CGO-free, always

Every release binary is built with cgo disabled. No exceptions.

```sh
CGO_ENABLED=0 GOOS=<os> GOARCH=<arch> \
  go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" \
  -o dist/lk-mcp-<os>-<arch> ./cmd/lk-mcp
```

Build targets: `darwin/arm64`, `linux/amd64`, `linux/arm64`. **Only Apple Silicon macOS and Linux are supported** (Linux covers server deployments such as AWS Lightsail). No Intel Macs. Never build, ship or add code paths for Windows.

Rules:

- **No dependency that needs cgo.** Before adding any dependency, confirm it builds with `CGO_ENABLED=0` and record why it's needed in `docs/DECISIONS.md`. Prefer the standard library.
- **`make verify-static` must pass.** Every binary in `dist/` must show `CGO_ENABLED=0` in `go version -m <binary>`. Linux binaries must also report `statically linked` from `file`.
- **macOS is not fully static, and that's expected.** Go binaries there always link the system `libSystem` dynamically, even with cgo off. On macOS, the `CGO_ENABLED=0` check is the requirement.
- **The race detector needs cgo.** `make test-race` sets `CGO_ENABLED=1` for tests only. Never ship a binary built that way.

## Commands

| Command | Does |
|---|---|
| `make generate` | Regenerate `lk/internal/gen` from `docs/legendkeeper/openapi.json` |
| `make lint` | `gofmt -l`, `go vet`, `staticcheck` (or `golangci-lint`), `govulncheck` |
| `make test` | Unit tests (`CGO_ENABLED=0`) |
| `make test-race` | Unit tests with `-race` |
| `make test-integration` | `-tags integration` against the sandbox project |
| `make build` | Cross-compile all targets into `dist/` |
| `make verify-static` | Check every binary in `dist/` as described above |

Run `make lint test` before every commit.

## Go conventions

- **Go version.** Use the current stable release. Range-over-func iterators (`iter.Seq2`) are expected.
- **Context.** `context.Context` is the first parameter of every call that does I/O. Respect cancellation, and leak no goroutines.
- **Client.** The client is safe for concurrent use and built with functional options: `lk.New(apiKey, lk.WithBaseURL(...), lk.WithHTTPClient(...), ...)`. `lk.NewFromEnv()` reads `LEGENDKEEPER_API_KEY` and `LEGENDKEEPER_BASE_URL`. `New` never reads the environment implicitly.
- **Errors.**
  - `*lk.APIError` carries `Status`, `Code`, `Message`, `Errors []FieldError`, `RequestID`, `RetryAfter` and `CurrentVersion`.
  - `*lk.ConnectionError` wraps the underlying cause.
  - Error codes are typed string constants covering the full code catalogue in the guide.
  - Callers branch on code with `errors.Is(err, lk.ErrVersionMismatch)` (implemented by `APIError.Is` matching on `Code`) and inspect details with `errors.As`. Never branch on `Message`.
- **Three-state fields.** The API changes only the fields you send, and it rejects unknown fields, so a Go zero value must never be sent by accident.
  - `lk.Optional[T]` means omitted or set.
  - `lk.Nullable[T]` means omitted, explicit `null`, or set. For example, `parentId: null` moves a resource to the root.
  - Tests assert the exact JSON bytes sent.
- **Block model.**
  - `Block` is a sealed interface, discriminated by `kind`, with custom JSON marshalling.
  - `unsupported` blocks keep their `id` and are echoed back on full-page replaces. Dropping one deletes the user's content.
  - A kind this SDK doesn't recognize decodes into a raw block that preserves its JSON, rather than failing.
- **Times.** Use `time.Time` in UTC.
- **Library hygiene.** No panics, no global mutable state, and no logging in library code. Observability comes from an optional `*slog.Logger` and `http.RoundTripper` middleware.

## SDK behavior contract — highlights

`sdk-guide.md` is authoritative. `SDK_CONTRACT.md` is the full list. These are the non-negotiables:

1. **Auth.** Send `Authorization: Bearer <key>`. The default base URL is `https://www.legendkeeper.com`.
2. **Project resolution.** If `projectID` is omitted, resolve it once via `projects.list` and cache it for the client's lifetime.
   - If resolution fails, retry it on the next call.
   - If the credential reaches more than one project, return an error asking for an explicit project ID.
3. **Timeout.** The default is 30s per attempt, and each retry gets a fresh timeout.
4. **When to retry.** The default is 2 retries.
   - `GET`, `PUT`, `PATCH` and `DELETE` are retried on 429, 502, 503, 504, a timeout or a connection failure.
   - `POST` is retried in the same cases, because every `POST` carries an `Idempotency-Key` that is reused across attempts. It is also retried on 409 `IN_PROGRESS`.
   - Never retry on context cancellation, on 500, or on any other 4xx.
5. **How long to wait between retries.**
   - Honor `Retry-After`. When it exceeds 8s on a 429 or 503, don't retry: return the error with `RetryAfter` set. (The guide has a detail on this that conflicts; resolve it in `DECISIONS.md`.)
   - Without `Retry-After`, back off exponentially with jitter: the first wait is 250–500ms, doubling up to 8s.
6. **Idempotency.**
   - Generate one UUID per `POST` call and reuse it on every retry of that call.
   - Callers can pass their own key, or disable generated keys.
   - Keys are 1–255 characters from `[A-Za-z0-9._-]`.
   - Surface 409 `IDEMPOTENCY_KEY_REUSED` clearly.
7. **Versioned writes.**
   - `IfMatch` maps to the `If-Match` header, and a mismatch returns 412 `VERSION_MISMATCH`.
   - Versions are opaque strings; compare them only for equality.
   - When a response body lacks a version, copy it from the `ETag` header.
8. **Pagination.**
   - Server-paged lists: `assets`, `projects`, `resources`, `templates`, `timelines.listEvents`. Cursors are opaque.
   - Every other list returns the whole collection on one page.
   - Expose every list as an iterator.
9. **Rate limits.**
   - Pace requests client-side with `golang.org/x/time/rate`, defaulting to safely under 120 writes/min and 300 requests/min per key.
   - Use `createMany` (up to 100 items, counted as one write) for bulk creates.
10. **Error bodies.**
    - Error bodies look like `{message, code, requestId, errors?}`.
    - Take `RequestID` from the `x-request-id` header, falling back to the body.
    - A request body over 1MB gets a plain-text 413 from the host, with no code and no request ID.
11. **Content rules.**
    - A `blank` document converts on its first typed write.
    - Image fields accept only the project's own asset URLs, and `data:` and `blob:` URIs are rejected.
    - Client-supplied resource IDs are cuid2, 8 or 24 characters.

## MCP server rules (phase 3+)

- **SDK and transport.** Use `github.com/modelcontextprotocol/go-sdk`. The default transport is stdio, and HTTP is optional and comes later.
- **Logging.** stdout is reserved for the protocol, so log to stderr with `slog`. Never log API keys, and never log page content at INFO.
- **Tools.**
  - Tools are task-shaped, not 1:1 copies of the endpoints.
  - Every tool carries MCP annotations: read-only, destructive or idempotent.
  - Every write tool supports `dry_run`.
- **Modes.**
  - Read-only mode is set with `LK_MCP_READ_ONLY=true`.
  - Multiple projects: `LEGENDKEEPER_API_KEY` is the default project, and `LEGENDKEEPER_API_KEY_<NAME>` adds a named one.
- **Spoiler guard.**
  - Protected terms load from the file at `LK_MCP_GUARD_FILE`. That file lives outside this repo and is **never committed**.
  - A write to player-visible content that contains a protected term outside a `secret` block or a hidden document is rejected.
  - Tests use made-up terms only.
- **Tool output.** Keep it concise, and include IDs, names and versions. Errors include the code, the request ID and an actionable hint.

## Testing

- **Unit tests** use `httptest.Server` fakes. Assert the exact request: method, path, query, headers (`Idempotency-Key`, `If-Match`, `Authorization`) and body JSON.
- **Retry tests** use an injected clock and random source, so tests never sleep and jitter is deterministic.
- **Table-driven tests** are the default. **Fuzz tests** cover block JSON decoding and, later, the Markdown converter.
- **Integration tests** (`//go:build integration`):
  - They use `LK_SANDBOX_API_KEY`.
  - First, call the project endpoint and **abort unless the project name equals `LK_SANDBOX_PROJECT_NAME`**.
  - Every test deletes what it creates using `t.Cleanup`.
  - Tests share one rate limiter.
- **Example ports.** Port the guide's examples into `examples/` as integration tests. They are the outside oracle for the SDK's behavior.

## Safety

- **Never commit secrets.** `.env` is in `.gitignore`.
- **Never point anything at a non-sandbox project.** The owner's real wiki is read by players.
- **Never write a key into code**, test fixtures or docs, even if one appears in a prompt.

## Workflow

- **Start every phase in plan mode.** Present the plan and wait for approval before writing code.
- **Commit small**, with conventional commit messages. Record non-obvious choices in `docs/DECISIONS.md`.
- **Don't build ahead of the current phase.**

## Phases

1. **SDK core.** Transport, errors, retry, idempotency, pagination, rate limiting, project resolution, codegen, and one vertical slice (resources).
2. **Wiki surface.** Documents, blocks, properties, search, tags, assets, and the Markdown converter.
3. **MCP server v1** on the phase 2 surface.
4. **Maps, timelines, calendars and templates.**
5. **Boards** (tldraw records).
6. **Skills and the plugin bundle.**
