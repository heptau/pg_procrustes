# pg_procrustes for VS Code

Formats PostgreSQL SQL and PL/pgSQL with [pg_procrustes](https://github.com/heptau/pg_procrustes), a formatter driven by the native PostgreSQL parser, and shows syntax errors while you type. The extension is a thin client: it starts `pg_procrustes --lsp` and VS Code talks to it over the Language Server Protocol.

## Requirements

The `pg_procrustes` binary ([installation](https://github.com/heptau/pg_procrustes#installation)):

```bash
brew install heptau/tap/pg-procrustes
```

The extension finds it on `PATH` and in the usual install folders (`/opt/homebrew/bin`, `/usr/local/bin`, `~/go/bin`); otherwise set `pgProcrustes.path`.

## Features

- **Format Document** and **Format Selection** (only the statements touching the selection are formatted).
- **Syntax errors** as diagnostics while you type (a warning when the config sets `syntax_errors: skip`). A document that does not parse is never touched.
- The config is the nearest `.pg_procrustes.yaml` above each file, re-read on every format; no restart after editing it.

To format on save and use pg_procrustes for SQL:

```json
{
  "[sql]": {
    "editor.defaultFormatter": "heptau.pg-procrustes",
    "editor.formatOnSave": true
  }
}
```

## Settings

| Setting | Default | |
|---|---|---|
| `pgProcrustes.path` | `pg_procrustes` | Executable name or absolute path |
| `pgProcrustes.configPath` | _empty_ | One fixed config for all documents |
| `pgProcrustes.trace.server` | `off` | `messages` / `verbose` log the LSP traffic to the "pg_procrustes" output channel |

Commands: **pg_procrustes: Restart Language Server**, **pg_procrustes: Show Output**.

## Languages

The extension activates for the `sql` and `pgsql` language ids.

## Build from source

```bash
cd editors/vscode
npm install
npm run package     # creates pg-procrustes-<version>.vsix
code --install-extension pg-procrustes-*.vsix
```

Press F5 in this folder to start an Extension Development Host.
