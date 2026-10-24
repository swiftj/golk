package lk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
)

// request is one API call before it is sent.
type request struct {
	method string
	// path is the expanded path, starting with "/".
	path  string
	query url.Values
	// body is encoded as JSON; nil sends no body. json.RawMessage is sent as is.
	body any
	// out receives the decoded 2xx body; nil discards it.
	out any
	// genKey lets a POST get a generated Idempotency-Key. Wrapped operations
	// set it; the escape hatch does not.
	genKey bool
	// versioned operations accept IfMatch and fill a missing version from ETag.
	versioned bool
	// header holds extra headers from the escape hatch.
	header http.Header
}

// response is a 2xx response with its body read.
type response struct {
	status int
	header http.Header
	body   []byte
}

// versionSetter is implemented by results of versioned operations, so a
// version missing from the body can be filled from the ETag header.
type versionSetter interface {
	setVersionIfEmpty(v string)
}

// do sends req with rate limiting, per-attempt timeouts and retries, and
// decodes the result into req.out.
func (c *Client) do(ctx context.Context, req *request, cc callConfig) error {
	if ctx == nil {
		return errors.New("lk: nil context")
	}
	if cc.ifMatchSet && !req.versioned {
		return ErrIfMatchNotAllowed
	}
	key, err := c.idempotencyKey(req, cc)
	if err != nil {
		return err
	}
	body, err := encodeBody(req.body)
	if err != nil {
		return err
	}
	target := c.baseURL.String() + req.path
	if len(req.query) > 0 {
		target += "?" + req.query.Encode()
	}

	for n := 0; ; n++ {
		if err := c.limiter.wait(ctx, c.clock, req.method); err != nil {
			return &ConnectionError{Method: req.method, Path: req.path, Err: err}
		}
		resp, failed, err := c.attempt(ctx, req, target, body, key, cc, n)
		if err != nil {
			return err
		}
		if failed == nil {
			cc.capture(resp.status, resp.header)
			return decode(req, resp)
		}
		final := failed.err(req)
		if failed.apiErr != nil {
			cc.capture(failed.apiErr.Status, failed.header)
		}
		if n >= c.maxRetries || !retryable(req.method, key != "", *failed) {
			return final
		}
		wait, ok := retryWait(n, *failed, c.rand)
		if !ok {
			return final
		}
		c.log(ctx, "lk retry", slog.String("method", req.method), slog.String("path", req.path),
			slog.Int("retry", n+1), slog.Duration("wait", wait), slog.String("reason", final.Error()))
		select {
		case <-c.clock.After(wait):
		case <-ctx.Done():
			return &ConnectionError{Method: req.method, Path: req.path, Err: context.Cause(ctx)}
		}
	}
}

func (a attempt) err(req *request) error {
	if a.apiErr != nil {
		return a.apiErr
	}
	return &ConnectionError{Method: req.method, Path: req.path, Err: a.connErr}
}

// attempt sends one HTTP request. It returns the response on 2xx, the failure
// on a non-2xx status or a missing response, or a terminal error when the
// caller's context is done.
func (c *Client) attempt(ctx context.Context, req *request, target string, body []byte, key string, cc callConfig, n int) (*response, *attempt, error) {
	actx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	if c.timeout > 0 {
		stop := c.clock.AfterFunc(c.timeout, func() { cancel(ErrTimeout) })
		defer stop()
	}

	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	hreq, err := http.NewRequestWithContext(actx, req.method, target, rd)
	if err != nil {
		return nil, nil, fmt.Errorf("lk: building request: %w", err)
	}
	for k, vs := range req.header {
		hreq.Header[k] = append([]string(nil), vs...)
	}
	hreq.Header.Set("Authorization", "Bearer "+string(c.apiKey))
	hreq.Header.Set("User-Agent", c.userAgent)
	hreq.Header.Set("Accept", "application/json")
	if body != nil {
		hreq.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		hreq.Header.Set("Idempotency-Key", key)
	}
	if cc.ifMatchSet {
		hreq.Header.Set("If-Match", cc.ifMatch)
	}

	start := c.clock.Now()
	hresp, err := c.hc.Do(hreq)
	var data []byte
	if err == nil {
		data, err = io.ReadAll(hresp.Body)
		_ = hresp.Body.Close()
	}
	if err != nil {
		if ctx.Err() != nil {
			return nil, nil, &ConnectionError{Method: req.method, Path: req.path, Err: context.Cause(ctx)}
		}
		timedOut := errors.Is(context.Cause(actx), ErrTimeout)
		if timedOut {
			err = fmt.Errorf("%w after %s", ErrTimeout, c.timeout)
		}
		c.log(ctx, "lk attempt", slog.String("method", req.method), slog.String("path", req.path),
			slog.Int("attempt", n+1), slog.String("error", err.Error()))
		return nil, &attempt{connErr: err, timedOut: timedOut}, nil
	}

	c.log(ctx, "lk attempt", slog.String("method", req.method), slog.String("path", req.path),
		slog.Int("attempt", n+1), slog.Int("status", hresp.StatusCode),
		slog.String("request_id", hresp.Header.Get("X-Request-Id")),
		slog.Duration("elapsed", c.clock.Now().Sub(start)))
	if hresp.StatusCode >= 200 && hresp.StatusCode <= 299 {
		return &response{status: hresp.StatusCode, header: hresp.Header, body: data}, nil, nil
	}
	return nil, &attempt{apiErr: newAPIError(hresp.StatusCode, hresp.Header, data, c.clock.Now()), header: hresp.Header}, nil
}

func (c *Client) log(ctx context.Context, msg string, attrs ...slog.Attr) {
	if c.logger != nil {
		c.logger.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
	}
}

// idempotencyKey returns the key for req: the caller's, one from the escape
// hatch's headers, or a generated one for a wrapped POST.
func (c *Client) idempotencyKey(req *request, cc callConfig) (string, error) {
	if cc.idemKeySet {
		if req.method != http.MethodPost {
			return "", ErrIdempotencyKeyNotAllowed
		}
		if !validIdempotencyKey(cc.idemKey) {
			return "", ErrInvalidIdempotencyKey
		}
		return cc.idemKey, nil
	}
	// An escape-hatch header is passed through as written; it counts as the
	// call's key (for retries) only on a POST, where the server honors it.
	if k := req.header.Get("Idempotency-Key"); k != "" {
		if req.method != http.MethodPost {
			return "", nil
		}
		if !validIdempotencyKey(k) {
			return "", ErrInvalidIdempotencyKey
		}
		return k, nil
	}
	if req.method == http.MethodPost && req.genKey && c.genKeys {
		return newUUID(), nil
	}
	return "", nil
}

func encodeBody(v any) ([]byte, error) {
	switch b := v.(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		return b, nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("lk: encoding request body: %w", err)
	}
	return data, nil
}

func decode(req *request, resp *response) error {
	if req.out == nil {
		return nil
	}
	if len(bytes.TrimSpace(resp.body)) > 0 {
		if raw, ok := req.out.(*json.RawMessage); ok {
			*raw = append(json.RawMessage(nil), resp.body...)
		} else if err := json.Unmarshal(resp.body, req.out); err != nil {
			return fmt.Errorf("lk: decoding %s %s response: %w", req.method, req.path, err)
		}
	}
	if vs, ok := req.out.(versionSetter); ok && req.versioned {
		if v := etagVersion(resp.header.Get("ETag")); v != "" {
			vs.setVersionIfEmpty(v)
		}
	}
	return nil
}

// etagVersion turns an ETag into a version: a weak prefix (W/) and the
// surrounding quotes are removed, so the result equals the body's version
// (DECISIONS.md D-009, A-06).
func etagVersion(etag string) string {
	etag = strings.TrimPrefix(strings.TrimSpace(etag), "W/")
	if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
		return etag[1 : len(etag)-1]
	}
	return etag
}

// expandPath fills {name} placeholders in an operation path with escaped
// values. An empty or missing value is an error, so a blank ID can never
// turn a call into one on a different endpoint.
func expandPath(tmpl string, vals map[string]string) (string, error) {
	var b strings.Builder
	rest := tmpl
	for {
		i := strings.IndexByte(rest, '{')
		if i < 0 {
			b.WriteString(rest)
			return b.String(), nil
		}
		j := strings.IndexByte(rest[i:], '}')
		if j < 0 {
			return "", fmt.Errorf("lk: malformed path template %q", tmpl)
		}
		name := rest[i+1 : i+j]
		v := vals[name]
		if v == "" {
			return "", fmt.Errorf("lk: %s is required", name)
		}
		b.WriteString(rest[:i])
		b.WriteString(url.PathEscape(v))
		rest = rest[i+j+1:]
	}
}

// Request is a call for the escape hatch, Client.Do.
type Request struct {
	Method string
	// Path is sent as written, such as "/api/v3/projects/p1/calendars". The
	// project ID is not filled in.
	Path  string
	Query url.Values
	// Body is sent as JSON when non-nil.
	Body any
	// Header is added to the request. An Idempotency-Key here makes a POST
	// retry like a keyed POST; none is generated.
	Header http.Header
}

// Do sends a request the SDK does not wrap yet, with the client's key, base
// URL, timeout, retries and error mapping, and decodes a 2xx JSON body into
// out (which may be nil, or a *json.RawMessage).
func (c *Client) Do(ctx context.Context, r Request, out any, opts ...CallOption) error {
	if r.Method == "" || !strings.HasPrefix(r.Path, "/") {
		return errors.New("lk: Request needs a Method and a Path starting with /")
	}
	return c.do(ctx, &request{
		method: strings.ToUpper(r.Method),
		path:   r.Path,
		query:  r.Query,
		body:   r.Body,
		out:    out,
		header: r.Header,
		// The SDK can't know whether an unwrapped endpoint is versioned, so
		// the escape hatch accepts IfMatch and leaves the check to the server.
		versioned: true,
	}, newCallConfig(opts))
}

func newCallConfig(opts []CallOption) callConfig {
	var cc callConfig
	for _, o := range opts {
		o(&cc)
	}
	return cc
}

// capture records the final response for CaptureResponse. Each attempt
// overwrites it, so it ends up describing the last one.
func (cc callConfig) capture(status int, h http.Header) {
	if cc.meta == nil {
		return
	}
	*cc.meta = ResponseMeta{Status: status, RequestID: h.Get("X-Request-Id"), ETag: h.Get("ETag"), Header: h.Clone()}
}

// call is the common path for wrapped operations: it applies call options,
// resolves the project when the path needs it, expands the path and sends.
func (c *Client) call(ctx context.Context, method, tmpl string, params map[string]string, req request, opts []CallOption) error {
	cc := newCallConfig(opts)
	// A path that names its own project (Projects.Get) is never filled in, so
	// an empty ID fails in expandPath instead of reading the default project.
	if _, own := params["projectId"]; !own && strings.Contains(tmpl, "{projectId}") {
		pid := cc.projectID
		if pid == "" {
			var err error
			if pid, err = c.resolveProjectID(ctx); err != nil {
				return err
			}
		}
		if params == nil {
			params = map[string]string{}
		}
		params["projectId"] = pid
	}
	path, err := expandPath(tmpl, params)
	if err != nil {
		return err
	}
	req.method, req.path, req.genKey = method, path, true
	return c.do(ctx, &req, cc)
}
