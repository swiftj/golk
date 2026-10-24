//go:build integration

package lk_test

import (
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

func TestMain(m *testing.M) { os.Exit(sandbox.Main(m)) }

func create(t *testing.T, c *lk.Client, r lk.ResourceCreate) *lk.Resource {
	t.Helper()
	r.Name = sandbox.Name(r.Name)
	res, err := c.Resources.Create(sandbox.Ctx(t), r)
	if err != nil {
		t.Fatalf("create %q: %v", r.Name, err)
	}
	sandbox.Cleanup(t, res.ID)
	return res
}

func TestIntegration_Projects(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	p, err := c.Project(ctx)
	if err != nil || p.ID != sandbox.ProjectID(t) || p.CreatedAt.IsZero() {
		t.Fatalf("Project = %+v, %v", p, err)
	}
	var ids []string
	for pr, err := range c.Projects.All(ctx, lk.ProjectListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, pr.ID)
	}
	if !slices.Equal(ids, []string{p.ID}) {
		t.Fatalf("projects.list with an API key = %v, want only %s", ids, p.ID)
	}
}

func TestIntegration_ResourceLifecycle(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)

	ooo := create(t, c, lk.ResourceCreate{Name: "The Land of Ooo", IconGlyph: lk.Some("globe"), IconColor: lk.Some("#4caf50")})
	if len(ooo.Documents) != 1 || ooo.Documents[0].Type != lk.DocumentBlank || ooo.Documents[0].Name != "Main" {
		t.Errorf("new resource documents = %+v, want one blank Main", ooo.Documents)
	}
	ice := create(t, c, lk.ResourceCreate{
		Name: "Ice Kingdom", ParentID: lk.Some(ooo.ID), Tags: lk.Some([]string{"Kingdom", "Cold"}),
		Aliases: lk.Some([]string{"Land of Ice"}),
	})
	if ice.ParentID != ooo.ID || !slices.Equal(ice.Tags, []string{"Kingdom", "Cold"}) {
		t.Errorf("created %+v", ice)
	}

	// Update changes only what is sent: aliases survive a name+tags update.
	renamed, err := c.Resources.Update(ctx, ice.ID, lk.ResourceUpdate{Name: lk.Some(sandbox.Name("The Ice Kingdom")), Tags: lk.Some([]string{"Kingdom"})})
	if err != nil {
		t.Fatal(err)
	}
	if renamed.Name != sandbox.Name("The Ice Kingdom") || !slices.Equal(renamed.Tags, []string{"Kingdom"}) || !slices.Equal(renamed.Aliases, []string{"Land of Ice"}) {
		t.Errorf("updated %+v", renamed)
	}

	got, err := c.Resources.Get(ctx, ice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ancestors) == 0 || got.Ancestors[len(got.Ancestors)-1].ID != ooo.ID {
		t.Errorf("ancestors = %+v", got.Ancestors)
	}

	// Move to the root, then back.
	moved, err := c.Resources.Move(ctx, ice.ID, lk.ResourceMove{ParentID: lk.Null[string]()})
	if err != nil || moved.ParentID != "" {
		t.Fatalf("move to root: %+v %v", moved, err)
	}
	if moved, err = c.Resources.Move(ctx, ice.ID, lk.ResourceMove{ParentID: lk.Value(ooo.ID)}); err != nil || moved.ParentID != ooo.ID {
		t.Fatalf("move back: %+v %v", moved, err)
	}

	// Delete without cascade refuses; with cascade succeeds; restore brings both back.
	if err := c.Resources.Delete(ctx, ooo.ID, lk.ResourceDeleteParams{}); !errors.Is(err, lk.ErrHasChildren) {
		t.Fatalf("delete with children: %v", err)
	}
	if err := c.Resources.Delete(ctx, ooo.ID, lk.ResourceDeleteParams{Cascade: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resources.Get(ctx, ice.ID); !errors.Is(err, lk.ErrNotFound) {
		t.Fatalf("child after cascade: %v", err)
	}
	restored, err := c.Resources.Restore(ctx, ooo.ID)
	if err != nil || restored.ID != ooo.ID {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	if _, err := c.Resources.Get(ctx, ice.ID); err != nil {
		t.Fatalf("child not restored with parent: %v", err)
	}
	if _, err := c.Resources.Restore(ctx, ooo.ID); !errors.Is(err, lk.ErrNotDeleted) {
		t.Fatalf("restoring a live resource: %v", err)
	}
}

func TestIntegration_StableIDsAndOrder(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	ids := map[string]string{"ooo": lk.NewID(), "candy": lk.NewID(), "fire": lk.NewID(), "ice": lk.NewID()}
	ooo := create(t, c, lk.ResourceCreate{ID: lk.Some(ids["ooo"]), Name: "The Land of Ooo"})
	if ooo.ID != ids["ooo"] {
		t.Fatalf("client id not kept: %s", ooo.ID)
	}
	create(t, c, lk.ResourceCreate{ID: lk.Some(ids["candy"]), Name: "Candy", ParentID: lk.Some(ooo.ID)})
	create(t, c, lk.ResourceCreate{ID: lk.Some(ids["fire"]), Name: "Fire", ParentID: lk.Some(ooo.ID)})
	create(t, c, lk.ResourceCreate{ID: lk.Some(ids["ice"]), Name: "Ice", ParentID: lk.Some(ooo.ID), Before: lk.Some(ids["fire"])})

	order := func() []string {
		t.Helper()
		var items []lk.ResourceListItem
		for r, err := range c.Resources.All(ctx, lk.ResourceListParams{ParentID: ooo.ID}) {
			if err != nil {
				t.Fatal(err)
			}
			items = append(items, r)
		}
		lk.SortByRank(items)
		var out []string
		for _, r := range items {
			out = append(out, r.ID)
		}
		return out
	}
	if got := order(); !slices.Equal(got, []string{ids["candy"], ids["ice"], ids["fire"]}) {
		t.Errorf("order = %v", got)
	}
	if _, err := c.Resources.Move(ctx, ids["fire"], lk.ResourceMove{After: lk.Some(ids["candy"])}); err != nil {
		t.Fatal(err)
	}
	if got := order(); !slices.Equal(got, []string{ids["candy"], ids["fire"], ids["ice"]}) {
		t.Errorf("order after move = %v", got)
	}

	_, err := c.Resources.Create(ctx, lk.ResourceCreate{ID: lk.Some(ids["candy"]), Name: sandbox.Name("dup")})
	if !errors.Is(err, lk.ErrDuplicateID) {
		t.Fatalf("duplicate id: %v", err)
	}
}

func TestIntegration_CreateManyAndPagination(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	root := lk.NewID()
	items := []lk.ResourceCreate{{ID: lk.Some(root), Name: sandbox.Name("Batch root")}}
	for _, n := range []string{"Candy Kingdom", "Ice Kingdom", "Fire Kingdom", "Lumpy Space", "Wildberry Kingdom"} {
		items = append(items, lk.ResourceCreate{ID: lk.Some(lk.NewID()), Name: sandbox.Name(n), ParentID: lk.Some(root)})
	}
	created, err := c.Resources.CreateMany(ctx, items)
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, root)
	if len(created) != len(items) {
		t.Fatalf("created %d, want %d", len(created), len(items))
	}
	for i := range items {
		if want, _ := items[i].ID.Get(); created[i].ID != want {
			t.Errorf("item %d out of order: %s", i, created[i].ID)
		}
	}

	// Five children at two per page: three pages, every child once.
	first, err := c.Resources.List(ctx, lk.ResourceListParams{ParentID: root, Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 2 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("first page = %+v", first)
	}
	seen := map[string]bool{}
	for r, err := range first.All(ctx) {
		if err != nil {
			t.Fatal(err)
		}
		if seen[r.ID] {
			t.Fatalf("%s yielded twice", r.ID)
		}
		seen[r.ID] = true
	}
	if len(seen) != 5 {
		t.Fatalf("iterated %d children, want 5", len(seen))
	}

	// One failing item fails the batch and creates nothing.
	bad := lk.NewID()
	_, err = c.Resources.CreateMany(ctx, []lk.ResourceCreate{{ID: lk.Some(bad), Name: sandbox.Name("ok")}, {Name: ""}})
	var apiErr *lk.APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, lk.ErrValidation) || len(apiErr.Errors) == 0 {
		t.Fatalf("bad batch: %v", err)
	}
	if _, err := c.Resources.Get(ctx, bad); !errors.Is(err, lk.ErrNotFound) {
		sandbox.Cleanup(t, bad)
		t.Fatalf("a failed batch created an item: %v", err)
	}
}

func TestIntegration_Idempotency(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	key := sandbox.RunPrefix + "-gunter"
	body := lk.ResourceCreate{Name: sandbox.Name("Gunter"), Tags: lk.Some([]string{"Penguin"})}
	gunter, err := c.Resources.Create(ctx, body, lk.WithIdempotencyKey(key))
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, gunter.ID)
	again, err := c.Resources.Create(ctx, body, lk.WithIdempotencyKey(key))
	if err != nil || again.ID != gunter.ID {
		t.Fatalf("replay: %+v %v (want the same resource)", again, err)
	}
	_, err = c.Resources.Create(ctx, lk.ResourceCreate{Name: sandbox.Name("Gunther, Destroyer of Worlds")}, lk.WithIdempotencyKey(key))
	if !errors.Is(err, lk.ErrIdempotencyKeyReused) {
		t.Fatalf("different body, same key: %v", err)
	}
}

func TestIntegration_Errors(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	_, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: ""})
	var e *lk.APIError
	if !errors.As(err, &e) || e.Code != lk.CodeValidationError || e.RequestID == "" || len(e.Errors) == 0 || e.Errors[0].Path != "name" {
		t.Fatalf("empty name: %#v", err)
	}
	if _, err := c.Resources.List(ctx, lk.ResourceListParams{Limit: 500}); !errors.Is(err, lk.ErrValidation) {
		t.Fatalf("limit 500: %v", err)
	}
	_, err = c.Resources.Get(ctx, "no-such-resource")
	if !errors.As(err, &e) || e.Code != lk.CodeNotFound || e.RequestID == "" {
		t.Fatalf("missing resource: %v", err)
	}
}
