# Fetched sources

| Field | Value |
|---|---|
| Fetch date | 2026-10-23 |
| `openapi.json` source | https://www.legendkeeper.com/api/v3/openapi.json |
| `openapi` (spec dialect) | 3.1.0 |
| `info.version` | 3.0.0 |
| `openapi.json` sha256 | `99109f621c18cbc5ae8b069ac301243e9d8c85a83c1903894d90a7a06853942b` |
| `sdk-guide.md` source | https://www.legendkeeper.com/developers/guide/ |
| `style-guide.md` source | https://www.legendkeeper.com/developers/style/ |
| `quickstart.md` source | https://www.legendkeeper.com/developers/quickstart/ |

`openapi.json` is stored byte-for-byte as served; never reformat it.

The three Markdown files were converted from the HTML pages with
`github.com/JohannesKaufmann/html-to-markdown/v2` (a throwaway tool, not a
repo dependency). Only the page's `<main>` element was converted; navigation,
scripts and buttons were stripped. Treat them as read-only copies — re-fetch
rather than hand-edit.

Verify:

```sh
shasum -a 256 docs/legendkeeper/openapi.json
```
