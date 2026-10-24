//go:build integration

// Package sandbox gives integration tests a client for the sandbox project
// only. It refuses to run against any other project.
//
// It reads LK_SANDBOX_API_KEY (and optionally LK_SANDBOX_BASE_URL), never
// LEGENDKEEPER_API_KEY, which may belong to a real, player-facing wiki. Before
// any test runs, it reads the key's project and aborts unless its name equals
// LK_SANDBOX_PROJECT_NAME exactly.
package sandbox

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/swiftj/golk/lk"
)

const (
	envKey     = "LK_SANDBOX_API_KEY"
	envName    = "LK_SANDBOX_PROJECT_NAME"
	envBaseURL = "LK_SANDBOX_BASE_URL"
)

var (
	// limiter is shared by every sandbox client in the process, so the tests
	// together stay under the key's limits.
	limiter = lk.NewRateLimiter(lk.DefaultRateLimits)

	once      sync.Once
	client    *lk.Client
	projectID string
	setupErr  error

	// RunPrefix is in the name of everything a test run creates, so Sweep
	// can find leftovers. It is unique per process.
	RunPrefix = "golk-it-" + randomHex(4)
)

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// setup builds the client and checks the project name, once per process.
func setup() {
	key, want := os.Getenv(envKey), os.Getenv(envName)
	if key == "" || want == "" {
		setupErr = fmt.Errorf("sandbox: %s and %s must both be set", envKey, envName)
		return
	}
	opts := []lk.Option{lk.WithRateLimiter(limiter), lk.WithUserAgent("golk-integration")}
	if u := os.Getenv(envBaseURL); u != "" {
		opts = append(opts, lk.WithBaseURL(u))
	}
	c, err := lk.New(key, opts...)
	if err != nil {
		setupErr = err
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	p, err := c.Project(ctx)
	if err != nil {
		setupErr = fmt.Errorf("sandbox: reading the key's project: %w", err)
		return
	}
	if p.Name != want {
		setupErr = fmt.Errorf("sandbox: ABORT — the key's project is named %q, not %q; refusing to touch a non-sandbox project", p.Name, want)
		return
	}
	client, projectID = c, p.ID
}

// Guard runs the project check. TestMain calls it before m.Run and exits on
// error, so no test can run against the wrong project.
func Guard() error {
	once.Do(setup)
	return setupErr
}

// Client returns the guarded sandbox client.
func Client(t testing.TB) *lk.Client {
	t.Helper()
	if err := Guard(); err != nil {
		t.Fatal(err)
	}
	return client
}

// ProjectID returns the sandbox project's ID.
func ProjectID(t testing.TB) string {
	t.Helper()
	Client(t)
	return projectID
}

// Name prefixes name with RunPrefix.
func Name(name string) string { return RunPrefix + " " + name }

// Ctx returns a context that ends with the test, bounded so a hung server
// cannot stall the suite.
func Ctx(t testing.TB) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

// Cleanup deletes resource id (with its descendants) when the test ends. A
// resource the test already deleted is fine.
func Cleanup(t testing.TB, id string) {
	t.Helper()
	c := Client(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		err := c.Resources.Delete(ctx, id, lk.ResourceDeleteParams{Cascade: true})
		if err != nil && !errors.Is(err, lk.ErrNotFound) && !errors.Is(err, lk.ErrAlreadyDeleted) {
			t.Errorf("cleanup: deleting %s: %v", id, err)
		}
	})
}

// CleanupAsset deletes asset id (a folder with its contents) when the test
// ends. An asset the test already deleted is fine.
func CleanupAsset(t testing.TB, id string) {
	t.Helper()
	c := Client(t)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if err := c.Assets.Delete(ctx, id); err != nil && !errors.Is(err, lk.ErrNotFound) {
			t.Errorf("cleanup: deleting asset %s: %v", id, err)
		}
	})
}

// Sweep deletes every live resource and every root asset whose name carries
// RunPrefix and reports how many it found. A non-zero count means a test
// leaked something.
func Sweep() (int, error) {
	if err := Guard(); err != nil {
		return 0, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var leaked []lk.ResourceListItem
	for r, err := range client.Resources.All(ctx, lk.ResourceListParams{Limit: 100}) {
		if err != nil {
			return 0, err
		}
		if strings.HasPrefix(r.Name, RunPrefix) {
			leaked = append(leaked, r)
		}
	}
	for _, r := range leaked {
		err := client.Resources.Delete(ctx, r.ID, lk.ResourceDeleteParams{Cascade: true})
		if err != nil && !errors.Is(err, lk.ErrNotFound) && !errors.Is(err, lk.ErrAlreadyDeleted) {
			return len(leaked), err
		}
	}
	var assets []lk.Asset
	for a, err := range client.Assets.All(ctx, lk.AssetListParams{Limit: 100}) {
		if err != nil {
			return len(leaked), err
		}
		if strings.HasPrefix(a.Name, RunPrefix) {
			assets = append(assets, a)
		}
	}
	for _, a := range assets {
		if err := client.Assets.Delete(ctx, a.ID); err != nil && !errors.Is(err, lk.ErrNotFound) {
			return len(leaked) + len(assets), err
		}
	}
	return len(leaked) + len(assets), nil
}

// Main is TestMain for integration packages: guard, run, then sweep.
func Main(m interface{ Run() int }) int {
	if err := Guard(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	code := m.Run()
	n, err := Sweep()
	switch {
	case err != nil:
		fmt.Fprintln(os.Stderr, "sandbox sweep:", err)
		return 1
	case n > 0:
		fmt.Fprintf(os.Stderr, "sandbox sweep: %d resource(s) or asset(s) named %q were left behind and have been deleted\n", n, RunPrefix)
		return 1
	}
	return code
}

// APIKey returns the guarded sandbox key, for code under test that builds
// its own client (lk-mcp). Never log it.
func APIKey(t testing.TB) string {
	t.Helper()
	Client(t)
	return os.Getenv(envKey)
}

// ClientOptions are the options that point another lk client at the
// sandbox project: its base URL, its project ID and the shared rate limiter.
func ClientOptions(t testing.TB) []lk.Option {
	t.Helper()
	opts := []lk.Option{lk.WithRateLimiter(limiter), lk.WithProjectID(ProjectID(t))}
	if u := os.Getenv(envBaseURL); u != "" {
		opts = append(opts, lk.WithBaseURL(u))
	}
	return opts
}
