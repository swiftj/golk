package lk

import (
	"errors"
	"log/slog"
	"net/http"
	"time"
)

// Defaults from the SDK guide's Client options.
const (
	DefaultBaseURL    = "https://www.legendkeeper.com"
	DefaultTimeout    = 30 * time.Second
	DefaultMaxRetries = 2
)

// Option configures a Client.
type Option func(*config)

type config struct {
	baseURL    string
	httpClient *http.Client
	projectID  string
	timeout    time.Duration
	maxRetries int
	noGenKeys  bool
	limiter    *RateLimiter
	limiterSet bool
	logger     *slog.Logger
	userAgent  string
	clock      clock
	rand       randFloat
	errs       []error
}

// WithBaseURL sets the server, such as a proxy or a test server. The default
// is https://www.legendkeeper.com.
func WithBaseURL(u string) Option { return func(c *config) { c.baseURL = u } }

// WithHTTPClient sets the HTTP client. Its Transport sees every attempt,
// including retries, so it is the place for tracing or proxying. The client's
// own Timeout should be left zero; the per-attempt timeout is WithTimeout.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *config) {
		if hc == nil {
			c.errs = append(c.errs, errors.New("lk: WithHTTPClient: nil client"))
			return
		}
		c.httpClient = hc
	}
}

// WithProjectID sets the project, so the client never resolves it.
func WithProjectID(id string) Option { return func(c *config) { c.projectID = id } }

// WithTimeout sets the time limit for each attempt. Each retry gets a fresh
// limit. Zero or negative disables the per-attempt limit; the context still
// applies. The default is 30s.
func WithTimeout(d time.Duration) Option { return func(c *config) { c.timeout = d } }

// WithMaxRetries sets how many times a failed attempt is retried. 0 turns
// retries off. The default is 2.
func WithMaxRetries(n int) Option {
	return func(c *config) {
		if n < 0 {
			c.errs = append(c.errs, errors.New("lk: WithMaxRetries: negative count"))
			return
		}
		c.maxRetries = n
	}
}

// WithoutIdempotencyKeys stops the client generating an Idempotency-Key for
// every POST. A key passed with WithIdempotencyKey is still sent. Without a
// key, a POST is retried only on 429 and on a connection failure.
func WithoutIdempotencyKeys() Option { return func(c *config) { c.noGenKeys = true } }

// WithRateLimits replaces DefaultRateLimits for this client.
func WithRateLimits(l RateLimits) Option {
	return func(c *config) { c.limiter, c.limiterSet = NewRateLimiter(l), true }
}

// WithRateLimiter shares l with other clients. A nil l disables client-side
// pacing; the server's limits and 429 retries still apply.
func WithRateLimiter(l *RateLimiter) Option {
	return func(c *config) { c.limiter, c.limiterSet = l, true }
}

// WithLogger logs each attempt and retry at debug level: method, path,
// status, request ID and timing. It never logs the API key or bodies.
func WithLogger(l *slog.Logger) Option { return func(c *config) { c.logger = l } }

// WithUserAgent prepends product to the User-Agent header.
func WithUserAgent(product string) Option { return func(c *config) { c.userAgent = product } }

// withClock and withRand make timing deterministic in tests.
func withClock(clk clock) Option       { return func(c *config) { c.clock = clk } }
func withRand(r func() float64) Option { return func(c *config) { c.rand = r } }

// CallOption configures one call.
type CallOption func(*callConfig)

type callConfig struct {
	projectID  string
	idemKey    string
	idemKeySet bool
	ifMatch    string
	ifMatchSet bool
}

// ForProject runs the call against project id instead of the client's project.
// It does not change the client's cached project.
func ForProject(id string) CallOption { return func(c *callConfig) { c.projectID = id } }

// WithIdempotencyKey sends key as the call's Idempotency-Key instead of a
// generated one. It is reused on every retry. A key derived from your own
// data, such as "import-row-42", makes a script safe to rerun within the
// server's 24-hour window. Only POST operations accept it.
func WithIdempotencyKey(key string) CallOption {
	return func(c *callConfig) { c.idemKey, c.idemKeySet = key, true }
}

// IfMatch makes a versioned write apply only if the document is still at
// version; otherwise the call fails with ErrVersionMismatch. Only versioned
// operations accept it.
func IfMatch(version string) CallOption {
	return func(c *callConfig) { c.ifMatch, c.ifMatchSet = version, true }
}
