//go:build integration

package examples

import (
	"testing"

	"github.com/swiftj/golk/internal/sandbox"
	"github.com/swiftj/golk/lk"
	"github.com/swiftj/golk/lk/blocks"
)

// TestTemplates ports the guide's "templates" example.
func TestTemplates(t *testing.T) {
	c, ctx := sandbox.Client(t), sandbox.Ctx(t)
	kingdom := createResource(t, c, lk.ResourceCreate{
		Name: "Kingdom", Tags: lk.Some([]string{"Kingdom"}),
		Documents: lk.Some([]lk.ResourceDocument{{Blocks: blocks.List{blocks.P("Ruler, capital, allies.")}}}),
	})
	createResource(t, c, lk.ResourceCreate{Name: "Castle", ParentID: lk.Some(kingdom.ID)})

	template, err := c.Resources.SaveAsTemplate(ctx, kingdom.ID, lk.SaveAsTemplate{
		Name: lk.Some(sandbox.Name("Kingdom Template")), IncludeChildren: lk.Some(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, template.ID)

	for listed, err := range c.Templates.All(ctx, lk.TemplateListParams{}) {
		if err != nil {
			t.Fatal(err)
		}
		t.Log("template:", listed.Name)
	}

	realm, err := c.Resources.CreateFromTemplate(ctx, lk.FromTemplate{
		TemplateID: template.ID, Name: lk.Some(sandbox.Name("Realm")), IncludeChildren: lk.Some(true),
	})
	if err != nil {
		t.Fatal(err)
	}
	sandbox.Cleanup(t, realm.ID)
	pg, err := c.Documents.GetBlocks(ctx, page(realm))
	if err != nil {
		t.Fatal(err)
	}
	t.Log(realm.Name, realm.Tags, pg.Text)
	n := 0
	for child, err := range c.Resources.All(ctx, lk.ResourceListParams{ParentID: realm.ID}) {
		if err != nil {
			t.Fatal(err)
		}
		t.Log("child:", child.Name)
		n++
	}
	if pg.Text != "Ruler, capital, allies." || n != 1 {
		t.Errorf("realm text %q, %d children", pg.Text, n)
	}
}
