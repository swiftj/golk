package lk

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/swiftj/golk/internal/testutil"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestIdem_C_IDEM_01_C_IDEM_02_GeneratedPerCallReusedOnRetry(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.APIError(503, "INTERNAL", "a"), testutil.APIError(409, "IN_PROGRESS", "b"), testutil.JSON(201, okResource),
		testutil.JSON(201, okResource))
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	if _, err := c.Resources.Create(ctx, ResourceCreate{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resources.Create(ctx, ResourceCreate{Name: "a"}); err != nil {
		t.Fatal(err)
	}
	r := srv.Requests()
	k := r[0].Header.Get("Idempotency-Key")
	if !uuidV4.MatchString(k) {
		t.Fatalf("key %q is not a UUIDv4", k)
	}
	if r[1].Header.Get("Idempotency-Key") != k || r[2].Header.Get("Idempotency-Key") != k {
		t.Fatal("key changed across retries")
	}
	if r[3].Header.Get("Idempotency-Key") == k {
		t.Fatal("two calls shared a key")
	}
}

func TestIdem_C_IDEM_01_EveryPostOperation(t *testing.T) {
	srv := testutil.NewFakeServer(t,
		testutil.JSON(201, okResource), testutil.JSON(201, `{"resources":[]}`),
		testutil.JSON(200, okResource), testutil.JSON(200, okResource))
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	_, _ = c.Resources.Create(ctx, ResourceCreate{Name: "a"})
	_, _ = c.Resources.CreateMany(ctx, []ResourceCreate{{Name: "a"}})
	_, _ = c.Resources.Move(ctx, "r1", ResourceMove{ParentID: Null[string]()})
	_, _ = c.Resources.Restore(ctx, "r1")
	for _, r := range srv.Requests() {
		if r.Method != "POST" || !uuidV4.MatchString(r.Header.Get("Idempotency-Key")) {
			t.Errorf("%s %s: key %q", r.Method, r.Path, r.Header.Get("Idempotency-Key"))
		}
	}
}

func TestIdem_C_IDEM_03_NoKeyOnOtherMethods(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(200, okResource), testutil.JSON(200, okResource), testutil.Reply{Status: 204},
		testutil.JSON(200, `{"data":[]}`))
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	_, _ = c.Resources.Get(ctx, "r1")
	_, _ = c.Resources.Update(ctx, "r1", ResourceUpdate{Name: Some("x")})
	_ = c.Resources.Delete(ctx, "r1", ResourceDeleteParams{})
	_, _ = c.Resources.List(ctx, ResourceListParams{})
	for _, r := range srv.Requests() {
		if v := r.Header.Get("Idempotency-Key"); v != "" {
			t.Errorf("%s sent Idempotency-Key %q", r.Method, v)
		}
	}
}

func TestIdem_C_IDEM_04_CallerKey(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(502, "INTERNAL", "a"), testutil.JSON(201, okResource))
	c, _ := newTestClient(t, srv)
	if _, err := c.Resources.Create(context.Background(), ResourceCreate{Name: "Gunter"}, WithIdempotencyKey("import-gunter-42")); err != nil {
		t.Fatal(err)
	}
	for _, r := range srv.Requests() {
		if k := r.Header.Get("Idempotency-Key"); k != "import-gunter-42" {
			t.Errorf("key = %q", k)
		}
	}
}

func TestIdem_C_IDEM_05_DisabledKeysAndCallerKey(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.JSON(201, okResource), testutil.JSON(201, okResource))
	c, _ := newTestClient(t, srv, WithoutIdempotencyKeys())
	ctx := context.Background()
	_, _ = c.Resources.Create(ctx, ResourceCreate{Name: "a"})
	_, _ = c.Resources.Create(ctx, ResourceCreate{Name: "a"}, WithIdempotencyKey("mine"))
	r := srv.Requests()
	if r[0].Header.Get("Idempotency-Key") != "" {
		t.Error("generated key sent while disabled")
	}
	if r[1].Header.Get("Idempotency-Key") != "mine" {
		t.Error("caller key dropped while generated keys are disabled")
	}
}

func TestIdem_C_IDEM_06_InvalidKeyRejectedLocally(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	for _, k := range []string{"", "has space", "slash/", "ünïcode", strings.Repeat("a", 256)} {
		_, err := c.Resources.Create(context.Background(), ResourceCreate{Name: "a"}, WithIdempotencyKey(k))
		if !errors.Is(err, ErrInvalidIdempotencyKey) {
			t.Errorf("key %q: err = %v", k, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("%d requests sent with invalid keys", n)
	}
	for _, k := range []string{"a", "A-z.0_9", strings.Repeat("k", 255)} {
		if !validIdempotencyKey(k) {
			t.Errorf("valid key %q rejected", k)
		}
	}
}

func TestIdem_C_IDEM_07_KeyReusedSurfaced(t *testing.T) {
	srv := testutil.NewFakeServer(t, testutil.APIError(409, "IDEMPOTENCY_KEY_REUSED", "r1"))
	c, _ := newTestClient(t, srv)
	_, err := c.Resources.Create(context.Background(), ResourceCreate{Name: "Gunther, Destroyer of Worlds"}, WithIdempotencyKey("import-gunter"))
	if !errors.Is(err, ErrIdempotencyKeyReused) || !IsConflict(err) {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "use a new key") {
		t.Errorf("Error() has no hint: %v", err)
	}
	if n := len(srv.Requests()); n != 1 {
		t.Fatalf("IDEMPOTENCY_KEY_REUSED retried: %d attempts", n)
	}
}

func TestIdem_C_IDEM_08_GeneratedKeysValid(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		k := newUUID()
		if !validIdempotencyKey(k) || !uuidV4.MatchString(k) || seen[k] {
			t.Fatalf("bad or repeated key %q", k)
		}
		seen[k] = true
	}
}

func TestIdem_C_IDEM_09_KeyOnlyForPost(t *testing.T) {
	srv := testutil.NewFakeServer(t)
	c, _ := newTestClient(t, srv)
	ctx := context.Background()
	for name, call := range map[string]func() error{
		"get": func() error { _, err := c.Resources.Get(ctx, "r1", WithIdempotencyKey("k")); return err },
		"update": func() error {
			_, err := c.Resources.Update(ctx, "r1", ResourceUpdate{}, WithIdempotencyKey("k"))
			return err
		},
		"delete": func() error { return c.Resources.Delete(ctx, "r1", ResourceDeleteParams{}, WithIdempotencyKey("k")) },
	} {
		if err := call(); !errors.Is(err, ErrIdempotencyKeyNotAllowed) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
	if n := len(srv.Requests()); n != 0 {
		t.Fatalf("%d requests sent", n)
	}
}
