---
name: legendkeeper-worldbuilding
description: Work with LegendKeeper maps, timelines, calendars, templates and boards through the lk-mcp tools. Use when the user asks to place or move map pins, labels, regions or paths; add timeline lanes or events; create or change a custom calendar; save or use a template; or draw shapes and arrows on a board.
---

# LegendKeeper worldbuilding

Each of these lives in a document (tab) of a resource. Read before you write, keep ids from the read, and use `dry_run: true` before deleting or replacing. The spoiler guard and hidden-content rules from the legendkeeper-wiki skill apply to everything here. In addition, **hidden pins and hidden timeline events are still checked like public content**, so a protected term can't go on one.

## Maps

- `read_map` shows the map image (size and bounds) and every feature: pins, labels, regions, paths and folders, with ids and positions `[lat, lng]`.
- `set_map_feature` creates a feature (give `type`), or updates one by `feature_id`, sending only the fields you give.
  - Place it with `pos: [lat, lng]`, or with `pixel: [x, y]` counted from the image's top-left corner, but not both.
  - Group features with a folder (`type: folder`) and `parent_id`. Link a feature to an entry with `uri: lk://resources/<id>`.
  - `style` holds colours, icons and label text settings.
- `delete_map_feature` deletes one feature. Deleting a folder leaves its features in place.
- `set_map_image` sets the image from an `asset_id` returned by `upload_image`. Features keep their positions.
- A blank document becomes a map when you write to it by `document_id`.

## Timelines

- `read_timeline` shows lanes by rank and one page of events (pass `cursor` for more) with formatted dates. Lanes and events from chained timelines are marked.
- Create a lane first: `set_timeline_lane` takes `name`, `rank` (`a0`, `a1`, …) and `size` (`sm` or `lg`).
- `set_timeline_event` takes `lane_id`, `type` (`event` or `note`) and `name`.
  - Give dates as `formatted_start`/`formatted_end` in the calendar's format, or as `start`/`end` in minutes.
  - `uri` links the event to an entry.
- `delete_timeline_lane` deletes the lane's events too, so dry run it first. `delete_timeline_event` deletes one event.
- `update_document` sets a timeline's `calendar_id`, or its chained `sources` (other timelines it shows).

## Calendars

- `list_calendars` lists custom and built-in calendars with versions. `get_calendar` returns one calendar's full spec as JSON.
- `create_calendar` takes a spec. `replace_calendar` takes the whole spec plus the `version` from `get_calendar`. `delete_calendar` takes the `version` too.
- To change one detail, `get_calendar`, edit that field, and `replace_calendar` with the rest unchanged.
- Built-in calendars can't be changed.

## Templates

- `list_templates` lists them.
- `save_as_template` copies a resource (`include_children` for its subtree) into a template. `create_from_template` makes a new resource from one.
- A copy keeps each resource's own hidden flag, and the copy is checked by the spoiler guard.

## Boards

A board is a tldraw canvas of shapes, arrows (bindings) and images (assets).

1. `read_board` shows each shape's id, type, position and text, arrows as "from A to B", and the board `version`. Add `raw: true` for the full JSON records.
2. Pass that `version` to every write. A write after someone else's change is refused, so read again.

The write tools:
- **`add_board_shapes`:** each shape needs only `type` (`geo`, `text`, `note`, `arrow`, `frame`, …). Put `w`, `h`, `text` and `color` in `props`.
  - Connect an arrow with two bindings: `{type: "arrow", fromId: <arrow id>, toId: <shape id>, props: {terminal: "start"}}`, then the same with `terminal: "end"`.
  - Give your own ids (`shape:candy-kingdom`) so bindings can name shapes made in the same call.
- **`update_board_shapes`** changes shapes by `id`. `props` merge, so send only what changes.
- **`delete_board_shapes`** also removes shapes inside frames and groups, and arrows' bindings.
- **`replace_board`** swaps all content. The records from `read_board raw=true` can be sent back as they are.

A blank document becomes a board when you add shapes to it by `document_id`.
