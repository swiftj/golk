# SDK behavioral contract

Numbered behavioral requirements for the `lk` Go SDK, extracted from
[`legendkeeper/sdk-guide.md`](legendkeeper/sdk-guide.md) (cited as **G§Section**) and,
where the spec's own prose (`info.description` in
[`legendkeeper/openapi.json`](legendkeeper/openapi.json)) adds behavior, from the spec
(cited as **S§Section**). CLAUDE.md additions are cited as **CLAUDE.md**.

Rules:

- Every item has a stable ID `C-<AREA>-NN`. IDs are never reused or renumbered; a
  dropped item is marked **Withdrawn**.
- **Phase** says when the item becomes in scope. An in-scope item must have at least
  one test whose name contains its ID with `-` replaced by `_`
  (e.g. `TestRetry_C_RETRY_03_...`). `lk/contract_coverage_test.go` enforces this.
- Items marked ⚠ depend on a resolution in [Ambiguities and conflicts](#ambiguities-and-conflicts).
- TypeScript names are translated to Go: `TypeError` → a sentinel error,
  `signal` → `context.Context`, `timeoutMs` → `time.Duration`, `xxxError` classes →
  `*APIError` + `errors.Is` on code / status helpers.

Spec: OpenAPI 3.1.0, `info.version` 3.0.0, sha256 in [`FETCHED.md`](legendkeeper/FETCHED.md).

---

## AUTH — authentication

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-AUTH-01 | Every request sends `Authorization: Bearer <apiKey>`. | G§Client options (`apiKey`) | 1 |
| C-AUTH-02 | Constructing a client without an API key fails (`New("")` returns an error); no request is made. | G§Client options | 1 |
| C-AUTH-03 | The API key is never logged, never included in error messages, and never appears in `String()`/`%v` output of the client or its options. | CLAUDE.md §MCP Logging, §Safety | 1 |

## CLIENT — construction and options

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-CLIENT-01 | Constructing a client makes no request. The first call that needs one does. | G§Make a first call | 1 |
| C-CLIENT-02 | Default base URL is `https://www.legendkeeper.com`; `WithBaseURL` overrides it. | G§Client options, G§Run the examples | 1 |
| C-CLIENT-03 | `New` never reads the environment. `NewFromEnv` reads `LEGENDKEEPER_API_KEY` and `LEGENDKEEPER_BASE_URL`. | CLAUDE.md §Client; G§Client options Details | 1 |
| C-CLIENT-04 | In `NewFromEnv`, an empty `LEGENDKEEPER_BASE_URL` counts as unset (production is used). An explicit `WithBaseURL` option takes precedence over the env var. | G§Client options Details | 1 |
| C-CLIENT-05 | A custom HTTP client / `RoundTripper` (`WithHTTPClient`) sees every attempt, including retries. | G§Client options (`fetch`) | 1 |
| C-CLIENT-06 | An error returned by the custom HTTP client/transport becomes a `*ConnectionError` wrapping it. | G§Client options Details | 1 |
| C-CLIENT-07 | Every call takes a `context.Context`. When it is cancelled or its deadline passes, for any reason, the call returns a `*ConnectionError` whose cause is the context error (so `errors.Is(err, context.Canceled)` holds), and it is not retried. | G§Client options; CLAUDE.md §Context | 1 |
| C-CLIENT-08 | `maxRetries` default is 2; `WithMaxRetries(0)` turns retries off. | G§Client options | 1 |
| C-CLIENT-09 | `idempotencyKeys` default is on; `WithoutIdempotencyKeys()` turns generated keys off (see C-IDEM-05). | G§Client options | 1 |
| C-CLIENT-10 | The client is safe for concurrent use by multiple goroutines. | CLAUDE.md §Client | 1 |
| C-CLIENT-11 | A request whose only path parameter is `projectId` takes it optionally; all project-scoped calls fill it from the resolved project when omitted (see PROJ). | G§Make a first call | 1 |
| C-CLIENT-12 | An escape hatch `Client.Do(ctx, Request)` sends an arbitrary request with the client's key, base URL, timeout, retries and error mapping. It sends `Path` as written (no `projectId` fill-in), and does **not** generate an `Idempotency-Key`; a caller-supplied `Idempotency-Key` header makes a `POST` retry like a keyed `POST`. Nil query values are omitted. | G§The escape hatch | 1 |

## PROJ — project resolution

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-PROJ-01 | When no project ID is configured, the client resolves it with one `projects.list` call the first time a project-scoped call needs it, and caches it for the client's lifetime. | G§Make a first call Details | 1 |
| C-PROJ-02 | `WithProjectID(id)` (or a per-call project ID) skips resolution entirely. | G§Make a first call | 1 |
| C-PROJ-03 | If resolution fails, nothing is cached and the next call tries again. | G§Make a first call Details | 1 |
| C-PROJ-04 | If the credential reaches more than one project, resolution returns `ErrAmbiguousProject`, asking for an explicit project ID. | G§Make a first call Details | 1 |
| C-PROJ-05 | Concurrent first calls trigger at most one in-flight resolution request; all waiters share its result (and its error, without caching the error). A waiter's own context cancellation releases only that waiter. | CLAUDE.md §SDK contract 2; C-CLIENT-10 | 1 |
| C-PROJ-06 | `Client.Project(ctx)` returns the key's project, the same object `projects.get` returns for it. | G§projects | 1 |
| C-PROJ-07 ⚠ | A credential that reaches zero projects returns `ErrNoProject`. | — (A-09) | 1 |

## TIMEOUT

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-TIMEOUT-01 | Default per-attempt timeout is 30s; `WithTimeout` overrides it. | G§Client options | 1 |
| C-TIMEOUT-02 | Each retry gets a fresh per-attempt timeout. | G§Client options | 1 |
| C-TIMEOUT-03 | If both the per-attempt timeout and the caller's context deadline apply, the attempt ends at whichever comes first. Only the per-attempt timeout is retryable; the caller's deadline is not (C-CLIENT-07). | G§Client options | 1 |
| C-TIMEOUT-04 | A per-attempt timeout surfaces as `*ConnectionError` whose cause satisfies `errors.Is(err, lk.ErrTimeout)`. | G§Error classes (`TimeoutError`) | 1 |

## ERR — errors

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-ERR-01 | Every non-2xx response returns `*APIError` with `Status`, `Code`, `Message`, `Errors []FieldError{Path, Message}`, `RequestID`, `RetryAfter`, `CurrentVersion`. | G§Errors; CLAUDE.md §Errors | 1 |
| C-ERR-02 | Error bodies decode from `{message, code, requestId, errors?}`. | G§Error codes; S§Errors | 1 |
| C-ERR-03 | `RequestID` comes from the `x-request-id` response header, falling back to the body's `requestId`. | G§Errors | 1 |
| C-ERR-04 | `Errors` is populated whenever the body carries `errors` (400 `VALIDATION_ERROR`, `INVALID_CONTENT`, `UNSUPPORTED_KIND`; 409 `DUPLICATE_ID` on block writes), otherwise nil. ⚠ A-02 | G§Errors | 1 |
| C-ERR-05 | `ErrorCode` is a typed string with one exported constant per catalogue code (table below). A code the SDK does not know is preserved verbatim in `Code` and `Code.Known()` reports false. ⚠ A-04 | G§Error codes | 1 |
| C-ERR-06 | `errors.Is(err, lk.ErrXxx)` matches on `Code` (via `APIError.Is`). `errors.As(err, &apiErr)` exposes details. Callers never need to inspect `Message`. | CLAUDE.md §Errors; G§Errors (`message`) | 1 |
| C-ERR-07 | Status-class helpers mirror the TS classes: `IsValidation` (400), `IsAuthentication` (401), `IsPermission` (403), `IsNotFound` (404), `IsConflict` (409), `IsVersionMismatch` (412), `IsRateLimit` (429), `IsServer` (5xx). | G§Error classes | 1 |
| C-ERR-08 | A plain-text 413 from the host (body > 1 MB) returns `*APIError{Status: 413}` with empty `Code` and `RequestID` and a message stating the 1 MB limit; it is not a decode failure. A JSON 413 body is decoded normally. ⚠ A-03 | G§Error codes (`PAYLOAD_TOO_LARGE`); S§Errors | 1 |
| C-ERR-09 | Any non-JSON or malformed error body still yields `*APIError` with the HTTP status, never a panic or JSON error. | C-ERR-08 generalised; CLAUDE.md §Library hygiene | 1 |
| C-ERR-10 | `*ConnectionError` means no response arrived (DNS, TCP, TLS, timeout, cancellation). It has no `RequestID`, is not an `*APIError`, and `Unwrap()` returns the cause. | G§Error classes | 1 |
| C-ERR-11 | `RetryAfter` on a 429 (and any response with `Retry-After`) is parsed from the header. | G§Error classes (`retryAfterMs`) | 1 |
| C-ERR-12 ⚠ | On 412, `CurrentVersion` is parsed from the message when it can be; otherwise empty. | G§Error classes; A-05 | 2a |
| C-ERR-13 | A request body with an unknown field is rejected by the server with `VALIDATION_ERROR`; the SDK must therefore never send a field the caller did not set (see C-WIRE-01). | G§Errors Details | 1 |

### Error code catalogue

The spec's `ErrorCode` enum and the guide's table list the same 29 codes (verified 2026-10-23).

| Status | Code | Go constant / sentinel |
|---|---|---|
| 400 | `VALIDATION_ERROR` | `CodeValidationError` / `ErrValidation` |
| 400 | `INVALID_CONTENT` | `CodeInvalidContent` / `ErrInvalidContent` |
| 400 | `UNSUPPORTED_KIND` | `CodeUnsupportedKind` / `ErrUnsupportedKind` |
| 400 | `KIND_MISMATCH` | `CodeKindMismatch` / `ErrKindMismatch` |
| 401 | `UNAUTHENTICATED` | `CodeUnauthenticated` / `ErrUnauthenticated` |
| 402 | `SUBSCRIPTION_LAPSED` | `CodeSubscriptionLapsed` / `ErrSubscriptionLapsed` |
| 402 | `PAID_PLAN_REQUIRED` | `CodePaidPlanRequired` / `ErrPaidPlanRequired` |
| 403 | `FORBIDDEN` | `CodeForbidden` / `ErrForbidden` |
| 403 | `WRONG_PROJECT` | `CodeWrongProject` / `ErrWrongProject` |
| 403 | `INSUFFICIENT_SCOPE` | `CodeInsufficientScope` / `ErrInsufficientScope` |
| 403 | `FEATURE_UNAVAILABLE` | `CodeFeatureUnavailable` / `ErrFeatureUnavailable` |
| 404 | `NOT_FOUND` | `CodeNotFound` / `ErrNotFound` |
| 405 | `METHOD_NOT_ALLOWED` | `CodeMethodNotAllowed` / `ErrMethodNotAllowed` |
| 409 | `DUPLICATE_ID` | `CodeDuplicateID` / `ErrDuplicateID` |
| 409 | `AMBIGUOUS` | `CodeAmbiguous` / `ErrAmbiguous` |
| 409 | `ALREADY_DELETED` | `CodeAlreadyDeleted` / `ErrAlreadyDeleted` |
| 409 | `HAS_CHILDREN` | `CodeHasChildren` / `ErrHasChildren` |
| 409 | `NOT_DELETED` | `CodeNotDeleted` / `ErrNotDeleted` |
| 409 | `PARENT_DELETED` | `CodeParentDeleted` / `ErrParentDeleted` |
| 409 | `IN_PROGRESS` | `CodeInProgress` / `ErrInProgress` |
| 409 | `IDEMPOTENCY_KEY_REUSED` | `CodeIdempotencyKeyReused` / `ErrIdempotencyKeyReused` |
| 409 | `TOO_MANY_PENDING_INVITES` | `CodeTooManyPendingInvites` / `ErrTooManyPendingInvites` |
| 412 | `VERSION_MISMATCH` | `CodeVersionMismatch` / `ErrVersionMismatch` |
| 422 | `DOCUMENT_TOO_LARGE` | `CodeDocumentTooLarge` / `ErrDocumentTooLarge` |
| 413 | `PAYLOAD_TOO_LARGE` | `CodePayloadTooLarge` / `ErrPayloadTooLarge` |
| 413 | `STORAGE_QUOTA_EXCEEDED` | `CodeStorageQuotaExceeded` / `ErrStorageQuotaExceeded` |
| 429 | `RATE_LIMITED` | `CodeRateLimited` / `ErrRateLimited` |
| 429 | `UPLOAD_RATE_EXCEEDED` | `CodeUploadRateExceeded` / `ErrUploadRateExceeded` |
| 500 | `INTERNAL` | `CodeInternal` / `ErrInternal` |

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-ERR-14 | The catalogue above is complete: a test asserts the Go constants equal the spec's `ErrorCode` enum exactly (same set). | G§Error codes; S `ErrorCode` | 1 |
| C-ERR-15 | `ErrPayloadTooLarge` also matches the plain-text host 413 (C-ERR-08), so callers have one check for "body too large". | G§Error codes (`PAYLOAD_TOO_LARGE` is "Reserved") | 1 |

## RETRY — when to retry

"Keyed POST" = a POST that sends an `Idempotency-Key` (generated or caller-supplied).

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-RETRY-01 | A failed attempt is retried up to `maxRetries` times (so at most `maxRetries+1` attempts). | G§Retries | 1 |
| C-RETRY-02 | `GET`, `PUT`, `PATCH`, `DELETE` are retried on 429, 502, 503, 504, a per-attempt timeout, and a connection failure. | G§Retries | 1 |
| C-RETRY-03 | A keyed `POST` is retried in the same cases as C-RETRY-02, and also on 409 `IN_PROGRESS`. | G§Retries | 1 |
| C-RETRY-04 | An unkeyed `POST` is retried only on 429 and on a connection failure; never on a timeout, 502, 503 or 504 (the server may have applied it). | G§Retries Details | 1 |
| C-RETRY-05 | 409 `IN_PROGRESS` is retried only on a keyed request. | G§Retries Details | 1 |
| C-RETRY-06 | Never retried: context cancellation/deadline (caller's), 500, and every 4xx other than 429 (and keyed-POST 409 `IN_PROGRESS`). | G§Retries | 1 |
| C-RETRY-07 | When retries are exhausted, the last attempt's error is returned (`*APIError` or `*ConnectionError`) — e.g. `IN_PROGRESS` surfaces as `ErrInProgress`. | G§Idempotent writes table | 1 |
| C-RETRY-08 | A connection failure where the request may have been sent (e.g. reset after write) is still a "connection failure" for retry purposes, subject to C-RETRY-04 for unkeyed POSTs. | G§Retries | 1 |
| C-RETRY-09 | The request body is replayed byte-for-byte on every attempt. | G§Idempotent writes (same body ⇒ replay) | 1 |

## BACKOFF — how long to wait

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-BACKOFF-01 | Before a retry, if the response carries `Retry-After`, wait that long. | G§Retries | 1 |
| C-BACKOFF-02 ⚠ | A 429 or 503 whose `Retry-After` exceeds 8s is not retried: return the error immediately with `RetryAfter` set. | G§Retries (para. 2) vs Details; A-01 | 1 |
| C-BACKOFF-03 | Without `Retry-After`, back off exponentially with jitter: first wait uniformly in [250ms, 500ms], each later wait doubles, capped at 8s. | G§Retries | 1 |
| C-BACKOFF-04 | `Retry-After` is accepted as delta-seconds (the documented form) and as an HTTP-date; an unparseable value is treated as absent. ⚠ A-07 | S§Rate limits; RFC 9110 | 1 |
| C-BACKOFF-05 | Waits respect context cancellation: a cancel during a wait returns at once (C-CLIENT-07). | CLAUDE.md §Context | 1 |
| C-BACKOFF-06 | The clock and random source are injectable, so tests never sleep and jitter is deterministic. | CLAUDE.md §Testing | 1 |

## IDEM — idempotency keys

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-IDEM-01 | Every `POST` operation sends `Idempotency-Key` by default. | G§Idempotent writes | 1 |
| C-IDEM-02 | The client generates one UUID per call and sends the same key on every retry of that call. Different calls get different keys. | G§Idempotent writes | 1 |
| C-IDEM-03 | `PUT`, `PATCH`, `DELETE` and `GET` never send `Idempotency-Key`. | G§Idempotent writes; S§Idempotency | 1 |
| C-IDEM-04 | A caller can pass their own key on any POST call (`lk.WithIdempotencyKey(k)`); it replaces the generated key and is reused across retries. | G§Idempotent writes | 1 |
| C-IDEM-05 | With generated keys disabled, no key is sent unless the caller passes one; a caller key is still sent. | G§Client options; G§Idempotent writes | 1 |
| C-IDEM-06 | A caller key must be 1–255 characters from `[A-Za-z0-9._-]`; an invalid key is rejected client-side before any request is sent (the server would reject it with 400 `VALIDATION_ERROR` anyway). | G§Idempotent writes Details; S `idempotency-key` pattern | 1 |
| C-IDEM-07 | 409 `IDEMPOTENCY_KEY_REUSED` surfaces as `*APIError` matching `ErrIdempotencyKeyReused`, is not retried, and its `Error()` text tells the caller to use a new key for a different request. | G§Idempotent writes table; CLAUDE.md §SDK contract 6 | 1 |
| C-IDEM-08 | Generated keys match the allowed alphabet (UUID text form). | G§Idempotent writes Details | 1 |
| C-IDEM-09 | `WithIdempotencyKey` passed to a non-POST call is an error, not silently ignored ("Only operations that send POST accept idempotencyKey"). | G§Idempotent writes | 1 |

## VER — versioned writes

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-VER-01 | `IfMatch(version)` sends `If-Match: <version>`. | G§Versioned writes | 1 (mechanism) |
| C-VER-02 | 412 `VERSION_MISMATCH` returns `*APIError` matching `ErrVersionMismatch`; not retried. | G§Versioned writes; G§Retries | 1 |
| C-VER-03 | Versions are opaque strings: the SDK never parses, orders or modifies them; equality only. | G§Versioned writes | 1 |
| C-VER-04 | When a versioned response body lacks `version`, the client copies it from the `ETag` header. ⚠ A-06 | G§Versioned writes Details | 1 (mechanism) |
| C-VER-05 | A write that returns no body returns no version. | G§Versioned writes Details | 2a |
| C-VER-06 | `IfMatch` is accepted only by operations the guide lists as versioned (`documents.replaceBlocks`, every `blocks` write, sidebar property row writes, every `boards` write, calendars). | G§Versioned writes | 2a (documents, blocks, properties); 4–5 (boards, calendars) |

## PAGE — pagination

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-PAGE-01 | Server-paged lists take `limit` and `cursor`: `assets`, `projects`, `resources`, `templates`, `timelines.listEvents`. | G§Pagination | 1 (projects, resources) |
| C-PAGE-02 | Cursors are opaque: the SDK sends back exactly the `nextCursor` it received, never builds or parses one. | G§Pagination | 1 |
| C-PAGE-03 | Every list is exposed as an iterator (`iter.Seq2[T, error]`) that yields every item from the current page to the end of the collection, fetching as it goes. | G§Pagination; CLAUDE.md §SDK contract 8 | 1 |
| C-PAGE-04 | A page exposes its items, `HasMore` and `NextCursor`, plus a manual `Next(ctx)` that fetches the next page and reports done after the last. | G§Pagination | 1 |
| C-PAGE-05 | Non-paged lists return the whole collection in one page with `HasMore` false and no cursor, behind the same iterator shape. | G§Pagination | 2a |
| C-PAGE-06 | Breaking out of the iterator early sends no further requests. An error ends iteration after yielding `(zero, err)` once. | CLAUDE.md §Context | 1 |
| C-PAGE-07 | Every page fetch uses the same filters (`parentId`, `limit`) as the first page. | G§Pagination | 1 |
| C-PAGE-08 ⚠ | `hasMore` and `nextCursor` are both optional in the spec. Iteration continues only when `nextCursor` is a non-empty string. `hasMore: true` with no cursor returns `ErrPaginationInconsistent` rather than silently truncating. | S `ResourceList`; A-08 | 1 |
| C-PAGE-09 | `limit` outside 1–100 is the server's to reject (400 `VALIDATION_ERROR`); the SDK does not clamp it. | S `listResources.limit`; G§Errors example | 1 |

## RATE — rate limits

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-RATE-01 | Client-side pacing with `golang.org/x/time/rate`: one limiter for all requests and one for writes (POST/PUT/PATCH/DELETE), shared by every call on the client (and across clients when a limiter is shared explicitly). | CLAUDE.md §SDK contract 9 | 1 |
| C-RATE-02 ⚠ | Defaults are safely under the documented per-key minute tiers (300 req/min, 120 writes/min). | G§Rate limits; A-11 | 1 |
| C-RATE-03 | Every attempt, including retries, waits on the limiter(s) first; a write waits on both. Waiting respects the context. | CLAUDE.md §SDK contract 9 | 1 |
| C-RATE-04 | `WithRateLimits(...)` overrides the defaults; `WithRateLimiter(...)` lets several clients share one; limits can be disabled explicitly. | CLAUDE.md §Testing ("share one rate limiter") | 1 |
| C-RATE-05 | 429 handling follows RETRY/BACKOFF; exhausted retries return `ErrRateLimited` with `RetryAfter` set when the server sent it. | G§Rate limits | 1 |
| C-RATE-06 | `createMany` (1–100 items) counts as one write against the limiter. | G§Rate limits; G§Creating many resources | 1 |

## WIRE — request encoding (three-state fields)

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-WIRE-01 | Request bodies contain only the fields the caller set. Go zero values are never sent by accident. Tests assert exact JSON bytes. | G§resources (`update` changes only the fields you send); G§Errors Details; CLAUDE.md | 1 |
| C-WIRE-02 | `Optional[T]` encodes "omitted" or "set"; `Nullable[T]` encodes "omitted", explicit `null`, or "set". | CLAUDE.md §Three-state fields | 1 |
| C-WIRE-03 | `Resources.Move` with `ParentID: lk.Null[string]()` sends `{"parentId":null}` (move to root). | G§Stable ids and tree order | 1 |
| C-WIRE-04 | On update, `tags` and `aliases` replace the whole list; an explicitly set empty list sends `[]`. | G§resources | 1 |
| C-WIRE-05 | JSON bodies are sent with `Content-Type: application/json`; requests send `Accept: application/json` and a `User-Agent` identifying `golk/<version>`. | G§The escape hatch (`body` as JSON) | 1 |
| C-WIRE-06 | Timestamps decode to `time.Time` in UTC. | G§Namespaces; CLAUDE.md §Times | 1 |
| C-WIRE-07 | Response decoding tolerates unknown fields (forward compatibility); request encoding never emits them. | Spec-is-shape rule; G§Errors Details | 1 |

## RES — resources slice

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-RES-01 | `Resources.List` — `GET .../resources`, server-paged, optional `parentId`, `limit`, `cursor`. | G§resources; S `listResources` | 1 |
| C-RES-02 | `Resources.Get` — `GET .../resources/{id}`; returns documents and sidebar properties. | G§resources | 1 |
| C-RES-03 | `Resources.Create` — `POST .../resources`; `name` required; optional `id`, `parentId`, `after`, `before`, `documents` (≤5), tags, aliases, icon fields. | G§resources; G§Stable ids | 1 |
| C-RES-04 | `Resources.CreateMany` — `POST .../resources/batch`, 1–100 items, returned in order. | G§Creating many resources | 1 |
| C-RES-05 | `Resources.Update` — `PATCH .../resources/{id}`; changes only the fields sent. | G§resources | 1 |
| C-RES-06 | `Resources.Move` — `POST .../resources/{id}/move` with any of `parentId` (id or null), `after`, `before`. | G§Stable ids and tree order | 1 |
| C-RES-07 | `Resources.Delete` — `DELETE .../resources/{id}`; `Cascade: true` sends `?cascade=true`; without it, 409 `HAS_CHILDREN` surfaces as `ErrHasChildren`. | G§Deleting and restoring | 1 |
| C-RES-08 | `Resources.Restore` — `POST .../resources/{id}/restore`. | G§Deleting and restoring | 1 |
| C-RES-09 | `Projects.List` (paged) and `Projects.Get`. | G§projects | 1 |
| C-RES-10 | Path parameters are escaped with `url.PathEscape`. | — (defensive; ids are server-validated) | 1 |

## CONTENT — content rules

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-CONTENT-01 | Client-supplied resource/lane/event/feature ids are cuid2: a lowercase letter followed by 7 or 23 lowercase letters or digits. `lk.NewID()` generates a valid 24-char id. The server remains the authority (no client-side rejection). ⚠ A-10 | S§Identifiers; G§Stable ids | 1 (helper) |
| C-CONTENT-02 | A new resource has one `blank` document named `Main`; the first typed write converts it; `blank` never fails with `KIND_MISMATCH`. | G§Make a first call; G§Error codes Details | 2a |
| C-CONTENT-03 | Image fields accept only the project's own asset URLs; `data:`/`blob:` URIs are rejected in `url`/`src`/`href`/`*Url`/`*Src` fields. | G§Image URLs | 2a |
| C-CONTENT-04 | `unsupported` blocks keep their `id` and are echoed back on full-page replaces; unknown kinds decode to a raw block preserving JSON. | G§Unsupported blocks; CLAUDE.md §Block model | 2a |
| C-CONTENT-05 | Siblings sort by `rank` compared as plain byte strings (no locale collation). | G§Stable ids and tree order; S§Resource order | 1 (doc + helper) |

## CORE2 — core additions for Phase 2a

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-CORE2-01 | Results of versioned operations expose `Version`. That covers `documents.getBlocks`, `blocks.get` and every write that takes `IfMatch`. When the body has no `version`, it is copied from `ETag` (C-VER-04). | G§Versioned writes | 2a |
| C-CORE2-02 | `documents.assignBlockIds` returns a `Version` but is not versioned. It takes no `IfMatch`, so passing one is a misuse error. | S `assignBlockIds` (no `if-match` parameter); G§Block operations | 2a |
| C-CORE2-03 | Single-page lists that carry a version (`RowList`, `BlockPropertyList`) expose `Items` and `Version`. | S `RowList`, `BlockPropertyList` | 2a |
| C-CORE2-04 ⚠ | A JSON request body over the host limit fails before sending, with an error matching `ErrPayloadTooLarge` (C-ERR-15). No request is made and no rate-limit token is spent. | G§Error codes (`PAYLOAD_TOO_LARGE`); A-03, A-30 | 2a |
| C-CORE2-05 | `CaptureResponse(&meta)` fills `ResponseMeta{Status, RequestID, ETag, Header}` from the final attempt's response, on success and on `*APIError`. It is left empty on `*ConnectionError`. | Phase 1 report; CLAUDE.md §MCP Tool output (request id) | 2a |
| C-CORE2-06 | `IfMatch` is accepted by exactly the operations listed in C-VER-06 that are in scope (documents.replaceBlocks, every `blocks` write including infobox properties and their rows, and `properties` row writes). Every other wrapped operation rejects it before sending (`ErrIfMatchNotAllowed`). The escape hatch (`Client.Do`) accepts it, since the SDK can't know whether an unwrapped endpoint is versioned. | G§Versioned writes; S `if-match` parameters | 2a |

## DOC — documents

All paths are under `/api/v3/projects/{projectId}/resources/{resourceId}/docs`.

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-DOC-01 | `Documents.List`: `GET …/docs`. Returns a single page (C-PAGE-05) of documents with the same fields as `Documents.Get`. | G§documents; S `DocumentList` | 2a |
| C-DOC-02 | `Documents.Get`: `GET …/docs/{documentId}`. Returns `ID, Name, Type, Rank, IsHidden, IsFullWidth`, the timestamps (UTC) and `CalendarID`. `mapImage` and `sources` are kept raw until Phase 4. | G§documents; S `Document` | 2a |
| C-DOC-03 | `Documents.Create`: `POST …/docs` with `name` and `type` required, and `isHidden`, `isFullWidth`, `calendarId` optional. It's a keyed POST. | G§documents | 2a |
| C-DOC-04 | `Documents.Update`: `PATCH …/docs/{documentId}` sends only the fields set: `name`, `rank`, `isHidden`, `isFullWidth`, `calendarId`. `mapImage` and `sources` are Phase 4. | G§documents | 2a |
| C-DOC-05 | `Documents.Delete` is `DELETE …/docs/{documentId}`. `Documents.Restore` is `POST …/docs/{documentId}/restore`. `ALREADY_DELETED`, `NOT_DELETED` and `PARENT_DELETED` surface as their sentinels. | G§documents; S `deleteDocument` | 2a |
| C-DOC-06 | `Documents.GetBlocks`: `GET …/docs/{documentId}/blocks`. Returns `{ID, Version, Schema, Text, Blocks}`. An unexpected `schema` value is decoded verbatim, not rejected. A `blank` document reads as an empty page and stays `blank`. | G§The block model; G§documents Details | 2a |
| C-DOC-07 | `Documents.ReplaceBlocks`: `PUT …/docs/{documentId}/blocks` with `{"blocks":[…]}`. It takes `IfMatch` and returns the whole tree with ids and the new version. | G§The block model; G§Versioned writes | 2a |
| C-DOC-08 | `Documents.AssignBlockIDs`: `POST …/docs/{documentId}/block-ids` with no body. Returns `{Assigned, Version}`. | G§Block operations | 2a |
| C-DOC-09 | `Resources.Create` accepts `Documents` (≤5, in order), each with `name`, `type`, optional flags and, for `page`, `Blocks`. The response carries no blocks. | G§Documents with content; S `ResourceDocumentParams` | 2a |

## BLOCK — block model and single-block operations

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-BLOCK-01 | `blocks.Block` is a sealed interface with one Go type per spec kind (all 28 in `Block.discriminator.mapping`). JSON is discriminated by `kind`. | G§TypeScript types; CLAUDE.md §Block model | 2a |
| C-BLOCK-02 | A block of a kind the SDK does not know decodes to `blocks.Raw`, which keeps its JSON and re-encodes it unchanged. Decoding never fails because of an unknown kind. | CLAUDE.md §Block model | 2a |
| C-BLOCK-03 | Encoding sends required fields always and optional fields only when set (`Optional`/`omitzero`). Each block type's JSON field set equals its spec schema; a test fails on drift. | C-WIRE-01; S `*Block` schemas | 2a |
| C-BLOCK-04 ⚠ | A field the server sends on a known kind that the SDK does not model is kept and re-encoded on write, so a read → modify → write never drops it. Callers cannot set such fields. | CLAUDE.md §Block model ("dropping one deletes the user's content"); A-32 | 2a |
| C-BLOCK-05 | Enum-typed fields (`Kind`, `Layout`, `Align`, `Tone`, `EmbedProvider`, `KeyValueDisplay`, `MeterMode`, `PropertyKind`) are string types with constants. An unknown value decodes and re-encodes verbatim. | G§Enums; A-28 | 2a |
| C-BLOCK-06 | `Blocks.Get`: `GET …/docs/{documentId}/blocks/{blockId}`. Returns the block with its subtree and the document `Version`. | G§Block operations | 2a |
| C-BLOCK-07 | `Blocks.Insert`: `POST …/docs/{documentId}/blocks` with `{block, after?|before?|parent?}`. Setting more than one anchor is a misuse error before sending. With none, the block is appended. It's a keyed POST, takes `IfMatch`, and returns the block and `Version`. | G§Block operations; S `BlockInsertRequest` | 2a |
| C-BLOCK-08 | `Blocks.Update`: `PATCH …/blocks/{blockId}` with a `BlockPatch` holding only the fields set. The patch type has no `kind`, `id` or `children`. | G§Block operations; S `BlockPatch` | 2a |
| C-BLOCK-09 | `Blocks.Replace`: `PUT …/blocks/{blockId}` with a whole block. | G§Block operations | 2a |
| C-BLOCK-10 | `Blocks.Move`: `POST …/blocks/{blockId}/move` with exactly one of `after`, `before`, `parent`. Zero or several anchors are a misuse error before sending. It's a keyed POST. | G§Block operations; S `BlockMoveRequest` | 2a |
| C-BLOCK-11 | `Blocks.Delete`: `DELETE …/blocks/{blockId}`. A 204 returns no version (C-VER-05). | G§Versioned writes Details | 2a |
| C-BLOCK-12 | Every `Blocks` write accepts `IfMatch`. Every write except the deletes returns `Version`. | G§blocks ("Every write takes `ifMatch`") | 2a |
| C-BLOCK-13 | Infobox properties are addressed by the infobox's block id: `ListProperties`, `GetProperty`, `CreateProperty`, `UpdateProperty`, `DeleteProperty` under `…/blocks/{blockId}/properties`. They take the same bodies as sidebar properties and return the document `Version`. | G§Infoboxes | 2a |

## RUN — inline text

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-RUN-01 | `blocks.Run` is a sealed interface: `Text` (text plus the boolean marks `bold, italic, underline, strike, code, sub, sup` and optional `link`, `color`, `highlight`), `Mention` (`resourceId`, optional `documentId`, `text`, read-only `name`) and `Break` (`{"break":true}`). | G§Runs and marks; S `Run` | 2a |
| C-RUN-02 | Decoding picks the variant by key (`mention`, `break`, else `text`). A run matching none decodes to a raw run that is preserved. Encoding sends a mark only when it's true. | S `Run`; CLAUDE.md §Block model | 2a |
| C-RUN-03 | Run helpers follow the guide: `Mono` clears every other mark and later marks skip a mono run; `Sub` clears `sup` and vice versa; `Color`/`Highlight` also apply to mentions and skip mono runs; helpers nest. | G§Runs and marks | 2a |
| C-RUN-04 | A mention reads (in `PlainText`) as its `text` label, or else its resource's `name`. ⚠ A-24 governs whether `name` is sent back. | G§The block model | 2a |

## BUILD — builders

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-BUILD-01 | There is a builder for every builder in the guide's table. Builders return the same plain block structs callers can write by hand. | G§Builders | 2a |
| C-BUILD-02 | A kind that requires children takes the first child as a required argument (`first, rest...`), so the empty form does not compile. | G§Builders | 2a |
| C-BUILD-03 | A string passed where children are expected is a paragraph. List items take inline content (strings and runs) as a one-paragraph item, and `LI` takes nested lists after the first paragraph. | G§Builders | 2a |
| C-BUILD-04 | `Styled` applies only to paragraph, heading, quote and layout column (enforced by the type system). `WithID` sets a block's id. `AsProperty` wraps a property-capable block with an envelope. | G§Builders | 2a |
| C-BUILD-05 ⚠ | `Map` takes both ids. An empty map block (neither id) has its own constructor. | G§Maps in pages; A-25 | 2a |
| C-BUILD-06 | Builders don't validate what the server validates (callout `icon`/`color` with a non-`custom` tone, column widths, colour format). The server's 400 is the authority. | G§Builders; G§Text styles | 2a |

## READ — readers

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-READ-01 ⚠ | `PlainText` of a block, a list of blocks or a document equals the server's `DocumentBlocks.text` for the same content. | G§Readers; A-17 | 2a |
| C-READ-02 ⚠ | `Walk` visits every block in pre-order with its dotted path (`blocks[0].children[1]`), descends into `children` and infobox `rows`, and stops when the visitor returns false. The paths match the server's `errors[].path`. | G§Readers; G§Rejected writes; A-18 | 2a |
| C-READ-03 | `FindBlock` returns the first block in `Walk` order with that id, or nil. | G§Readers | 2a |

## PROP — sidebar properties

All paths are under `/api/v3/projects/{projectId}/resources/{resourceId}/properties`.

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-PROP-01 | `blocks.Property` is a sealed interface over the 8 property-capable kinds, plus `UnsupportedProperty` and a raw fallback. Each carries the envelope `ID, Rank, Title, IsTitleHidden, IsHidden`. | G§properties; S `PropertyBlock` | 2a |
| C-PROP-02 | `Properties.List` returns a single page sorted by rank. `Properties.Get` returns one property. | G§properties | 2a |
| C-PROP-03 | `Properties.Create`: `POST …/properties` with the property and an optional `before`/`after`. The create body never carries `rank` (sending it is a 400). Not versioned. | G§properties; G§Rejected writes | 2a |
| C-PROP-04 | `Properties.Update`: `PATCH …/properties/{propertyId}` merges only the fields set (envelope and kind fields). `kind` and `id` cannot be patched. Not versioned. | G§properties; S `PropertyUpdateRequest` | 2a |
| C-PROP-05 | `Properties.Delete` removes the property and its rows. | G§properties | 2a |
| C-PROP-06 | An `unsupported` property round-trips unchanged, and only its envelope can be patched. | S `listProperties`, `updateProperty` | 2a |

## ROW — key-value rows

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-ROW-01 | A row is `{ID, Rank, Key, Content []Run, Text}`, where `ID` may be absent (it's nullable in the spec). Rows are ordered by rank. | G§Key-value rows; S `Row` | 2a |
| C-ROW-02 | The three row containers (keyValue block, sidebar property, infobox property) use the same row types. Their list, create, update and delete paths are as in the guide's table. | G§Key-value rows | 2a |
| C-ROW-03 | Create sends `key` plus `content` or `text`, and an optional `before`/`after`. It's a keyed POST. | G§Key-value rows; S `RowCreateRequest` | 2a |
| C-ROW-04 | Update sends only the fields set. `Content` is `Nullable`, and `null` clears the value. `Text` is `Optional` and never sends `null`. | S `RowPatchRequest` | 2a |
| C-ROW-05 | Every row write accepts `IfMatch`. Row lists return `Items` and `Version`. | G§Versioned writes; S `RowList` | 2a |

## ASSET — assets and uploads

All paths are under `/api/v3/projects/{projectId}/assets`.

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-ASSET-01 | `Assets.List`: `GET …/assets`, server-paged, with optional `folderId`, `limit` and `cursor`. | G§assets; C-PAGE-01 | 2a |
| C-ASSET-02 | `Assets.Get` returns `ID, Name, Type, FolderID, IsHidden, Description, URL, Width, Height, CreatedAt`. An unknown `Type` (the spec also has `AUDIO`) is kept verbatim. | G§assets; S `Asset` | 2a |
| C-ASSET-03 | `Assets.CreateFolder`: `POST …/assets` with `name` and an optional `folderId`. | G§assets | 2a |
| C-ASSET-04 | `Assets.Update`: `PATCH …/assets/{assetId}` sends only the fields set. `FolderID` is `Nullable`, and `null` moves the asset to the root. The 204 returns only an error. | G§assets; S `updateAsset` | 2a |
| C-ASSET-05 | `Assets.Delete`: `DELETE …/assets/{assetId}`. | G§assets | 2a |
| C-ASSET-06 | `Assets.CreateImageUpload`: `POST …/assets/uploads` with `contentType`, `contentLength` and `contentHash`. Returns `{UploadKey, UploadURL, Method, Headers, ExpiresAt}`. | G§Uploading images; S `ImageUploadCreateParams` | 2a |
| C-ASSET-07 | `Assets.CreateImage`: `POST …/assets/images` with `uploadKey`, `name`, and optional `description` and `folderId`. | G§Uploading images | 2a |
| C-ASSET-08 ⚠ | `Assets.UploadImage` holds the bytes in memory, sends their length and SHA-256 (A-27), uploads them, then calls `CreateImage`. The content type must be one of the five accepted types. When none is given, it's detected from the bytes, and an undetectable or unaccepted type fails before any request. | G§Uploading images | 2a |
| C-ASSET-09 ⚠ | The storage upload uses the returned `method` and `uploadUrl` verbatim and sends only the returned `headers` (A-23). It never sends `Authorization` or any API header. A non-2xx answer returns `*UploadError` with the storage status. | G§Uploading images Details; A-31 | 2a |

## SEARCH and TAG

| ID | Requirement | Source | Phase |
|---|---|---|---|
| C-SEARCH-01 | `Search.Search`: `GET …/search` with `q`, plus `limit` only when set. Returns a single page of `{Resource, Document?}` in server order. Duplicates per resource are kept (one per matching document). | G§search; S `search` | 2a |
| C-TAG-01 | `Tags.List`: `GET …/tags`. Returns a single page of `{Tag, Count}` in server order. | G§tags | 2a |

## MD — LK Markdown (`lk/markdown`)

The format is ours, not LegendKeeper's ([`LK_MARKDOWN.md`](LK_MARKDOWN.md), cited as **M§n**; approved in Phase 2b). Its IDs are `M-<AREA>-NN` and follow the same rules as `C-` IDs.

| ID | Requirement | Source | Phase |
|---|---|---|---|
| M-API-01 | `FromMarkdown(ToMarkdown(p), {Base: p})` encodes to exactly the JSON of `p` for any decodable `p`, including unknown kinds, unsupported blocks and unmodelled fields. Without `Base` the result differs only in fields Markdown can't show. | M§1 | 2b |
| M-API-02 | `ToMarkdown` is deterministic: the same page always renders the same bytes. | M§1 | 2b |
| M-API-03 | `ToMarkdown` and `FromMarkdown` make no request, don't log, and never panic on any input (fuzzed). | M§1 | 2b |
| M-API-04 | `markdown.Replace` always sends `If-Match` with the base page's version. | M§6 | 2b |
| M-API-05 | `ReplaceOptions.DryRun` sends no request and returns the parsed blocks and report. | M§6 | 2b |
| M-INL-01 | Text is escaped so it round-trips verbatim, including whitespace at a line edge and at a mark's edge (kept inside the mark, written as a span flag). | M§2.2 | 2b |
| M-INL-02 | Marks render in a fixed order: link, bold, italic, strike, span, code. | M§2.2 | 2b |
| M-INL-03 | An unknown span attribute or inline HTML is an error. | M§2.2 | 2b |
| M-DIR-01 | Directive attribute names are the spec's JSON field names; values are bare tokens or quoted strings; a bare flag means `true`. | M§2.3 | 2b |
| M-DIR-02 | Every optional field can be written as an attribute; a field not written is unset. | M§2.3 | 2b |
| M-DIR-03 | Key-value rows, meter items and resource items use the `- key :: value`, `- name :: current/max`, `- @resourceId` item forms. | M§2.3 | 2b |
| M-DIR-04 | Block attributes of standard-syntax kinds go in a `{…}` set after the block. | M§2.3 | 2b |
| M-OPQ-01 | A block whose Markdown form would not parse back to the identical block renders as an `lk-raw` fence; `unsupported` renders as `::unsupported id=…`. | M§2.4, §2.5, §2.6 | 2b |
| M-OPQ-02 | An `lk-raw` fence must hold one JSON object with a string `kind`. | M§2.4 | 2b |
| M-ID-01 | With `Base`, an unchanged block comes back byte-identical to its base block. | M§3.1 | 2b |
| M-ID-02 | Editing one paragraph changes only that paragraph; its siblings and container keep their ids. | M§3.1 | 2b |
| M-ID-03 | Matching never moves an id onto a block of another kind and never uses a base block twice. Explicit `{#id}` wins over matching. | M§3.1, §3.2 | 2b |
| M-TXT-01 | The visible text of a block's Markdown equals `blocks.PlainText` of the block. | M§4 | 2b |
| M-SAFE-01 | Unknown directive, unknown or duplicate attribute, or a value the field can't hold is an error. | M§5 | 2b |
| M-SAFE-02 | An unclosed container directive or a stray closing fence is an error; an unclosed `:::secret` never leaks into the page, and a code fence holding a `:::secret` line is an error. | M§5 | 2b |
| M-SAFE-03 | Raw HTML, link reference definitions and footnotes are errors. | M§5 | 2b |
| M-SAFE-04 | Removing an `unsupported` or `lk-raw` block of `Base` (at any depth) is an error unless `AllowRemoveOpaque`; removed secrets are listed in `Report.Removed`. | M§5 | 2b |
| M-SAFE-05 | An image not alone in its paragraph, or in a heading or table cell, is an error. | M§5 | 2b |
| M-SAFE-06 | An `::unsupported` id not in `Base`, the same id twice, or an `::unsupported` moved out of the secret that holds it in `Base`, is an error. | M§5 | 2b |
| M-SAFE-07 | Input over 1 MiB, nested deeper than 64 levels, or with more than 2000 link and emphasis markers in one paragraph or heading is rejected before (inline) parsing. | M§5 | 2b |

---

## Ambiguities and conflicts

Each entry: what the sources say, and the proposed resolution. Approved resolutions are
copied into [`DECISIONS.md`](DECISIONS.md).

### A-01 — `Retry-After` over 8 seconds (guide internal conflict)

- G§Retries para. 2: *"A 429 or 503 whose `Retry-After` is longer than 8 seconds is not retried: the call throws at once, and the error's `retryAfterMs` says how long to wait."*
- G§Retries Details: *"When `Retry-After` exceeds 8 seconds, the client waits 8 seconds. A request that meets a spent per-minute tier can then exhaust its retries…"*
- G§Error classes (429): *"The client has already retried any wait of 8 seconds or less."* — supports para. 2.
- CLAUDE.md: "When it exceeds 8s on a 429 or 503, don't retry."

**Proposed:** follow para. 2 + CLAUDE.md. On **429 or 503** with `Retry-After` > 8s, return immediately with `RetryAfter` set (C-BACKOFF-02). For the **other retryable statuses** (502, 504, keyed-POST 409 `IN_PROGRESS`) with `Retry-After` > 8s, apply the Details sentence: wait 8s (cap) and retry. Rationale: two statements plus CLAUDE.md agree for 429/503, and waiting 8s against a spent minute-tier only burns the retry budget and adds write load; the Details rule is kept for the only cases it doesn't contradict.

### A-02 — Which statuses carry `errors[]`

- Guide: `errors` on 400 `VALIDATION_ERROR`/`INVALID_CONTENT`/`UNSUPPORTED_KIND` **and 409 `DUPLICATE_ID` from a block write**.
- Spec `ErrorBody.errors` and S§Errors: *"appears only on 400 responses"*.

**Proposed:** shape rule says spec wins, but the field is optional in both, so decode `errors` from any error body that carries it. No behavior depends on which statuses carry it. Record as a spec/guide conflict; add an integration probe in Phase 2 (block `DUPLICATE_ID`).

### A-03 — 413 `PAYLOAD_TOO_LARGE`

- Guide: code is *Reserved*; a >1 MB body is refused by the host as plain text with no `code`/`requestId`.
- Spec: `PAYLOAD_TOO_LARGE` — *"The body exceeds the route's limit"* (implies a JSON body is possible).

**Proposed:** handle both. Non-JSON 413 → `APIError{Status:413}` with empty `Code`; JSON 413 → decoded normally. `errors.Is(err, ErrPayloadTooLarge)` matches either (C-ERR-15). No client-side size pre-check in Phase 1 (the guide doesn't describe one; revisit in Phase 2 when block payloads get large).

### A-04 — Unknown error codes

- TS: an unknown code *"reads as `undefined`"*.

**Proposed:** Go keeps the raw string in `Code` (losing it would hurt debugging) and adds `ErrorCode.Known() bool`. `errors.Is` against any sentinel is false for an unknown code. This is a deliberate shape difference, not a behavior difference.

### A-05 — Parsing `currentVersion` from a 412 message

- Guide: `currentVersion` is *"the version parsed from the message, which can be `undefined`"*. The message format is not documented, and the guide says the message wording can change.

**Proposed:** no parser in Phase 1 (resources aren't versioned). In Phase 2, write an integration probe that triggers a 412 on `documents.replaceBlocks`, record the observed message in DECISIONS.md, and implement a tolerant parser that returns empty on any mismatch. C-ERR-12 is Phase 2.

### A-06 — ETag form

- Guide: copy `ETag` into `version` when the body lacks it. The spec doesn't declare response headers at all, and doesn't say whether the ETag is quoted or weak (`W/"…"`).

**Superseded by D-009:** the server's ETags are weak (`W/"…"`) around the body's version, so both `W/` and the quotes are stripped.

**Originally proposed:** copy the ETag value with surrounding double quotes stripped, and a `W/` prefix preserved verbatim (versions are opaque — C-VER-03). Confirm the exact form with a Phase 2 probe on `documents.getBlocks` and record it. In Phase 1, apply the mechanism only to response types that have a `Version` field (none of the resource ops do).

### A-07 — `Retry-After` format

- Spec S§Rate limits: `Retry-After` *"in seconds"*. RFC 9110 also allows an HTTP-date.

**Proposed:** parse integer seconds (documented); also accept an HTTP-date (relative to the injected clock) defensively; treat anything else, or a negative value, as absent. No fractional seconds.

### A-08 — Pagination termination

- Spec: `hasMore` and `nextCursor` are both optional; `data` is required.
- Guide: `nextCursor` is "the cursor for the next page, or `null`"; `next()` resolves null after the last page.

**Proposed:** continue only when `nextCursor` is a non-empty string. If `hasMore` is true but there is no cursor, end with `ErrPaginationInconsistent` instead of silently returning a truncated collection — silent truncation is a data-safety risk for any later sync/cleanup tool. If `hasMore` is false but a cursor is present, trust `hasMore` and stop.

### A-09 — Zero projects

- Guide covers one project and "more than one". Zero isn't mentioned (with an API key, `projects.list` holds only the key's project, so this shouldn't happen).

**Proposed:** return `ErrNoProject` (not cached; retried next call per C-PROJ-03).

### A-10 — Client-side id validation

- Spec gives the exact cuid2 pattern; the server rejects others with 400.

**Proposed:** don't validate ids client-side (the server is the authority and the pattern may widen); do provide `lk.NewID()` that always generates a valid 24-char id via `crypto/rand`, and recommend it for `createMany` (G§Creating many resources: "Give every item an `id`").

### A-11 — Client-side rate-limit defaults

- Tiers: 120 writes/min, 3,000 writes/hour, 300 req/min, 6,000 req/hour, 60,000/day, 12,000/hour per project. The TS SDK does no client pacing; CLAUDE.md requires it.

**Proposed:** defaults of **250 requests/min (burst 10)** and **100 writes/min (burst 5)** — about 83% of each minute tier. Hour/day/project tiers are not paced client-side (a token bucket for an hour window would either allow large bursts or throttle short scripts badly); they're handled by 429 + `Retry-After`. Callers running large imports can use `WithRateLimits` to drop to the guide's "1.2 s apart" (50 writes/min) for imports over 3,000 writes.

### A-12 — Timeout vs. caller deadline when retrying

- Guide: retried "on a timeout"; never retried when aborted through `signal`. In Go both look like `context.DeadlineExceeded`.

**Proposed:** distinguish by source. The SDK owns the per-attempt timer (a child context with a cause, `context.WithTimeoutCause(…, ErrTimeout)`), so a per-attempt timeout has cause `ErrTimeout` and is retryable; any error while the **parent** context is done is the caller's and is not retried.

### A-13 — `parentId` sentinels on `listResources`

- Spec: `parentId=root` lists top-level resources; `parentId=templates` lists top-level templates. The guide doesn't mention either.

**Proposed:** expose as constants `lk.ParentRoot` and `lk.ParentTemplates` (spec wins for shape). Templates themselves are Phase 4; the constant is harmless and documented.

### A-14 — Project resolution with an explicit per-call project ID vs. cached project

**Proposed:** a per-call project ID always wins and does not populate or consult the cache. `WithProjectID` on the client pre-populates the cache and `Client.Project(ctx)` then calls `projects.get` on it.

### A-15 — `Idempotency-Key` header case

- Spec parameter name is `idempotency-key`; guide/spec prose use `Idempotency-Key`. HTTP header names are case-insensitive; Go canonicalises to `Idempotency-Key`. No conflict in behavior; noted for completeness.

### A-16 — What an omitted field means on a block replace (guide internal conflict)

- G§Layout and width: *"On a replace or an update, an omitted field keeps its current value."*
- G§Block titles and image fill: *"A replace that omits `fill` removes it, as it does `alt`."*
- G§Text styles: *"To remove a style, replace the block without the field."*

**Proposed:** the SDK never adds or strips fields, so a read → modify → replace sends back everything that was read (C-BLOCK-04), and the conflict cannot cause data loss through the SDK. A Phase 2a probe replaces an image, a layout column and a paragraph without each optional field, and records which fields persist. The doc comments on `Blocks.Replace` and `Documents.ReplaceBlocks` state the observed rule.

### A-17 — `plainText` derivation

- G§Readers: `plainText` is *"derived the same way as `DocumentBlocks.text`"*.
- G§The block model: `text` is *"one line per text-bearing block"*.
- Nothing documents the separator, a trailing newline, or the treatment of tables, code, expand titles, key-value rows, secret blocks, `break` runs or `unsupported` blocks.

**Proposed:** a Phase 2a probe writes a fixture page with every text-bearing kind and records the server's `text` as a golden file (made-up content only). `PlainText` is implemented to reproduce it, and an integration test asserts `PlainText(blocks) == text` on fresh content. Where the probe can't settle a case (`unsupported`), `PlainText` contributes nothing for it and says so in its doc comment.

### A-18 — `walk` path form

- G§Readers gives one example path, `blocks[0].children[1]`, and says it descends into `children` and infobox `rows`.

**Proposed:** pre-order. The root segment is `blocks[i]`. Children are `.children[j]` and infobox rows are `.rows[k]`. A property's own children (a `textField` property) are `.rows[k].children[j]`. A probe compares these against `errors[].path` from a deliberately invalid nested write, and the probe result wins.

### A-19 — Row write responses and `version`

- S row-write operations: *"Responses carry the document `version` in the body and as `ETag`"*, but the `Row` schema has no `version` field.

**Proposed:** row writes return `Versioned[Row]`. `Version` is taken from a body `version` when present (decoded leniently), else from `ETag` (C-VER-04). A probe records which one the server sends. If it sends neither, `Version` is empty and documented as such.

### A-20 — `deleteBlock` and the other versioned deletes

- S `deleteBlock` prose says the response carries `version`, but the declared response is 204 with no body. G§Versioned writes Details: *"A write that returns no body, such as `blocks.delete`, returns no version."*

**Proposed:** the guide wins for behavior. The versioned deletes (block, row, infobox property) return only `error`.

### A-21 — `BlockPatch.view`

- S `BlockPatch` prose lists a `view` field, but `properties` doesn't define it, and the schema rejects unknown fields.

**Proposed:** not exposed (never guess). Recorded as a spec defect.

### A-22 — Single-page envelopes with paging fields

- `DocumentList`, `PropertyList`, `TagList` and `SearchResultList` declare `hasMore`/`nextCursor`, but their operations take no cursor and are documented as not paginated.

**Proposed:** treat them as single-page (C-PAGE-05). If the server ever answers `hasMore: true`, return `ErrPaginationInconsistent` rather than silently truncating (the same reasoning as A-08), since there is no cursor parameter to follow.

### A-23 — Upload request headers

- S `ImageUploadCreateParams.contentType`: *"The upload must send it as `content-type`."* G: the client sends *"only the returned `headers`"*.

**Proposed:** send the returned headers exactly. If they don't include `Content-Type`, add it with the declared type (that's the only addition). Never send `Authorization`, `Idempotency-Key`, `If-Match` or the SDK `User-Agent` override. A probe records what `headers` contains.

### A-24 — Echoing a mention's read-only `name`

- G: *"`name` is read-only."* The write schema accepts `name` in a mention.

**Proposed:** a probe writes a mention with `name` set. If the server accepts and ignores it, echo it as read. If it rejects it, the encoder omits `name` on write. Either way, `PlainText` uses it when there is no `text` label.

### A-25 — Empty map block

- G§Maps in pages allows *"neither"* id, but the TS `map` builder takes both.

**Proposed:** `blocks.Map(resourceID, documentID, opts)` takes both ids, and `blocks.EmptyMap(opts)` builds the "Link a map" block. Hand-written structs can do either.

### A-26 — Row key length

- `RowCreateRequest.key` maxLength 256; inline `keyValue.rows[].key` maxLength 2048.

**Proposed:** no client-side check; the server is the authority. Recorded as a spec inconsistency.

### A-27 — `contentHash`

- Optional in the spec; the TS `uploadImage` always sends it.

**Proposed:** `UploadImage` always sends it, lowercase hex. `CreateImageUpload` sends it only when set.

### A-28 — Unknown enum values

- Spec enums can grow (`Asset.type` already has `AUDIO`, which the guide never mentions; `DocumentBlocks.schema` is `1`).

**Proposed:** decode any string verbatim and never fail. The constants document the known values.

### A-29 — Caller-supplied key reused after a 412

- G§Idempotent writes: the key comparison *"does not include `If-Match`"*, so retrying a keyed POST block write with a fresh `IfMatch` and the same key replays the stored 412.

**Proposed:** generated keys are per call, so the normal path is unaffected. Document on `WithIdempotencyKey` and `IfMatch` that a new version needs a new key.

### A-30 — The exact client-side body limit

- G: the host refuses a body *"over 1 MB"*. Whether that is 1,000,000 or 1,048,576 bytes is not stated.

**Proposed:** refuse bodies over **1,048,576** bytes client-side. A body between the two still gets the host's 413, which C-ERR-08 already handles. Refusing at 1,000,000 could block writes the server accepts.

### A-31 — Retrying the storage upload

- The guide says only that a storage refusal throws with the storage status.

**Proposed:** the storage request gets the per-attempt timeout and no retries (the URL is signed and short-lived, and the guide describes no retry). It does not wait on the API rate limiters, since it isn't an API request. A failed `UploadImage` is retried by calling it again, which starts a new upload.

### A-32 — Fields the SDK doesn't model on a known kind

- Block schemas are `additionalProperties: false` today, so the server shouldn't send unknown fields. If a future spec adds one, a decoder that drops it would delete it on the next full-page replace.

**Proposed:** each block struct keeps unmodelled fields from decode and re-encodes them (C-BLOCK-04). The spec-drift test (C-BLOCK-03) still fails, so the SDK gets updated, but no content is lost in the meantime.
