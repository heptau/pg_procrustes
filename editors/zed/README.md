# pg_procrustes for Zed

Runs [pg_procrustes](https://github.com/heptau/pg_procrustes) as a language server for SQL in [Zed](https://zed.dev): format document / selection and syntax errors as diagnostics. The extension only starts `pg_procrustes --lsp`; the formatting itself is done by the binary.

## Requirements

- The `pg_procrustes` binary on your `PATH` ([installation](https://github.com/heptau/pg_procrustes#installation)), for example `brew install heptau/tap/pg-procrustes`.
- Zed's **SQL** extension (it provides the `SQL` language this server attaches to).

## Settings

Use pg_procrustes as the SQL formatter (`settings.json`):

```json
{
  "languages": {
    "SQL": {
      "language_servers": ["pg_procrustes", "..."],
      "formatter": { "language_server": { "name": "pg_procrustes" } },
      "format_on_save": "on"
    }
  }
}
```

If Zed does not see your shell's `PATH`, or you want one fixed config, point at them:

```json
{
  "lsp": {
    "pg_procrustes": {
      "binary": { "path": "/opt/homebrew/bin/pg_procrustes" },
      "initialization_options": { "configPath": "/path/to/.pg_procrustes.yaml" }
    }
  }
}
```

Without `configPath` the nearest `.pg_procrustes.yaml` above each file is used and re-read on every format.

## Install as a dev extension

Needs a Rust toolchain installed through [rustup](https://rustup.rs) (Zed builds the extension itself):

1. Open the command palette and run **zed: install dev extension**.
2. Choose this folder (`editors/zed`).

## Publish

Extensions are published by a pull request to [zed-industries/extensions](https://github.com/zed-industries/extensions) that adds this folder as a git submodule with `path = "editors/zed"`; see the [Zed extension docs](https://zed.dev/docs/extensions/developing-extensions#publishing-your-extension).
