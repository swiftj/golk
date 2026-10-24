// Package lk is a Go client for the LegendKeeper v3 REST API.
//
// It follows the behavior of the official TypeScript SDK (@legendkeeper/sdk):
// retries with backoff, an Idempotency-Key on every POST, versioned writes,
// pagination and lazy project resolution. See docs/SDK_CONTRACT.md.
//
//	c, err := lk.NewFromEnv()
//	if err != nil { ... }
//	project, err := c.Project(ctx)
package lk

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Version is this SDK's version, sent in the User-Agent header.
const Version = "0.1.0-dev"

// Client calls the LegendKeeper API. It is safe for concurrent use.
// Constructing one makes no request.
type Client struct {
	apiKey     secret
	baseURL    *url.URL
	hc         *http.Client
	timeout    time.Duration
	maxRetries int
	genKeys    bool
	limiter    *RateLimiter
	logger     *slog.Logger
	userAgent  string
	clock      clock
	rand       randFloat

	projMu    sync.Mutex
	projectID string
	projCall  *resolveCall

	// Projects lists and reads projects.
	Projects *ProjectsService
	// Resources manages entries in the project tree.
	Resources *ResourcesService
	// Documents manages a resource's documents and page content.
	Documents *DocumentsService
	// Blocks reads and changes single blocks, key-value rows and infoboxes.
	Blocks *BlocksService
	// Properties manages a resource's sidebar properties.
	Properties *PropertiesService
	// Assets manages the asset library and image uploads.
	Assets *AssetsService
	// Search searches the project.
	Search *SearchService
	// Tags lists the project's tags.
	Tags *TagsService
	// Features are a map document's pins, labels, regions, paths and folders.
	Features *FeaturesService
	// Timelines are a timeline document's lanes and events.
	Timelines *TimelinesService
	// Calendars are the project's custom and built-in calendars.
	Calendars *CalendarsService
	// Templates are the project's resource templates.
	Templates *TemplatesService
}

// New returns a client that authenticates with apiKey. It never reads the
// environment; see NewFromEnv.
func New(apiKey string, opts ...Option) (*Client, error) {
	cfg := config{
		baseURL:    DefaultBaseURL,
		timeout:    DefaultTimeout,
		maxRetries: DefaultMaxRetries,
		clock:      realClock{},
		rand:       defaultRand,
	}
	for _, o := range opts {
		o(&cfg)
	}
	if err := errors.Join(cfg.errs...); err != nil {
		return nil, err
	}
	if apiKey == "" {
		return nil, ErrMissingAPIKey
	}
	u, err := url.Parse(strings.TrimRight(cfg.baseURL, "/"))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("lk: invalid base URL %q: want http(s)://host", cfg.baseURL)
	}
	if !cfg.limiterSet {
		cfg.limiter = NewRateLimiter(DefaultRateLimits)
	}
	if cfg.httpClient == nil {
		cfg.httpClient = &http.Client{}
	}
	ua := "golk/" + Version
	if cfg.userAgent != "" {
		ua = cfg.userAgent + " " + ua
	}
	c := &Client{
		apiKey:     secret(apiKey),
		baseURL:    u,
		hc:         cfg.httpClient,
		timeout:    cfg.timeout,
		maxRetries: cfg.maxRetries,
		genKeys:    !cfg.noGenKeys,
		limiter:    cfg.limiter,
		logger:     cfg.logger,
		userAgent:  ua,
		clock:      cfg.clock,
		rand:       cfg.rand,
		projectID:  cfg.projectID,
	}
	c.Projects = &ProjectsService{c: c}
	c.Resources = &ResourcesService{c: c}
	c.Documents = &DocumentsService{c: c}
	c.Blocks = &BlocksService{c: c}
	c.Properties = &PropertiesService{c: c}
	c.Assets = &AssetsService{c: c}
	c.Search = &SearchService{c: c}
	c.Tags = &TagsService{c: c}
	c.Features = &FeaturesService{c: c}
	c.Timelines = &TimelinesService{c: c}
	c.Calendars = &CalendarsService{c: c}
	c.Templates = &TemplatesService{c: c}
	return c, nil
}

// Environment variables read by NewFromEnv.
const (
	EnvAPIKey  = "LEGENDKEEPER_API_KEY"
	EnvBaseURL = "LEGENDKEEPER_BASE_URL"
)

// NewFromEnv returns a client using LEGENDKEEPER_API_KEY and, when it is set
// and non-empty, LEGENDKEEPER_BASE_URL. Options override the environment.
func NewFromEnv(opts ...Option) (*Client, error) {
	var pre []Option
	if u := os.Getenv(EnvBaseURL); u != "" {
		pre = append(pre, WithBaseURL(u))
	}
	key := os.Getenv(EnvAPIKey)
	if key == "" {
		return nil, fmt.Errorf("%w: set %s", ErrMissingAPIKey, EnvAPIKey)
	}
	return New(key, append(pre, opts...)...)
}

// String describes the client without its API key.
func (c *Client) String() string { return "lk.Client{" + c.baseURL.String() + "}" }

// GoString describes the client without its API key.
func (c *Client) GoString() string { return c.String() }

// secret hides the API key from fmt and slog.
type secret string

func (secret) String() string               { return "[REDACTED]" }
func (secret) GoString() string             { return `"[REDACTED]"` }
func (secret) LogValue() slog.Value         { return slog.StringValue("[REDACTED]") }
func (s secret) Format(f fmt.State, _ rune) { _, _ = f.Write([]byte(s.String())) }
