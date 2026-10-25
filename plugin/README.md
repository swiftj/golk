# LegendKeeper for Claude

Two pieces, installed separately:

| Piece | File | What it does |
|---|---|---|
| Claude Desktop extension | `lk-mcp-darwin-arm64.mcpb` | Runs lk-mcp, the server that gives Claude the LegendKeeper tools. Apple Silicon Macs only. |
| Cowork plugin | `legendkeeper-plugin.zip` | Two skills that teach Claude how to use those tools well: `legendkeeper-wiki` and `legendkeeper-worldbuilding`. |

Build both with `make mcpb plugin` in the repository root; they land in `dist/`.

## 1. Install the Desktop extension

1. Double-click `dist/lk-mcp-darwin-arm64.mcpb` (or drag it into Claude Desktop, or use Settings › Extensions › Advanced settings › Install Extension).
2. Fill in the settings:
   - **LegendKeeper API key:** the key of the one project Claude should work in. Start with a **sandbox** project, not the wiki your players read.
   - **Spoiler terms file** (optional): leave it empty and Claude has full DM access, so every write is allowed. To protect spoilers, choose a text file with one term per line, kept outside the wiki and out of any repository. A write that would show one of those terms to players is then refused. Secret blocks and hidden pages, documents and properties may still hold them.
   - **Read-only:** offers Claude only the read tools.
3. In a new chat, ask "List my LegendKeeper projects". The answer names the project and says whether the guard is on.

The key is stored by Claude Desktop, masked, and never written to a file by lk-mcp. To change a setting later, open Settings › Extensions › LegendKeeper.

## 2. Install the plugin

In Cowork: Customize › Plugins › Add › Upload plugin, then choose `dist/legendkeeper-plugin.zip`.

The plugin carries no server and no key. The tools come from the Desktop extension.

## Upgrading

Install the new `.mcpb` over the old one. Your settings are kept. Bump `version` in `plugin/mcpb/manifest.json` and `plugin/.claude-plugin/plugin.json` together (a test checks that they match).
