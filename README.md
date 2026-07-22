# pg_procrustes

[![CI](https://github.com/heptau/pg_procrustes/actions/workflows/ci.yml/badge.svg)](https://github.com/heptau/pg_procrustes/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/heptau/pg_procrustes)](https://goreportcard.com/report/github.com/heptau/pg_procrustes)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A fast, flexible PostgreSQL formatter in Go. Driven by the native PostgreSQL parser, it understands your code like the DB itself. You design the Procrustean bed via rich config, and it shapes your queries with precision. 🛏️📐

## The name

In Greek mythology, Procrustes was a bandit who operated an iron bed on the road to Athens. He invited travellers to spend the night — then made sure they fit the bed perfectly. If a guest was too tall, he cut off their legs. If too short, he stretched them until they matched. Either way, everyone fit.

pg_procrustes works the same way. You define the bed — the formatting rules — via `.pg_procrustes.yaml`. Your SQL will fit. Whether it likes it or not.

## Installation

**Homebrew** (macOS and Linux):
```bash
brew install heptau/tap/pg-procrustes
```

**go install** (requires Go toolchain):
```bash
go install github.com/heptau/pg_procrustes@latest
```

**Binary download**: grab the archive for your platform from [GitHub Releases](https://github.com/heptau/pg_procrustes/releases), extract, and place the binary on your `PATH`.

### Editor integration

**Neovim** (requires the `pg_procrustes` binary on your `PATH` — see above): this repository doubles as a Neovim plugin. With [lazy.nvim](https://github.com/folke/lazy.nvim):

```lua
{
  "heptau/pg_procrustes",
  ft = "sql",
  config = function()
    require("pg_procrustes").setup()
  end,
}
```

With [packer.nvim](https://github.com/wbthomason/packer.nvim):

```lua
use({
  "heptau/pg_procrustes",
  config = function()
    require("pg_procrustes").setup()
  end,
})
```

If [conform.nvim](https://github.com/stevearc/conform.nvim) is installed, `sql` buffers get `pg_procrustes` registered as a formatter automatically — `require("conform").format()` and format-on-save pick it up like any other formatter. Without conform.nvim, use `:PgProcrustesFormat` to format the current buffer directly. Run `:checkhealth pg_procrustes` to verify the setup. See `:help pg_procrustes` for details, or [pg_procrustes.80.cz](https://pg_procrustes.80.cz).

**Zed** has built-in support for external formatters — no extension needed. Add this to your project (`.zed/settings.json`) or user (`~/.config/zed/settings.json`) settings:

```json
{
  "languages": {
    "SQL": {
      "formatter": {
        "external": {
          "command": "pg_procrustes",
          "arguments": []
        }
      }
    }
  }
}
```

Then format with the `editor: format` command, or set `"format_on_save": "on"`.

**VS Code** has no built-in "external formatter" option, but the [Custom Local Formatters](https://marketplace.visualstudio.com/items?itemName=jkillian.custom-local-formatters) extension adds one purely through settings:

```json
"customLocalFormatters.formatters": [
  {
    "command": "pg_procrustes",
    "languages": ["sql"]
  }
],
"[sql]": {
  "editor.defaultFormatter": "jkillian.custom-local-formatters"
}
```

Format Document (`Shift+Alt+F`) and `editor.formatOnSave` will now run `pg_procrustes`.

**DataGrip & other JetBrains IDEs**: DataGrip bundles the *File Watchers* plugin (enable it under `Settings → Tools → File Watchers` if it's off). Add a custom watcher with:

| Field | Value |
| --- | --- |
| File type | `SQL` |
| Program | path to `pg_procrustes` (or just the binary name if it resolves on `PATH`) |
| Arguments | `-w $FilePath$` |
| Output paths to refresh | `$FilePath$` |

The file is reformatted in place on every save. For a manual, on-demand trigger instead, use `Settings → Tools → External Tools` with the same Program/Arguments.

**Emacs**: with [apheleia.el](https://github.com/radian-software/apheleia) (async, keeps point position):

```elisp
(setf (alist-get 'pg-procrustes apheleia-formatters) '("pg_procrustes"))
(setf (alist-get 'sql-mode apheleia-mode-alist) 'pg-procrustes)
(add-hook 'sql-mode-hook #'apheleia-mode)
```

Or with the lighter [reformatter.el](https://github.com/purcell/reformatter.el):

```elisp
(reformatter-define pg-procrustes-format
  :program "pg_procrustes")
(add-hook 'sql-mode-hook #'pg-procrustes-format-on-save-mode)
```

Either gives you format-on-save without writing a package of your own.

**Helix**: add to `~/.config/helix/languages.toml` (or a project-local `.helix/languages.toml`):

```toml
[[language]]
name = "sql"
formatter = { command = "pg_procrustes" }
auto-format = true
```

Format on demand with `:format`, or let `auto-format` run it on every save.

**Sublime Text**: install [Fmt](https://packagecontrol.io/packages/Fmt) via Package Control, then add to its settings (`Preferences → Package Settings → Fmt → Settings`):

```json
{
  "rules": [
    {
      "selector": "source.sql",
      "cmd": ["pg_procrustes"],
      "format_on_save": true,
      "merge_type": "diff"
    }
  ]
}
```

Or trigger it manually from the command palette with **Fmt: Format Buffer**.

**Vim** (via [ALE](https://github.com/dense-analysis/ale)): define a fixer in your `.vimrc`:

```vim
let g:ale_fixers = {
\   'sql': [
\     {buffer -> {'command': 'pg_procrustes'}},
\   ],
\}
let g:ale_fix_on_save = 1
```

Or run it on demand with `:ALEFix`.

Zed, VS Code, DataGrip, Emacs, Helix, Sublime Text, and Vim above are config-only integrations — no dedicated plugin required, though one may follow later. If your editor launches as a GUI app rather than from a terminal, it may not see your shell's `PATH`; use the absolute path from `which pg_procrustes` if the command isn't found.

### Automation & CI

**pre-commit**: add a [pre-commit](https://pre-commit.com) local hook (requires `pg_procrustes` on `PATH` — see [Installation](#installation)) to `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: local
    hooks:
      - id: pg_procrustes
        name: pg_procrustes
        language: system
        entry: pg_procrustes --check
        files: \.sql$
```

`--check` fails the commit without touching files, so you review the diff and run `pg_procrustes -w` yourself. Prefer autofix-on-commit instead? Swap the entry for `pg_procrustes -w` — pre-commit detects that files were modified and still fails that run, so you can review, re-stage, and commit again.

**GitHub Actions**: a minimal CI job that fails a pull request if any tracked `.sql` file isn't formatted:

```yaml
name: SQL formatting
on: [pull_request]

jobs:
  pg_procrustes:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: go install github.com/heptau/pg_procrustes@latest
      - run: pg_procrustes --check $(git ls-files '*.sql')
```

Swap the `go install` step for the [Homebrew or binary download](#installation) methods if you'd rather not depend on a Go toolchain in CI.

## Usage

```bash
# Format one or more files in place
pg_procrustes -w query.sql
pg_procrustes -w migrations/*.sql

# Print formatted output to stdout
pg_procrustes query.sql

# Read from stdin, write to stdout
cat query.sql | pg_procrustes

# CI mode — exit 1 if any file would change
pg_procrustes --check *.sql

# Show a unified diff without writing
pg_procrustes --diff query.sql

# Save original before overwriting (default extension: .bak)
pg_procrustes -w --backup query.sql
pg_procrustes -w --backup=.orig query.sql

# Write formatted files to a different directory
pg_procrustes --out-dir formatted/ migrations/*.sql
```

## Configuration

pg_procrustes looks for `.pg_procrustes.yaml` starting in the current directory and walking up to the filesystem root. Specify a custom path with `-c path/to/config.yaml`. All settings default to `preserve` (no change).

> **Note:** The `.pg_procrustes.yaml` in this repository is the configuration used to format pg_procrustes' own source files — not a default template. Copy and adjust it for your own project.

### Keyword and identifier casing

```yaml
reserved_keywords:
  case: upper     # upper | lower | preserve  (SELECT, FROM, WHERE, …)

keywords:
  case: upper     # upper | lower | preserve  (ANALYZE, CASCADE, VERBOSE, …)

data_types:
  case: lower     # upper | lower | preserve
  form: long      # preserve | long | short | long_no_space
                  #   long:          character varying, double precision
                  #   short:         varchar, float8, timestamptz
                  #   long_no_space: integer, bigint, varchar (no multi-word forms)

literals:
  case: upper     # upper | lower | preserve  (TRUE, FALSE, NULL)

operators:
  case: upper     # upper | lower | preserve  (AND, OR, NOT, IN, IS, LIKE, …)

schemas:
  case: lower     # upper | lower | preserve
tables:
  case: lower     # upper | lower | preserve
functions:
  case: lower     # upper | lower | preserve  (now(), count(), my_func(), …)
conditional_functions:
  case: upper     # upper | lower | preserve  (COALESCE, NULLIF, GREATEST, LEAST)
system_functions:
  case: upper     # upper | lower | preserve  (CURRENT_DATE, SESSION_USER, …)
aliases:
  case: lower     # upper | lower | preserve
  as: add         # add | preserve | remove
columns:
  case: lower     # upper | lower | preserve

plpgsql_variables:
  case: upper     # upper | lower | preserve
                  # Controls PL/pgSQL runtime values: NEW, OLD, EXCLUDED (ON CONFLICT),
                  # FOUND, TG_OP, TG_TABLE_NAME, TG_TABLE_SCHEMA, TG_NAME, TG_WHEN,
                  # TG_LEVEL, TG_NARGS, TG_ARGV, TG_RELID, TG_RELNAME, TG_EVENT, TG_TAG,
                  # SQLSTATE, SQLERRM, ROW_COUNT, PG_CONTEXT, PG_EXCEPTION_DETAIL,
                  # PG_EXCEPTION_HINT, PG_EXCEPTION_CONTEXT, RETURNED_SQLSTATE,
                  # MESSAGE_TEXT, PG_DATATYPE_NAME, PG_ROUTINE_OID.
                  # When omitted, NEW/OLD follow keywords.case; TG_*/FOUND are left as-is.

plpgsql_keywords:
  case: upper     # upper | lower | preserve
                  # Controls PL/pgSQL-only statement keywords that the PostgreSQL scanner
                  # does not classify as reserved or unreserved keywords and therefore
                  # receive no transformation from the keywords/reserved_keywords rules:
                  # RAISE, PERFORM, ELSIF, ELSEIF, FOREACH, REVERSE, SLICE, EXIT, LOOP,
                  # WHILE, OPEN, ASSERT, EXCEPTION (as RAISE severity / handler keyword),
                  # DEBUG, INFO, NOTICE, WARNING.
                  # When omitted these keywords are written exactly as they appear in source.
```

Every casing section accepts an optional `exceptions` list. Words that match an
exception (comparison is case-insensitive) are left exactly as written in the
source — the `case` rule is not applied to them.

```yaml
# Example: NAME is a PostgreSQL built-in type that doubles as a keyword.
# Keep it uppercase even though data_types.case is lower.
data_types:
  case: lower
  exceptions:
    - NAME

# Example: preserve the exact capitalisation of two non-standard keywords.
keywords:
  case: upper
  exceptions:
    - someThing
    - AnotherWord
```

`exceptions` is available on every casing section: `reserved_keywords`,
`keywords`, `data_types`, `literals`, `operators`, `schemas`, `tables`,
`functions`, `conditional_functions`, `system_functions`, `aliases`, `columns`,
`plpgsql_variables`, and `plpgsql_keywords`.

### Punctuation and spacing

```yaml
trailing_whitespace: strip   # strip | preserve

semicolons: preserve         # preserve | add | remove

inequality_op: c             # preserve | ansi | c
                             #   ansi: always use <>
                             #   c:    always use !=

join_form: preserve          # preserve | short | long
                             #   short: JOIN, LEFT JOIN, FULL JOIN
                             #   long:  INNER JOIN, LEFT OUTER JOIN, FULL OUTER JOIN

operator_spacing: normalize  # preserve | normalize | compact
                             #   normalize: exactly one space around = != <> < > <= >= ||
                             #   compact:   no spaces around = != <> < > <= >= ||

comma_spacing: normalize     # preserve | normalize | compact
                             #   normalize: one space after comma, none before
                             #   compact:   no spaces around commas

blank_lines: preserve        # preserve | max_3 | max_2 | max_1

paren_spacing: remove        # preserve | add | remove

quoted_identifiers: remove_safe  # preserve | remove_safe

schema_qualification: preserve   # preserve | remove_public

cast_style: preserve         # preserve | operator
                             #   operator: always use CAST(x AS type)

order_asc: preserve          # preserve | add | remove
                             #   add:    make ASC explicit in ORDER BY
                             #   remove: strip redundant ASC

not_in: preserve             # preserve | not_in | not_equals_all
                             #   not_in:        always use NOT IN (...)
                             #   not_equals_all: always use <> ALL (...)
```

### Layout

The `layout` section controls clause-level line breaking and indentation.

```yaml
layout:
  line_length: 128           # target line length for auto-break decisions

  indent:
    size: 3                  # spaces per indent level (ignored when type: tab)
    type: spaces             # spaces | tab
    normalize: preserve      # preserve | change  (convert existing indent to configured type)
    remainder: keep          # keep | add | remove | round  (when converting spaces → tabs)

  clauses:
    break: preserve          # preserve | never | always | auto
    align: same              # same | indent
    # per-clause overrides (inherit from clauses.break / clauses.align when omitted):
    # from, join, where, group_by, having, order_by, limit, offset,
    # values, on_conflict, set, using, returning, with, exception
    # Example:
    # join: { break: always }
    # limit: { break: never }

  content:
    break: preserve          # preserve | never | always | auto
    align: indent            # same | indent
    first_item: break        # break | inline
                             #   break:  first item on new indented line (default)
                             #   inline: first item stays on keyword line, rest indented below
    # per-section overrides (each inherits content.break / content.align / content.first_item when omitted):
    #   select_list    — SELECT column list (comma-split)
    #   where_conds    — WHERE conditions (AND/OR split)
    #   having_conds   — HAVING conditions (AND/OR split)
    #   join_on        — JOIN ON conditions (AND/OR split)
    #   group_list     — GROUP BY items (comma-split)
    #   order_list     — ORDER BY items (comma-split)
    #   set_list       — UPDATE SET assignments (comma-split)
    #   insert_columns — INSERT INTO table (col1, col2, …) column list
    #   values_list    — VALUES row tuples (each tuple is one item)
    #   returning_list — RETURNING items (comma-split)
    #   with_list      — WITH CTE definitions (comma-split)
    # Example:
    # where_conds:    { break: always }
    # having_conds:   { break: always, first_item: inline }
    # select_list:    { break: always, first_item: inline }
    # insert_columns: { break: always, first_item: inline }
    # returning_list: { break: always, first_item: inline }
    # with_list:      { break: always }

  union:
    blank_line: preserve     # preserve | none | before | after | both

  case:
    break: preserve          # preserve | never | always | auto
                             #   never:   collapse CASE…END to single line
                             #   always:  expand WHEN/ELSE/END onto separate lines
                             #   auto:    expand when flat CASE length > line_length
    indent: indent           # preserve | none | indent
                             #   indent: WHEN/ELSE indented one level relative to CASE
                             #   none:   WHEN/ELSE at same level as CASE
```

#### Clause breaking

`clauses.break: auto` breaks all clauses when the full flat statement length exceeds `line_length`. Breaking is all-or-none — either every clause breaks or none do.

```sql
-- auto, line_length: 80
SELECT u.id, u.name
FROM users u
INNER JOIN orders o ON o.user_id = u.id
WHERE u.active = TRUE
ORDER BY u.name
```

#### Content breaking

`content.break: auto` breaks the items inside a clause when that clause's content length exceeds `line_length`. SELECT list splits at commas; WHERE/HAVING/JOIN conditions split at AND/OR (keyword placed at start of continuation line).

`content.first_item` controls where the first item lands when breaking occurs. `inline` keeps it on the clause keyword line; `break` (default) puts every item on its own line.

```sql
-- select_list: { break: always }       -- select_list: { break: always, first_item: inline }
SELECT                                  SELECT u.id,
   u.id,                                   u.name,
   u.name,                                 u.email
   u.email

-- where_conds: { break: always }       -- where_conds: { break: always, first_item: inline }
WHERE                                   WHERE u.active = TRUE
   u.active = TRUE                         AND u.account_type = 'premium'
   AND u.account_type = 'premium'

-- having_conds: { break: always, first_item: inline }
HAVING count(*) > 5
   AND avg(salary) > 50000

-- returning_list: { break: always, first_item: inline }
RETURNING id,
   name,
   created_at

-- with_list: { break: always }
WITH
   cte1 AS (SELECT id FROM t),
   cte2 AS (SELECT name FROM t2)

-- insert_columns: { break: always, first_item: inline }
INSERT INTO users (id,
   name,
   email
)
```

#### CASE expressions

`case.break: always` expands SQL `CASE…END` expressions in SELECT, WHERE, and other clauses:

```sql
-- before
SELECT CASE WHEN status = 'active' THEN 1 WHEN status = 'pending' THEN 0 ELSE -1 END FROM t

-- after (break: always, indent: indent)
SELECT CASE status
  WHEN 'active' THEN 1
  WHEN 'pending' THEN 0
  ELSE -1
END FROM t
```

`case.break: auto` expands CASE only when the flat `CASE…END` length exceeds `line_length`.

### PL/pgSQL formatting

Dollar-quoted function and procedure bodies are formatted recursively using the same keyword-casing and spacing rules as regular SQL. The `layout.dollar_quote.plpgsql` section additionally controls the block structure.

```yaml
layout:
  dollar_quote:
    newline_after_open: preserve    # preserve | add | remove  (\n after opening $$)
    newline_before_close: preserve  # preserve | add | remove  (\n before closing $$)

    plpgsql:
      keyword_indent: preserve      # preserve | none | indent  (DECLARE/BEGIN/END indent)
      declare_when_empty: preserve  # preserve | add | remove   (keep empty DECLARE)
      end_semicolon: preserve       # preserve | add | remove   (semicolon after final END)

      declare:
        indent: preserve            # preserve | none | indent  (DECLARE body lines)
        blank_line_before: preserve
        blank_line_after: preserve

      begin_body:
        indent: preserve
        blank_line_before: preserve
        blank_line_after: preserve

      control_flow:
        if:
          body_indent: preserve     # preserve | none | indent
          blank_line_before: preserve
          blank_line_after: preserve

        loop:
          body_indent: preserve
          blank_line_before: preserve
          blank_line_after: preserve

        case:
          simple:                        # CASE expr WHEN value THEN …
            when_indent: preserve        # preserve | none | indent
            then_break: preserve         # preserve | never | always | auto
            then_indent: preserve        # preserve | none | indent
            body_break: preserve         # preserve | never | always | auto
            body_indent: preserve        # preserve | none | indent
            blank_line_before: preserve
            blank_line_after: preserve
          searched:                      # CASE WHEN condition THEN …
            when_indent: preserve
            then_break: preserve
            then_indent: preserve
            body_break: preserve
            body_indent: preserve
            blank_line_before: preserve
            blank_line_after: preserve
```

#### IF blocks

| Setting | Effect |
|---|---|
| `body_indent: indent` | Body normalised to IF depth + 1 |
| `blank_line_before: add` | Blank line before each IF / ELSIF / ELSE / END IF keyword |
| `blank_line_after: add` | Blank line after THEN / ELSE (before body) |

```sql
-- body_indent: indent
BEGIN
   IF x = 1 THEN
      y := 'a';
   ELSIF x = 2 THEN
      y := 'b';
   ELSE
      y := 'c';
   END IF;
END
```

#### LOOP blocks

Same settings as IF. Covers plain `LOOP`, `FOR i IN … LOOP`, `FOR rec IN SELECT … LOOP`, and `WHILE cond LOOP`. The loop header (everything before the `LOOP` keyword) is preserved verbatim.

```sql
-- loop.body_indent: indent, loop.blank_line_after: add
BEGIN
   FOR i IN 1..10 LOOP

      total := total + i;

   END LOOP;
END
```

#### CASE statements

Two independent configs — `simple` (value-based) and `searched` (condition-based):

| Setting | Values | Effect |
|---|---|---|
| `when_indent` | `preserve` | WHEN/ELSE at same depth as CASE |
| | `indent` | WHEN/ELSE one level deeper than CASE |
| `then_break` | `never` | WHEN val THEN stays on one line |
| | `always` | THEN on its own line |
| | `auto` | Breaks if `WHEN val THEN body` exceeds `line_length` |
| `then_indent` | `preserve` | THEN at same depth as WHEN |
| | `indent` | THEN one level deeper than WHEN |
| `body_break` | `never` | Body on same line as THEN |
| | `always` | Body on new line after THEN |
| | `auto` | Same line for single stmt, new line for multiple stmts |
| `body_indent` | `indent` | Body normalised to target depth |

**Compact style** (`then_break: never`, `body_break: never`, `when_indent: indent`):

```sql
BEGIN
   CASE status
      WHEN 'active' THEN result := 1;
      WHEN 'pending' THEN result := 0;
      ELSE result := -1;
   END CASE;
END
```

**Expanded style** (`then_break: always`, `then_indent: indent`, `body_break: always`, `body_indent: indent`):

```sql
BEGIN
   CASE
   WHEN status = 'active'
      THEN
         result := 1;
   WHEN status = 'pending'
      THEN
         result := 0;
   ELSE
      result := -1;
   END CASE;
END
```

**Auto style** (`then_break: auto`, `body_break: auto`) collapses short branches and expands long ones, using `line_length` as the threshold.

The `control_flow` settings apply equally to the EXCEPTION section body — IF, LOOP, and CASE blocks inside exception handlers are formatted with the same rules.

## Migrating from 0.1.x

### `layout.content.break: first_inline` removed

`first_inline` was a combined break-and-placement mode. In 0.2.0 it is replaced by two orthogonal settings:

```yaml
# 0.1.x
layout:
  content:
    break: first_inline

# 0.2.x equivalent
layout:
  content:
    break: auto          # or always / never — your original intent
    first_item: inline
```

Per-section overrides follow the same pattern:

```yaml
# 0.1.x
layout:
  content:
    select_list: { break: first_inline }

# 0.2.x
layout:
  content:
    select_list: { break: auto, first_item: inline }
```

## Troubleshooting

**Config file not found**

pg_procrustes walks up from the *current working directory*, not from the location of the SQL file. Run the tool from the project root, or use `-c path/to/.pg_procrustes.yaml` to point at the config explicitly.

**Dollar-quoted block is not formatted**

Only blocks tagged as `$$ … $$ LANGUAGE plpgsql` (or `LANGUAGE plpgsql` anywhere on the same `CREATE FUNCTION/PROCEDURE` statement) are treated as PL/pgSQL and reformatted recursively. Other dollar-quoted strings (`$$plain text$$`, `$tag$…$tag$`) are passed through unchanged.

**Quoted identifiers are not removed**

`quoted_identifiers: remove_safe` only removes double quotes when the identifier is all-lowercase, contains no spaces or special characters, and is not a PostgreSQL reserved keyword. Identifiers that fail any of these checks keep their quotes.

**Formatter returns an error on valid SQL**

pg_procrustes uses the native PostgreSQL parser (`libpg_query`). SQL that the parser rejects — including some procedural extensions or very new syntax — cannot be formatted. Please open an issue with a minimal reproducer.

**Output is not idempotent**

The formatter guarantees idempotence: running it twice on the same file should produce no further changes. If a second run changes the output, that is a bug — please open an issue.

**Performance on large files**

Parsing is done once per `Format()` call via the PostgreSQL C parser, so throughput scales linearly with file size. Files with many complex PL/pgSQL bodies are slower because each dollar-quoted block is parsed and formatted as a separate pass.

## Status

Work in progress. Here be dragons.

## License

MIT
