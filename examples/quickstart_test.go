//go:build integration

package examples

import (
	"os"
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
)

func TestMain(m *testing.M) { os.Exit(sandbox.Main(m)) }

// TestQuickstartFirstCall ports "Make a first call" (quickstart.md and
// sdk-guide.md, Getting started). The guide builds its client from
// LEGENDKEEPER_API_KEY; this port uses the guarded sandbox client instead,
// so it can never touch a real project.
func TestQuickstartFirstCall(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)

	project, err := c.Project(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Connected to %s (%s)", project.Name, project.ID)

	resource, err := c.Resources.Create(ctx, lk.ResourceCreate{Name: sandbox.Name("The Tree Fort")})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, resource.ID) // in case the delete below is never reached
	t.Logf("Created %s (%s)", resource.Name, resource.ID)

	if err := c.Resources.Delete(ctx, resource.ID, lk.ResourceDeleteParams{}); err != nil {
		t.Fatal(err)
	}
}
