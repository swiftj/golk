---
name: legendkeeper-wiki
description: Read and edit a LegendKeeper world wiki through the lk-mcp tools. Use when the user asks to look up, create, update, reorder, reorganise, hide or delete wiki entries (resources), their pages, blocks, tabs, sidebar properties, infoboxes, tags, images or assets in LegendKeeper, manage the project's members and invites, or mentions their campaign wiki, DM notes or spoilers.
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

**Small edits to one block**: `read_page` with `ids: true` (or `read_block`) shows block ids. Then `insert_block`, `replace_block`, `update_block` (one field, e.g. tick a task with `patch: {checked: true}`, or `content_markdown`), `move_block` or `delete_block`, each with the page `version`. Each returns the new version for the next edit. A page written in the app long ago may need `assign_block_ids` once first.

## Ordering the sidebar and tabs

- `list_resources` with `parent_id` lists siblings in sidebar order and shows each one's `rank`.
- To reorder, `update_resource` with `after` or `before` a sibling id. To sort children alphabetically, move each one `after` the previous in turn.
- Tabs (documents) are ordered by `update_document rank`; `get_resource` shows each tab's rank.

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

## Properties and infoboxes

- `set_property` sets a sidebar property by title: `markdown` (a text field), `rows` (key-value), or `property_markdown` for **any kind**, written as `get_resource` shows it, e.g.:

  ```
  :::meter mode=bar title=Vitals
  - HP :: 3/5 icon=heart
  :::
  ```

  Use `rank` to move an existing property, and `after`/`before` to place a new one.
- With `document_id` and `block_id`, `set_property` and `delete_property` work on an **infobox** inside a page.
- `set_row` / `delete_row` change one key-value row without touching the others. They work on a sidebar property (`property_id`), a keyValue block (`document_id` + `block_id`), or an infobox property (all three).

## Other tools

- **Resources:** `create_resource` (with `page_markdown`, or `documents` of any type), `create_resources` (up to 100 at once), `update_resource` (rename, tags, aliases, icon, lock, banner, permissions, move, reorder, hide or show), `delete_resource` (`cascade` for children; dry run first) and `restore_resource`.
- **Documents:** `create_document` (page, map, time, board or blank), `update_document` (rename, hide or show, tab order, full width), `delete_document` and `restore_document`.
- **Sidebar properties:** `set_property` (by title; Markdown or rows) and `delete_property`.
- **Asset library:** `list_assets`, `get_asset`, `create_asset_folder` and `update_asset` (rename, describe, move, hide).
- **Images:** `upload_image` (from a local file path), then use the returned URL in `![alt](url)`. Only this project's asset URLs are accepted. To replace an image, upload the new one, swap its URL into the page with `edit_page`, then remove the old one with `delete_asset` (use `list_assets` to find it, and dry run first). A deleted image stays on pages that already show it, but can't be added anywhere new.

For maps, timelines, calendars, templates and boards, see the legendkeeper-worldbuilding skill.

## Members and invites

`list_members` and `list_invites` show who can read the wiki. `invite_member` emails someone an invite, and `remove_member` and `revoke_invite` take access away. These change who your players are, so **always confirm with the user before inviting, removing or revoking**, and dry run first.
