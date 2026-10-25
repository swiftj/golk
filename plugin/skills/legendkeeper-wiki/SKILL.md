---
name: legendkeeper-wiki
description: Read and edit a LegendKeeper world wiki through the lk-mcp tools. Use when the user asks to look up, create, update, reorganise, hide or delete wiki entries (resources), their pages, sidebar properties, tags or images in LegendKeeper, or mentions their campaign wiki, DM notes or spoilers.
---

# LegendKeeper wiki

The DM's players read this wiki. Never let DM-only content reach a page players can see, and never overwrite the DM's own edits.

## The model

- A **resource** is a wiki entry in the sidebar. It has a name, tags, aliases, a parent (resources nest), and one or more **documents**, which are its tabs.
- A document is a **page** (text blocks), a **map**, a **time** document (a timeline) or a **board**. A **blank** document becomes whichever kind is written to it first.
- **Sidebar properties** sit beside the page, such as a key-value "Stats" box or a text field.
- **Hidden** content is DM-only: a hidden resource (all of it), a hidden document, a hidden property or infobox row, and anything inside a `:::secret` block. "Hidden" is not inherited; each resource and document has its own flag.

## Finding things

- `search` finds text in names, aliases and pages. `list_resources` walks the tree (pass `parent_id`). `get_resource` shows a resource's documents, tags and properties.
- `read_page` returns a page as LK Markdown with its `version`. `list_tags` and `list_assets` list tags and images.
- Every tool takes an optional `project` when several projects are configured. `list_projects` lists them and says whether the spoiler guard is on.

## Editing a page

1. `read_page` the page and keep its `version`.
2. Change the Markdown it returned, keeping everything you aren't changing exactly as it was, including `lk-raw` fences and `::unsupported` lines. They hold content Markdown can't show, and deleting them deletes it.
3. `edit_page` with `base_version` set to that version and the **whole** edited page. Blocks you leave out are deleted.
4. If it says the page changed since it was read, someone edited it in the app. Read it again and redo the change. Never retry with a guessed version.

Use `dry_run: true` first for anything large or destructive. It reports what would change and writes nothing.

## LK Markdown cheatsheet

Standard Markdown (CommonMark plus GFM tables, task lists and strikethrough) for paragraphs, headings, lists, quotes, code and images. LegendKeeper blocks are directives:

```
:::secret
Only the DM sees this. The Lich is not really gone.
:::

:::callout tone=warning
Beware the **Lich**.
:::

:::expand title="Rumours"
Body Markdown.
:::

:::keyvalue title=Stats
- Species :: Magic dog
:::
```

- Inline extras: `[x]{underline}`, `[x]{color=#EC4899}` and mentions `[Princess Bubblegum]{mention=<resourceId>}`.
- A line that starts with `::` plus a name is a directive. Write `\::` for literal colons, and `\{` for a literal brace.
- Raw HTML and indented code blocks are errors.
- An error message names the line and how to fix it. Fix the Markdown rather than dropping content.

## Keeping DM content hidden

- **Secrets on a public page** go in a `:::secret` block.
- **A whole DM-only entry** is created with `create_resource is_hidden=true`. A DM-only tab is `create_document is_hidden=true`. A DM-only sidebar box is `set_property is_hidden=true`.
- **Showing something** (`update_resource is_hidden=false`, `update_document is_hidden=false`, `restore_resource`) first checks everything it would reveal.

## The spoiler guard

`list_projects` says whether it is on. Without a terms file it is off, and every write is allowed (full DM access). When it is on, every write is checked against the DM's list of protected terms. A write that would put one on content players can see is refused, and the error lists each term and where it is.

When a write is refused:
- **Don't** rephrase, misspell, split or abbreviate the term to get past the check. That defeats the DM's protection.
- **Do** tell the DM which term was found where, and offer to put that part in a `:::secret` block or a hidden document or resource.

## Other tools

- **Resources:** `create_resource` (optionally with `page_markdown`), `update_resource` (rename, tags, aliases, move, hide or show), `delete_resource` (`cascade` for children; dry run first) and `restore_resource`.
- **Documents:** `create_document` and `update_document` (rename, hide or show).
- **Sidebar properties:** `set_property` (by title; Markdown or rows) and `delete_property`.
- **Images:** `upload_image` (from a local file path), then use the returned URL in `![alt](url)`. Only this project's asset URLs are accepted.

For maps, timelines, calendars, templates and boards, see the legendkeeper-worldbuilding skill.
