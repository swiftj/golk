package lk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/swiftj/golk/internal/testutil"
)

func getErr(t *testing.T, replies ...testutil.Reply) error {
	t.Helper()
	srv := testutil.NewFakeServer(t, replies...)
	c, _ := newTestClient(t, srv, WithMaxRetries(0))
	_, err := c.Resources.Get(context.Background(), "r1")
	if err == nil {
		t.Fatal("want an error")
	}
	return err
}

func TestErrors_C_ERR_01_C_ERR_02_APIErrorFields(t *testing.T) {
	body := `{"message":"Expected string","code":"VALIDATION_ERROR","requestId":"body-id",
	"errors":[{"path":"name","message":"Too short"},{"path":"block.children[0].level","message":"Bad"}]}`
	err := getErr(t, testutil.Reply{Status: 400, Header: map[string]string{"Retry-After": "3"}, Body: body})
	var e *APIError
	if !errors.As(err, &e) {
		t.Fatalf("err = %T", err)
	}
	want := APIError{
		Status: 400, Code: CodeValidationError, Message: "Expected string", RequestID: "body-id",
		Errors:     []FieldError{{"name", "Too short"}, {"block.children[0].level", "Bad"}},
		RetryAfter: 3 * time.Second, hasRetryAfter: true,
	}
	if e.Status != want.Status || e.Code != want.Code || e.Message != want.Message || e.RequestID != want.RequestID ||
		!slices.Equal(e.Errors, want.Errors) || e.RetryAfter != want.RetryAfter {
		t.Fatalf("got %+v\nwant %+v", *e, want)
	}
}

func TestErrors_C_ERR_03_RequestIDHeaderWins(t *testing.T) {
	err := getErr(t, testutil.Reply{Status: 404, Header: map[string]string{"X-Request-Id": "header-id"},
		Body: `{"message":"m","code":"NOT_FOUND","requestId":"body-id"}`})
	var e *APIError
	errors.As(err, &e)
	if e.RequestID != "header-id" {
		t.Errorf("RequestID = %q, want header value", e.RequestID)
	}
	err = getErr(t, testutil.JSON(404, `{"message":"m","code":"NOT_FOUND","requestId":"body-id"}`))
	errors.As(err, &e)
	if e.RequestID != "body-id" {
		t.Errorf("RequestID = %q, want body fallback", e.RequestID)
	}
	if !strings.Contains(e.Error(), "body-id") {
		t.Errorf("Error() lacks request id: %s", e)
	}
}

func TestErrors_C_ERR_04_FieldErrorsOnAnyStatus(t *testing.T) {
	err := getErr(t, testutil.JSON(409, `{"message":"m","code":"DUPLICATE_ID","requestId":"r","errors":[{"path":"block.id","message":"taken"}]}`))
	var e *APIError
	errors.As(err, &e)
	if len(e.Errors) != 1 || e.Errors[0].Path != "block.id" {
		t.Fatalf("Errors = %+v", e.Errors)
	}
	err = getErr(t, testutil.APIError(404, "NOT_FOUND", "r"))
	errors.As(err, &e)
	if e.Errors != nil {
		t.Fatalf("Errors = %+v, want nil", e.Errors)
	}
}

func TestErrors_C_ERR_05_UnknownCodeKept(t *testing.T) {
	err := getErr(t, testutil.JSON(409, `{"message":"m","code":"BRAND_NEW_CODE","requestId":"r"}`))
	var e *APIError
	errors.As(err, &e)
	if e.Code != "BRAND_NEW_CODE" || e.Code.Known() {
		t.Fatalf("Code = %q known=%v", e.Code, e.Code.Known())
	}
	for _, c := range allCodes() {
		if errors.Is(err, c) {
			t.Fatalf("unknown code matched %s", c)
		}
	}
	if !CodeHasChildren.Known() || CodeHasChildren.Status() != 409 {
		t.Fatal("catalogue lookup broken")
	}
}

func TestErrors_C_ERR_06_IsAndAs(t *testing.T) {
	err := getErr(t, testutil.APIError(412, "VERSION_MISMATCH", "r"))
	if !errors.Is(err, ErrVersionMismatch) || errors.Is(err, ErrNotFound) {
		t.Fatal("errors.Is does not match on code")
	}
	wrapped := errors.Join(errors.New("context"), err)
	if !errors.Is(wrapped, ErrVersionMismatch) {
		t.Fatal("errors.Is through a wrapper")
	}
	var e *APIError
	if !errors.As(wrapped, &e) || e.Code != CodeVersionMismatch {
		t.Fatal("errors.As")
	}
	// Message wording is irrelevant to matching.
	err = getErr(t, testutil.JSON(412, `{"message":"anything at all","code":"VERSION_MISMATCH","requestId":"r"}`))
	if !errors.Is(err, ErrVersionMismatch) {
		t.Fatal("match depended on message")
	}
}

func TestErrors_C_ERR_07_StatusHelpers(t *testing.T) {
	helpers := map[int]func(error) bool{
		400: IsValidation, 401: IsAuthentication, 403: IsPermission, 404: IsNotFound,
		409: IsConflict, 412: IsVersionMismatch, 429: IsRateLimit, 500: IsServer, 502: IsServer,
	}
	for status, is := range helpers {
		err := error(&APIError{Status: status})
		if !is(err) {
			t.Errorf("helper for %d returned false", status)
		}
		if IsNotFound(err) != (status == 404) {
			t.Errorf("IsNotFound(%d) wrong", status)
		}
	}
	if IsServer(&ConnectionError{Err: errors.New("x")}) {
		t.Error("ConnectionError reported as server error")
	}
}

func TestErrors_C_ERR_08_C_ERR_15_PlainText413(t *testing.T) {
	err := getErr(t, testutil.Reply{Status: 413, Header: map[string]string{"Content-Type": "text/plain"}, Body: "Request Entity Too Large"})
	var e *APIError
	if !errors.As(err, &e) {
		t.Fatalf("err = %T %v", err, err)
	}
	if e.Status != 413 || e.Code != "" || e.RequestID != "" || !strings.Contains(e.Message, "1 MB") {
		t.Fatalf("got %+v", *e)
	}
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatal("plain-text 413 does not match ErrPayloadTooLarge")
	}
	// A JSON 413 decodes normally and also matches.
	err = getErr(t, testutil.APIError(413, "PAYLOAD_TOO_LARGE", "r"))
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatal("JSON 413 does not match")
	}
	err = getErr(t, testutil.APIError(413, "STORAGE_QUOTA_EXCEEDED", "r"))
	if errors.Is(err, ErrPayloadTooLarge) || !errors.Is(err, ErrStorageQuotaExceeded) {
		t.Fatal("STORAGE_QUOTA_EXCEEDED mismatched")
	}
}

func TestErrors_C_ERR_09_MalformedBodies(t *testing.T) {
	for _, body := range []string{"", "<html>Bad Gateway</html>", "{", `{"code":5}`, "\xff\xfe", strings.Repeat("x", 5000)} {
		e := newAPIError(418, http.Header{}, []byte(body), epoch)
		if e.Status != 418 || len(e.Message) > 400 {
			t.Errorf("body %.20q: %+v", body, e)
		}
	}
}

func TestErrors_C_ERR_10_ConnectionError(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	srv.Close() // nothing listens: connection refused
	c, _ := newTestClient(t, srv, WithMaxRetries(0))
	_, err := c.Resources.Get(context.Background(), "r1")
	var ce *ConnectionError
	if !errors.As(err, &ce) || ce.Unwrap() == nil {
		t.Fatalf("err = %v", err)
	}
	var ae *APIError
	if errors.As(err, &ae) {
		t.Fatal("ConnectionError is an APIError")
	}
	if !strings.Contains(err.Error(), "/api/v3/projects/p1/resources/r1") {
		t.Errorf("Error() lacks the path: %v", err)
	}
}

func TestErrors_C_ERR_11_RetryAfterParsed(t *testing.T) {
	for _, tc := range []struct {
		name, header string
		want         time.Duration
		ok           bool
	}{
		{"seconds", "7", 7 * time.Second, true},
		{"zero", "0", 0, true},
		{"http-date", epoch.Add(90 * time.Second).Format(http.TimeFormat), 90 * time.Second, true},
		{"past date", epoch.Add(-time.Minute).Format(http.TimeFormat), 0, true},
		{"negative", "-3", 0, false},
		{"garbage", "soon", 0, false},
		{"fraction", "1.5", 0, false},
		{"absent", "", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRetryAfter(tc.header, epoch)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("parseRetryAfter(%q) = %s, %v", tc.header, got, ok)
			}
		})
	}
	err := getErr(t, testutil.Reply{Status: 429, Header: map[string]string{"Retry-After": "30"},
		Body: `{"message":"slow down","code":"RATE_LIMITED","requestId":"r"}`})
	var e *APIError
	errors.As(err, &e)
	if e.RetryAfter != 30*time.Second || !strings.Contains(e.Error(), "retry after 30s") {
		t.Fatalf("RetryAfter = %s, Error() = %s", e.RetryAfter, e)
	}
}

func TestErrors_C_ERR_13_NoUnsetFieldsSent(t *testing.T) {
	// The server rejects unknown fields, so sending nothing extra matters.
	// The exact-body assertions live with the operations (C-WIRE-01); here,
	// the empty update is checked to send an empty object.
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv)
	if _, err := c.Resources.Update(context.Background(), "r1", ResourceUpdate{}); err != nil {
		t.Fatal(err)
	}
	assertJSON(t, srv.Requests()[0].Body, `{}`)
}

func allCodes() []ErrorCode {
	codes := make([]ErrorCode, 0, len(codeStatus))
	for c := range codeStatus {
		codes = append(codes, c)
	}
	slices.Sort(codes)
	return codes
}

func TestErrors_C_ERR_14_CatalogueMatchesSpec(t *testing.T) {
	raw, err := os.ReadFile("../docs/legendkeeper/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var spec struct {
		Components struct {
			Schemas struct {
				ErrorCode struct {
					Enum []ErrorCode `json:"enum"`
				} `json:"ErrorCode"`
			} `json:"schemas"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	want := spec.Components.Schemas.ErrorCode.Enum
	slices.Sort(want)
	if got := allCodes(); !slices.Equal(got, want) {
		t.Fatalf("catalogue drift\n got: %v\nspec: %v", got, want)
	}
	if len(want) != 29 {
		t.Errorf("spec has %d codes; the guide lists 29", len(want))
	}
}

func TestErrors_IdempotencyHints(t *testing.T) {
	for code, hint := range map[ErrorCode]string{
		CodeIdempotencyKeyReused: "use a new key",
		CodeInProgress:           "retry later with the same key",
		CodeVersionMismatch:      "read the document again",
	} {
		e := &APIError{Status: code.Status(), Code: code, Message: "m"}
		if !strings.Contains(e.Error(), hint) {
			t.Errorf("%s: Error() = %q lacks %q", code, e, hint)
		}
	}
}

func FuzzDecodeAPIError(f *testing.F) {
	f.Add(400, []byte(`{"message":"m","code":"VALIDATION_ERROR","requestId":"r","errors":[{"path":"a","message":"b"}]}`), "5", "rid")
	f.Add(413, []byte("Request Entity Too Large"), "", "")
	f.Add(429, []byte(`{"code":null}`), "Wed, 21 Oct 2026 07:28:00 GMT", "")
	f.Add(500, []byte("\x00\xff{"), "-1", "x")
	f.Fuzz(func(t *testing.T, status int, body []byte, retryAfter, requestID string) {
		h := http.Header{}
		h.Set("Retry-After", retryAfter)
		h.Set("X-Request-Id", requestID)
		e := newAPIError(status, h, body, epoch)
		if e.Status != status {
			t.Fatalf("status %d became %d", status, e.Status)
		}
		if e.RetryAfter < 0 {
			t.Fatalf("negative RetryAfter %s", e.RetryAfter)
		}
		_ = e.Error()
		_ = errors.Is(e, ErrPayloadTooLarge)
	})
}
