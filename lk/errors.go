package lk

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrorCode is a LegendKeeper API error code. Each code has exactly one HTTP
// status. ErrorCode implements error so that the Err* values can be used as
// errors.Is targets:
//
//	if errors.Is(err, lk.ErrVersionMismatch) { ... }
type ErrorCode string

// Error returns the code itself.
func (c ErrorCode) Error() string { return string(c) }

// The API's error code catalogue (sdk-guide.md, Error codes).
const (
	CodeValidationError       ErrorCode = "VALIDATION_ERROR"
	CodeInvalidContent        ErrorCode = "INVALID_CONTENT"
	CodeUnsupportedKind       ErrorCode = "UNSUPPORTED_KIND"
	CodeKindMismatch          ErrorCode = "KIND_MISMATCH"
	CodeUnauthenticated       ErrorCode = "UNAUTHENTICATED"
	CodeSubscriptionLapsed    ErrorCode = "SUBSCRIPTION_LAPSED"
	CodePaidPlanRequired      ErrorCode = "PAID_PLAN_REQUIRED"
	CodeForbidden             ErrorCode = "FORBIDDEN"
	CodeWrongProject          ErrorCode = "WRONG_PROJECT"
	CodeInsufficientScope     ErrorCode = "INSUFFICIENT_SCOPE"
	CodeFeatureUnavailable    ErrorCode = "FEATURE_UNAVAILABLE"
	CodeNotFound              ErrorCode = "NOT_FOUND"
	CodeMethodNotAllowed      ErrorCode = "METHOD_NOT_ALLOWED"
	CodeDuplicateID           ErrorCode = "DUPLICATE_ID"
	CodeAmbiguous             ErrorCode = "AMBIGUOUS"
	CodeAlreadyDeleted        ErrorCode = "ALREADY_DELETED"
	CodeHasChildren           ErrorCode = "HAS_CHILDREN"
	CodeNotDeleted            ErrorCode = "NOT_DELETED"
	CodeParentDeleted         ErrorCode = "PARENT_DELETED"
	CodeInProgress            ErrorCode = "IN_PROGRESS"
	CodeIdempotencyKeyReused  ErrorCode = "IDEMPOTENCY_KEY_REUSED"
	CodeTooManyPendingInvites ErrorCode = "TOO_MANY_PENDING_INVITES"
	CodeVersionMismatch       ErrorCode = "VERSION_MISMATCH"
	CodeDocumentTooLarge      ErrorCode = "DOCUMENT_TOO_LARGE"
	CodePayloadTooLarge       ErrorCode = "PAYLOAD_TOO_LARGE"
	CodeStorageQuotaExceeded  ErrorCode = "STORAGE_QUOTA_EXCEEDED"
	CodeRateLimited           ErrorCode = "RATE_LIMITED"
	CodeUploadRateExceeded    ErrorCode = "UPLOAD_RATE_EXCEEDED"
	CodeInternal              ErrorCode = "INTERNAL"
)

// errors.Is targets, one per code. errors.Is(err, ErrX) is true when err wraps
// an *APIError whose Code is X.
const (
	ErrValidation            = CodeValidationError
	ErrInvalidContent        = CodeInvalidContent
	ErrUnsupportedKind       = CodeUnsupportedKind
	ErrKindMismatch          = CodeKindMismatch
	ErrUnauthenticated       = CodeUnauthenticated
	ErrSubscriptionLapsed    = CodeSubscriptionLapsed
	ErrPaidPlanRequired      = CodePaidPlanRequired
	ErrForbidden             = CodeForbidden
	ErrWrongProject          = CodeWrongProject
	ErrInsufficientScope     = CodeInsufficientScope
	ErrFeatureUnavailable    = CodeFeatureUnavailable
	ErrNotFound              = CodeNotFound
	ErrMethodNotAllowed      = CodeMethodNotAllowed
	ErrDuplicateID           = CodeDuplicateID
	ErrAmbiguous             = CodeAmbiguous
	ErrAlreadyDeleted        = CodeAlreadyDeleted
	ErrHasChildren           = CodeHasChildren
	ErrNotDeleted            = CodeNotDeleted
	ErrParentDeleted         = CodeParentDeleted
	ErrInProgress            = CodeInProgress
	ErrIdempotencyKeyReused  = CodeIdempotencyKeyReused
	ErrTooManyPendingInvites = CodeTooManyPendingInvites
	ErrVersionMismatch       = CodeVersionMismatch
	ErrDocumentTooLarge      = CodeDocumentTooLarge
	// ErrPayloadTooLarge also matches the host's plain-text 413, which has no code.
	ErrPayloadTooLarge      = CodePayloadTooLarge
	ErrStorageQuotaExceeded = CodeStorageQuotaExceeded
	ErrRateLimited          = CodeRateLimited
	ErrUploadRateExceeded   = CodeUploadRateExceeded
	ErrInternal             = CodeInternal
)

var codeStatus = map[ErrorCode]int{
	CodeValidationError:       400,
	CodeInvalidContent:        400,
	CodeUnsupportedKind:       400,
	CodeKindMismatch:          400,
	CodeUnauthenticated:       401,
	CodeSubscriptionLapsed:    402,
	CodePaidPlanRequired:      402,
	CodeForbidden:             403,
	CodeWrongProject:          403,
	CodeInsufficientScope:     403,
	CodeFeatureUnavailable:    403,
	CodeNotFound:              404,
	CodeMethodNotAllowed:      405,
	CodeDuplicateID:           409,
	CodeAmbiguous:             409,
	CodeAlreadyDeleted:        409,
	CodeHasChildren:           409,
	CodeNotDeleted:            409,
	CodeParentDeleted:         409,
	CodeInProgress:            409,
	CodeIdempotencyKeyReused:  409,
	CodeTooManyPendingInvites: 409,
	CodeVersionMismatch:       412,
	CodeDocumentTooLarge:      422,
	CodePayloadTooLarge:       413,
	CodeStorageQuotaExceeded:  413,
	CodeRateLimited:           429,
	CodeUploadRateExceeded:    429,
	CodeInternal:              500,
}

// Known reports whether c is in this SDK's catalogue. An unknown code from a
// newer server is kept verbatim in APIError.Code.
func (c ErrorCode) Known() bool { _, ok := codeStatus[c]; return ok }

// Status returns the HTTP status the API pairs with c, or 0 for an unknown code.
func (c ErrorCode) Status() int { return codeStatus[c] }

// Errors that the SDK itself returns, not the API.
var (
	// ErrMissingAPIKey is returned by New when the API key is empty.
	ErrMissingAPIKey = errors.New("lk: an API key is required")
	// ErrAmbiguousProject is returned when no project ID is configured and the
	// credential reaches more than one project.
	ErrAmbiguousProject = errors.New("lk: the credential reaches more than one project; pass lk.WithProjectID or lk.ForProject")
	// ErrNoProject is returned when no project ID is configured and the
	// credential reaches no project.
	ErrNoProject = errors.New("lk: the credential reaches no project")
	// ErrTimeout is the cause of a ConnectionError when one attempt exceeded
	// the client's per-attempt timeout.
	ErrTimeout = errors.New("lk: request attempt timed out")
	// ErrPaginationInconsistent is returned when a page reports more items but
	// carries no cursor to fetch them with.
	ErrPaginationInconsistent = errors.New("lk: page reports hasMore but has no nextCursor")
	// ErrInvalidIdempotencyKey is returned before any request is sent when a
	// caller-supplied key is not 1–255 characters from [A-Za-z0-9._-].
	ErrInvalidIdempotencyKey = errors.New("lk: idempotency key must be 1-255 characters from A-Z a-z 0-9 . _ -")
	// ErrIdempotencyKeyNotAllowed is returned when WithIdempotencyKey is passed
	// to an operation that does not send POST.
	ErrIdempotencyKeyNotAllowed = errors.New("lk: only POST operations accept an idempotency key")
	// ErrIfMatchNotAllowed is returned when IfMatch is passed to an operation
	// that is not versioned.
	ErrIfMatchNotAllowed = errors.New("lk: this operation does not accept IfMatch")
)

// FieldError identifies one failing field or block of a rejected request.
type FieldError struct {
	// Path names the field in dotted form, such as "name" or "block.children[0].level".
	Path    string `json:"path"`
	Message string `json:"message"`
}

// APIError is returned for every non-2xx response.
type APIError struct {
	// Status is the HTTP status.
	Status int
	// Code is the API error code. It is empty when the response had none,
	// such as the host's plain-text 413 for a body over 1 MB.
	Code ErrorCode
	// Message is human-readable. Its wording can change; branch on Code.
	Message string
	// Errors lists each failing field or block, when the API sent them.
	Errors []FieldError
	// RequestID identifies the server log line. Quote it when reporting a problem.
	RequestID string
	// RetryAfter is the server's Retry-After, or 0 when it sent none.
	RetryAfter time.Duration
	// CurrentVersion is the document's version reported by a 412, when known.
	CurrentVersion string

	hasRetryAfter bool
}

// Error formats the status, code, message and request ID, plus a hint for
// codes the caller must act on.
func (e *APIError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "lk: %d", e.Status)
	if e.Code != "" {
		fmt.Fprintf(&b, " %s", e.Code)
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	switch e.Code {
	case CodeIdempotencyKeyReused:
		b.WriteString(" (this idempotency key was already used for a different request; use a new key)")
	case CodeInProgress:
		b.WriteString(" (an earlier request with this idempotency key is still running; retry later with the same key)")
	case CodeVersionMismatch:
		b.WriteString(" (read the document again and reapply the change)")
	}
	if e.hasRetryAfter {
		fmt.Fprintf(&b, " (retry after %s)", e.RetryAfter)
	}
	if e.RequestID != "" {
		fmt.Fprintf(&b, " [request %s]", e.RequestID)
	}
	return b.String()
}

// Is matches an ErrorCode target against Code. ErrPayloadTooLarge also
// matches a 413 without a code.
func (e *APIError) Is(target error) bool {
	c, ok := target.(ErrorCode)
	if !ok {
		return false
	}
	if e.Code != "" && e.Code == c {
		return true
	}
	return c == CodePayloadTooLarge && e.Status == http.StatusRequestEntityTooLarge && e.Code == ""
}

// ConnectionError means no response arrived: a DNS, TCP or TLS failure, a
// per-attempt timeout (errors.Is(err, ErrTimeout)), or a cancelled context
// (errors.Is(err, context.Canceled) or context.DeadlineExceeded).
type ConnectionError struct {
	Method string
	Path   string
	Err    error
}

func (e *ConnectionError) Error() string {
	return fmt.Sprintf("lk: %s %s: no response: %v", e.Method, e.Path, e.Err)
}

func (e *ConnectionError) Unwrap() error { return e.Err }

func statusIs(err error, match func(int) bool) bool {
	var e *APIError
	return errors.As(err, &e) && match(e.Status)
}

func status(s int) func(int) bool { return func(got int) bool { return got == s } }

// IsValidation reports whether err is a 400 API error.
func IsValidation(err error) bool { return statusIs(err, status(400)) }

// IsAuthentication reports whether err is a 401 API error.
func IsAuthentication(err error) bool { return statusIs(err, status(401)) }

// IsPermission reports whether err is a 403 API error.
func IsPermission(err error) bool { return statusIs(err, status(403)) }

// IsNotFound reports whether err is a 404 API error.
func IsNotFound(err error) bool { return statusIs(err, status(404)) }

// IsConflict reports whether err is a 409 API error.
func IsConflict(err error) bool { return statusIs(err, status(409)) }

// IsVersionMismatch reports whether err is a 412 API error.
func IsVersionMismatch(err error) bool { return statusIs(err, status(412)) }

// IsRateLimit reports whether err is a 429 API error.
func IsRateLimit(err error) bool { return statusIs(err, status(429)) }

// IsServer reports whether err is a 5xx API error.
func IsServer(err error) bool {
	return statusIs(err, func(s int) bool { return s >= 500 && s <= 599 })
}

const maxErrorText = 200

// newAPIError builds the error for a non-2xx response. It never fails: a body
// that isn't the documented JSON shape still yields an APIError.
func newAPIError(statusCode int, h http.Header, body []byte, now time.Time) *APIError {
	e := &APIError{Status: statusCode}
	var eb struct {
		Message   string       `json:"message"`
		Code      ErrorCode    `json:"code"`
		RequestID string       `json:"requestId"`
		Errors    []FieldError `json:"errors"`
	}
	if json.Unmarshal(body, &eb) == nil && (eb.Code != "" || eb.Message != "") {
		e.Code, e.Message, e.Errors, e.RequestID = eb.Code, eb.Message, eb.Errors, eb.RequestID
	} else if statusCode == http.StatusRequestEntityTooLarge {
		e.Message = "request body exceeds the 1 MB limit"
	} else {
		e.Message = http.StatusText(statusCode)
		if t := errorText(body); t != "" {
			e.Message += ": " + t
		}
	}
	if id := h.Get("X-Request-Id"); id != "" {
		e.RequestID = id
	}
	e.RetryAfter, e.hasRetryAfter = parseRetryAfter(h.Get("Retry-After"), now)
	return e
}

// errorText returns a short single-line excerpt of a non-JSON error body.
func errorText(body []byte) string {
	if !utf8.Valid(body) {
		return ""
	}
	s := strings.Join(strings.Fields(string(body)), " ")
	if len(s) > maxErrorText {
		cut := maxErrorText
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}
