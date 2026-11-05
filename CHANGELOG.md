# Changelog

All notable changes to pg_procrustes are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **Templates:** a config file can start from other config files with `template: name` (an installed template), `template: path/to/file.yaml` (relative to the file that names it) or a list of them, applied in order; the file's own settings win. Maps merge key by key, lists and scalars are replaced, templates may use `template:` themselves (cycles are errors). Templates are ordinary config files in the new `templates/` folder (`postgresql` ≈ what PostgreSQL itself prints, `ai` = lean for LLM prompts, `modern` = lower-case keywords for syntax highlighting, one item per line with leading commas, no alignment padding: friendly to git, wide screens and AI); they are not embedded in the binary but installed next to it — release archives and the Homebrew formula carry `templates/`, `make install PREFIX=…` copies it to `PREFIX/share/pg_procrustes/templates`, and lookup also covers `$PG_PROCRUSTES_TEMPLATES` and the user's config directory. New flags `--print-config` (effective configuration with every option, plus the files it was built from) and `--list-templates`. Library users get `Config.Sources`, `config.TemplateDirs`, `config.ListTemplates` and `Config.YAML`
- `layout.subquery.break: preserve | never | always | auto` (default `preserve`): lays out parenthesised subqueries and CTE bodies — `(select …)`, `(with …)`, `(values …)` — with the same rules one level deeper: `(` stays on its line, the body goes below it, `)` gets a line of its own (`never` joins them onto one line, `auto` opens only those on a line longer than `line_length`; a subquery with a `--` comment is left alone). With `always`, a `content.break: auto` list that holds a subquery is broken one item per line. Nested subqueries, function bodies (including `exists (select …)` in an `if` / `elsif` / `while` condition) and leading commas are handled; the `modern` template uses it
- `WINDOW name AS (…)` is a clause of its own: `layout.clauses.window` and `layout.content.window_list` (inherit the general settings) put it on its own line and one definition per line, like `group by`
- `statement_break: preserve | always` (default `preserve`): `always` starts every statement that shares a line with the previous one (`select 1; select 2;`) on a line of its own; a comment on the line of a `;` stays with its statement and blank lines are untouched (`blank_lines_between_statements` also separates statements and takes precedence)
- `none` for the alignment options `comments.align_trailing`, `layout.content.align_aliases` and `layout.content.align_set`: removes the padding and leaves exactly one space before the trailing `--` comment / `AS` / `=` (`layout.ddl.create_table.align` already had `none`)
- `preserve` for `layout.content.comma_position`, `logical_operator_position`, `join_on_position` and `layout.content.align`, now the **default**: the layout keeps the style the input uses (leading commas, `AND`/`OR` ending a line, `ON` on its own line, items level with the keyword) and falls back to the classic style (trailing comma, leading `AND`, `ON` inline, indented items) when the input shows none; explicit values still force a style

### Fixed

- `trailing_newline: add` no longer puts the `;` of a statement inside a function body on a line of its own (the per-statement re-format treated the statement as a whole file)
- Expanding a `CASE` (`layout.case.break`) no longer capitalises `WHEN`/`THEN`/`ELSE`/`END` (`When`, `End`); they keep the case the keyword rules gave them
- `trailing_newline: strip` no longer removes the line break before the closing `$$` of a function body (`end;$$`); whole-file settings are not applied to the edges of dollar-quoted bodies
- `comma_spacing: compact` now also holds when the layout re-joins lines (`select a,\n b` gave `a, b` on the first pass and `a,b` on the second; same inside function bodies)
- `layout.content.join_on_position: inline` now really puts `ON` on the table's line: when the input had `ON` on its own line, the output kept the line break and a stray indent (`join\n  b\n   on …`); it is now `join\n  b on …` (a line comment right before `ON` still keeps the break)

### Changed

- Broken JOINs keep the table and its condition on the keyword line (`join b on a = b`, `join b using (id)`, with further `AND`/`OR` below) — also with `join_on_position: inline`; an explicit `layout.content.join_on.first_item: break` still puts them on the next line. `JOIN … USING (…)` is no longer split into a clause of its own (only `DELETE … USING` and `MERGE … USING` are)
- `layout.indent.size` now defaults to 2 spaces (was 3); set `size: 3` to keep the old indent. The config builder's downloaded/previewed file starts with a `# pg_procrustes vX.Y.Z configuration — https://pg_procrustes.80.cz/` comment line (the version is bumped by `make prepare-release`)
- `layout.content.when_actions.break` now defaults to `preserve` (the MERGE action stays below `THEN` or on its line exactly as in the input; before, the default was `never`). It still does not inherit `layout.content.break`
- **Breaking:** an empty `.pg_procrustes.yaml` (or no config file) is now a true no-op — every rule defaults to `preserve` (casing, `data_types.form`, `aliases.as`, `trailing_whitespace`, `inequality_op`, `operator_spacing`, `paren_spacing`, `quoted_identifiers`, `comma_spacing`), matching what README and the website already stated. Previously the built-in defaults upper-cased keywords, stripped trailing whitespace, etc.; set those options explicitly (see the reference `.pg_procrustes.yaml`) to keep the old output. New test `TestEmptyConfigIsNoop`; the formatter tests now layer their style on top of `loadStyled` (`formatter/style_test.go`)
- Config builder (`docs/config-builder.html`) help panel: the sample is shown as a single Meld-style diff — changed source lines in light red, result lines in light green, and the characters that actually changed (`s` of `Select`, the whole `SELECT`) in a darker shade; the separate sample-input block is gone, and every select now lists `preserve (default)` for the true defaults
- Config builder (`docs/config-builder.html`): the header has a template select (Empty plus every file of `templates/`, shows "Custom" once settings are edited by hand, asks before replacing existing settings) instead of a Reset button, plus one-word Load / Preview buttons with tooltips; the templates are generated from `templates/*.yaml` into `docs/config-builder-templates.js` by `make builder-templates` (run by `make build`, checked in CI) and validated against the formatter by `make builder-examples`; the preview can write a short file (`template: ai` plus the differences) and "Load" understands `template:` lines for the shipped templates; the page header stays fixed at the top (also on narrow screens, where the title no longer touches the menu button), the intro sentence is gone, and the description/sample panel is docked at the bottom of the page (description left, sample right) instead of a right-hand column, and the sample is a two-column diff (original left, output right, with horizontal and vertical scrolling kept in sync between the two panes); the navigation highlight follows scrolling again after a language switch; confirmation and error messages use in-page dialogs instead of native `confirm()`/`alert()`; "Load config file" and "Paste YAML" are merged into one "Load config" dialog (paste or load a file into the text box, review or replace it, nothing applies until Apply); the options are regrouped (new "General" group with `syntax_errors`, `line_endings`, `trailing_newline`), `blank_lines` and `blank_lines_between_statements` have distinct labels and descriptions (cap vs. exact number), the YAML preview moved into a "Preview file" dialog (copy / save / close); the settings column has a fixed width and the help column takes the rest; the "pg_procrustes defaults" button is gone and "No changes" asks before discarding existing settings; every select shows the real default value (`upper (default)`, inherited options show the inherited value); option labels, descriptions and value hints are translated into all six site languages (`docs/config-builder-i18n-<lang>.js`, loaded on demand); the help panel shows a sample input and its result for the selected value as a red/green diff, with results generated from the real formatter by `make builder-examples` (`scripts/gen_builder_examples.go`, `docs/config-builder-options.js` → `docs/config-builder-examples.js`)

## [0.3.0] — 2026-11-05

### Added

- Layout: `layout.content.wrap_expressions: preserve | wrap` breaks a list item longer than `line_length` at the commas of its first parenthesised group (function arguments, `IN` lists, rows) with one element per line and `)` on its own line; nested groups wrap recursively, `CASE`, subqueries and items with comments are left alone.
- DDL layout: `layout.ddl.create_function.params` and `.options` (`preserve | never | always | auto`) put every `CREATE FUNCTION` / `PROCEDURE` parameter on its own line (closing `)` on its own line) and every option (`RETURNS`, `LANGUAGE`, `STABLE`, `SECURITY DEFINER`, `SET …`, `AS` body) on its own line; headers with a `--` comment or a `BEGIN ATOMIC` body are left alone.
- DDL layout: `layout.ddl.create_index.break: preserve | never | always | auto` puts the `ON`, `USING`, `INCLUDE`, `NULLS [NOT] DISTINCT`, `WITH (…)`, `TABLESPACE` and `WHERE` parts of `CREATE INDEX` on their own indented lines (or collapses them to one line).
- Layout: `layout.content.align_aliases: preserve | align` pads the expressions of a broken SELECT list so every `AS` lines up, and `layout.content.align_set: preserve | align` does the same for the `=` of an `UPDATE … SET` list; items spanning several lines, with comments or `CASE` are left alone.
- `builtin_functions.case` (default empty = same as `functions`) casing for PostgreSQL built-in functions (`count`, `sum`, `date_trunc`, …) separately from user functions, with `exceptions`; `call_paren_spacing: preserve | add | remove` controls the space between a function name and `(` (`count (*)` vs `count(*)`) for function calls only.
- `blank_lines_between_statements: preserve | 0 … 10` (exactly N blank lines after each `;`, a same-line comment stays attached, comments above the next statement stay with it, psql meta-commands untouched), `line_endings: preserve | lf | crlf` and `comments.space_after_marker: preserve | add` (`--note` → `-- note`; `-----`, `--+`, `--!` and block comments untouched) / `comments.align_trailing: preserve | align` (trailing `--` comments of consecutive lines start in one column; single comments and runs with tabs are left alone; also inside function bodies)
- DDL layout: `layout.ddl.create_table.break: preserve | never | always | auto` puts every column and table constraint of `CREATE TABLE (…)` on its own line with the closing parenthesis on its own line (trailing options such as `WITH (…)` follow it), `layout.ddl.create_table.align: preserve | none | types | all` pads names and types so columns and constraints line up (table constraints are never aligned), and `layout.ddl.alter_table.break: preserve | never | always | auto` puts each `ALTER TABLE` action on its own indented line (`auto`: several actions or too long). `content.comma_position`, comments (a trailing `-- note` stays with its column) and the indent settings apply; `never` backs off when a line comment is inside; `ALTER TABLE ALL IN TABLESPACE` and `CREATE TABLE … PARTITION OF` are left alone. New golden case `formatter/testdata/ddl_layout`. Also: a `CREATE`/`ALTER` statement without a closing `;` at the end of a script is now processed by the layout like any other
- Layout styles for broken lists and conditions: `layout.content.comma_position: end | start` (leading commas, with the items kept aligned, also for `INSERT` column lists and `VALUES` rows), `layout.content.logical_operator_position: start | end` (`AND`/`OR` opening or ending the line; a comment after the operator stays on its line) and `layout.content.join_on_position: inline | newline` (`ON` and its conditions on indented lines below `JOIN t`; `clauses.join.align: indent` indents `JOIN` under `FROM`). Defaults keep the previous output
- `syntax_errors: fail | skip | ignore` and `--syntax-errors <mode>`: **breaking default** — input that PostgreSQL cannot parse is no longer formatted at token level. `fail` (default) reports every bad statement as `file:line:col: error: …` with the source line and a caret, writes nothing and exits 1 (also in `--check`, `--diff`, `-w` and stdin mode); `skip` formats the statements that parse, leaves the others exactly as written and warns on stderr; `ignore` restores the previous unchecked behaviour. psql meta-commands (`\set`, `\connect`, …) at the start of a statement are kept verbatim and never count as errors; a `BEGIN ATOMIC … END` body is one statement; an unterminated string/comment/dollar quote is fatal in every mode except `ignore`. Library users get `formatter.FormatWithDiagnostics` and the `SyntaxErrors` error type (line, column, message per statement). The syntax check reuses the parse the formatter needs anyway, so valid input costs nothing extra (benchmarks unchanged)
- `capitalize` as a fourth value for every `case:` setting (alongside `upper | lower | preserve`): the first letter of each word is upper-cased and the rest lower-cased (`order_items` → `Order_items`, `character varying` → `Character Varying`, `GROUP BY` → `Group By`); underscores are never removed so identifiers keep their names, quoted identifiers are still never re-cased, and `exceptions` still win. Documented in README, reference config, website and config builder
- Layout support for `MERGE` (PostgreSQL 15+): `MERGE INTO target`, `USING source`, `ON condition` (split at `AND`/`OR`) and every `WHEN [NOT] MATCHED … THEN …` action get their own lines, with the target and each action kept on the keyword line by default; `CASE WHEN` inside an action is not mistaken for a `WHEN` clause, and `MERGE` is recognised inside PL/pgSQL bodies. Separate settings: `layout.clauses.merge` / `merge_on` / `when` (`break`, `align`) and `layout.content.merge_on` (`break`, `first_item`) and `layout.content.when_actions` (`break: always` puts the action on its own line below `THEN`; defaults to `never` and does not inherit `content.break`). New golden case `formatter/testdata/merge`
- Test matrix `formatter/matrix_test.go` formats every golden input plus extra statements under 20 config variants and checks parseability, unchanged AST fingerprint and idempotence; `FuzzFormat` (`go test ./formatter -run '^$' -fuzz FuzzFormat`) does the same for random parseable SQL, with regressions kept in `formatter/testdata/fuzz/`
- New `arithmetic_spacing: preserve | normalize | compact` (default `preserve`, i.e. no change) formats spaces around binary `+ - * / % ^`; only operators proven binary by their neighbours are touched, so unary signs (`x = -1`), wildcards (`SELECT *`, `count(*)`, `t.*`), `%TYPE` and `a - -1` (never `a--1`) stay safe. Also fixes the `operator_spacing` validation message to list `compact`
- Language switcher (Auto / English / Čeština / Español / Deutsch / Français / Português / Italiano) and theme switcher (Auto / Light / Dark) on the documentation website — fixed top-right icon buttons (backdrop-blurred for readability over any background) with a dropdown menu, matching in position and style on both `index.html` and `config-builder.html`; the choice is persisted in `localStorage` and synced across both pages. `index.html`'s content and `config-builder.html`'s static chrome (header, buttons, nav, modal) are fully translated to Czech, Spanish, German, French, Portuguese, and Italian; the interactive option catalog on the config builder remains English-only
- JSON-LD structured data, Open Graph/Twitter preview images, and `sitemap.xml` `lastmod` dates for `config-builder.html`, matching `index.html`
- Sponsor (heart) and "Star on GitHub" (star) icon buttons in the top-right header of `index.html` and `config-builder.html`, next to the language and theme switchers, with translated tooltips
- `docs/llms-full.txt` — the complete README plus the fully commented reference `.pg_procrustes.yaml` in one file for LLMs, generated by `make llms` (`scripts/gen_llms_full.sh`) and listed in `sitemap.xml`

### Changed

- Performance: `aliases.as: remove` no longer runs the parser for every alias — only keyword aliases are checked (and cached by name), cutting its overhead from +17% to +3% over `preserve` on the CTE/window fixture (444 → 389 µs/op); dollar-quoted PL/pgSQL bodies skip the trailing-whitespace scan when there is nothing to strip, 333 → 268 µs/op (−20%) and 4445 → 3003 allocs/op on `plpgsql_full`. New benchmarks `BenchmarkAliases{Preserve,Add,Remove}` in `formatter/bench_test.go`
- `docs/llms.txt` rewritten to follow the llmstxt.org structure: current CLI flags and config keys (adds `trailing_newline`, `inline_spacing`, `cast_style`, `order_asc`, `not_in`, `paren_indent`), editor integrations, and a link to `llms-full.txt`
- Documentation website footer now reads "© 2026 Zbyněk Vanžura" (linking to https://www.80.cz/) with MIT License, GitHub, and Releases links; the sidebar GitHub and llms.txt links and the footer llms.txt/Sitemap/Security links were removed as noise for human readers

### Fixed

- Formatter: function parameter names that are also keywords (`name`, `type`, `event`, `label`) in `CREATE FUNCTION f(name text) RETURNS TABLE(name text)` are no longer upper-cased; this changes the output of existing files that use such names.
- Layout: the `WITH [NO] DATA` / `WITH [CASCADED | LOCAL] CHECK OPTION` trailer of `CREATE [MATERIALIZED] VIEW` and `CREATE TABLE AS` is its own clause instead of being glued to the last `WHERE` condition.
- Formatter: plain column names in `CREATE INDEX … (name) INCLUDE (name)` and CTE names such as `WITH data AS` are no longer upper-cased as keywords.
- PL/pgSQL: `RAISE EXCEPTION` is no longer mistaken for the `EXCEPTION` section when `dollar_quote.plpgsql` restructuring is active (a statement wrapped as `RAISE` / `EXCEPTION '…'` was fused into `raiseexception`); trailing whitespace left behind by the blank-line and DECLARE/BEGIN settings inside dollar-quoted bodies is stripped, so those settings are idempotent
- `not_in: not_equals_all` turned `x NOT IN ('a', 'b')` into `x <> ALL ('a', 'b')`, which PostgreSQL rejects; it now writes `x <> ALL (ARRAY['a', 'b'])` (subqueries are unchanged). `not_in: not_in` turned `x <> ALL (ARRAY['a', 'b'])` into `x NOT IN (ARRAY['a', 'b'])`, which compares `x` with a single array value instead of its elements; it now writes `x NOT IN ('a', 'b')`, converts `<> ALL (SELECT …)`, and leaves `<> ALL (array_column)` alone because no `NOT IN` form exists. Found by the new syntax check (the golden files had recorded the broken output)
- CRLF files were silently converted to LF (the trailing-whitespace pass removed the `\r`); the new `line_endings: preserve` (default) keeps the input's style, and `--check` no longer reports every CRLF file as changed
- Layout: the `AND` of `x BETWEEN a AND b` is no longer treated as a condition separator (it used to put `AND b` on a new line), and the `ON` of a `JOIN` is found at the top level only, so a joined subquery with its own `ON` no longer confuses the split
- Layout: line comments (`-- …`) no longer break the SQL in three more places — `case.break: never` / short `auto` keeps a `CASE` that contains a line comment as written instead of flattening everything after the comment into it; the closing `;` goes on its own line when the statement ends with a line comment (`… -- row two;` left the statement unterminated); and `VALUES` rows keep a same-line comment with their row, also with leading-comma style (`(1, 2) -- one` / `, (3, 4)` produced a stray comma line)
- Formatter: a quoted alias that loses its quotes (`select 1 "a"`) now gets `AS` and follows `aliases.case` in the same pass (it used to be added only on the second run); layout no longer leaves a space before a comma when it joins a line that started with a leading comma (`a , b`)
- Tests: `TestConfigMatrix` now also covers `dollar_quote` / PL/pgSQL (`keyword_indent`, `declare`, `begin_body`, `control_flow` incl. CASE, `end_semicolon`, `declare_when_empty`, boundaries), `paren_indent`, indent normalization, `blank_lines`, `semicolons`, `quoted_identifiers` and `capitalize`, with multi-line PL/pgSQL functions as inputs and a structural check of the parsed PL/pgSQL tree (`ParsePlPgSqlToJSON`, embedded SQL compared by AST fingerprint)
- Layout: per-clause and per-section overrides (`layout.clauses.<clause>.break`, `layout.content.<section>.break`) now take effect even when the global `clauses.break` / `content.break` is `preserve`; previously `IsNoop()` looked only at the global values and skipped layout entirely. Per-clause `break`/`align` values are now validated too (invalid values used to be accepted silently)
- Layout: `SET`, `USING`, `WITH (…)` and `UPDATE` are only treated as clause keywords where they are query clauses; `ALTER TABLE … SET`/`ALTER COLUMN … SET NOT NULL`/`… TYPE … USING`, `ALTER ROLE … SET`, standalone `SET`, `CREATE TABLE … WITH (…)`, `FOR UPDATE`, `ON UPDATE CASCADE`, trigger events (`BEFORE INSERT OR UPDATE`) and `ON CONFLICT … DO UPDATE` no longer get broken into bogus clauses (`DO UPDATE SET` now stays on the `ON CONFLICT` line before `SET`)
- Layout: a leading `SELECT`/`WITH` after a statement prefix (`CREATE VIEW v AS`, `CREATE TABLE t AS`, `EXPLAIN`, …) now starts on its own line when clauses are broken, instead of `CREATE VIEW v AS SELECT` with the rest of the query left-aligned underneath
- Formatter: column names in `ALTER TABLE … ALTER/RENAME/DROP/ADD COLUMN`, in `RENAME COLUMN … TO new` and in `CREATE INDEX … (cols)` are no longer upper-cased when they are unreserved keywords (`label`, `name`, `event`, …)
- Layout: a line comment inside collapsed clause content no longer swallows the code that followed it (`SELECT a, -- c b` after `clauses.break: always`), and `paren_spacing` is honored when multi-line parenthesised content is joined (no more `( SELECT … )`, which also broke idempotence)
- Layout: `clauses.align: indent` keeps the statement-opening keyword (and the first clause) at the base indent instead of shifting it, so the output is stable on re-run; multi-line items with a blank line or a leading comment no longer gain indentation on every pass
- Formatter: `aliases.as: remove` keeps `AS` for aliases PostgreSQL cannot take as bare labels (`month`, `year`, `over`, `filter`, …), which previously produced a syntax error
- Formatter: removing quotes or inserting `AS`/`INNER`/`OUTER` can no longer fuse neighbouring tokens (`select"a"` → `SELECTa`, `"a"where` → `aWHERE`); `arithmetic_spacing: compact` never merges an operator with a following symbol (`0 % +a` → `0%+a`)
- Layout (`internal/layout/layout.go`): with `clauses.break: always` and the default `union.blank_line: preserve`, `UNION`/`INTERSECT`/`EXCEPT` was glued to the preceding token (`uUNION`, `1UNION ALL`), producing invalid SQL; the source gap is now kept and the operator stays on its own line
- Formatter: `operator_spacing` now also normalizes `:=` and `=>` (PL/pgSQL assignment, named arguments); arithmetic operators remain untouched by design
- Formatter: derived-table aliases (`FROM (SELECT …) x`, `LATERAL`, `JOIN (SELECT …) y`) now honor `aliases.as` (add/remove) and `aliases.case`
- Formatter: a single-line gap before the closing dollar-quote delimiter is kept (`END $$` no longer becomes `END$$`)
- Layout: a trailing `-- comment` on a comma-separated item no longer swallows the separating comma (`a -- first,` turned `a, b` into `a b`, an alias); the comma is placed before the comment, and a comment on the same line as the comma stays with its item (stable across passes, also in `INSERT` column lists)
- Formatter: `trigger` is keyword-cased in `CREATE/DROP/ALTER ... TRIGGER` (it stays a lowercase pseudo-type after `RETURNS`)
- CLI: prints a `syntax error` warning to stderr when the input does not parse (formatting still falls back to token-level, exit status unchanged); new `formatter.SyntaxError()`
- Formatter: quoted identifiers (`"User"`, `"My Alias"`) are no longer re-cased, which changed their meaning in PostgreSQL and made output non-idempotent (`applyCase` in `formatter/formatter.go`)
- `go install` instructions in the README, on the website, and in the CI example pointed at the module root (`github.com/heptau/pg_procrustes@latest`), which has no `main` package and fails; they now use `github.com/heptau/pg_procrustes/cmd/pg_procrustes@latest`
- `config-builder.html` had no mobile layout at all (fixed sidebar plus a two-column builder grid) and was unusable below ~900px; both documentation pages now collapse to an off-canvas nav with a single-column layout on narrow viewports
- Long unbreakable content (SQL code samples, wide tables) forced the whole documentation page to scroll horizontally on narrow viewports; fixed by giving the affected flex/grid containers `min-width: 0`
- Tables on the documentation site now scroll horizontally within their own container instead of overflowing the page on narrow viewports
- Enlarged touch targets for the mobile navigation toggle and nav links

### Security

- `config-builder.html`'s YAML preview (`renderYAML()`) inserted values from a loaded or pasted `.pg_procrustes.yaml` file into `innerHTML` without escaping, allowing a crafted config file (e.g. `reserved_keywords: { case: "<img src=x onerror=...>" }`) to execute arbitrary JavaScript in the visitor's browser when loaded via "Load config file" or "Paste YAML" (DOM-based XSS). Values are now HTML-escaped before rendering

## [0.2.2] — 2026-08-07

### Added

- Neovim plugin (`lua/pg_procrustes`, `plugin/pg_procrustes.lua`, `doc/pg_procrustes.txt`) — this repository doubles as a Neovim plugin providing `pg_procrustes` as a `conform.nvim` formatter for `sql` buffers, a standalone `:PgProcrustesFormat` command, and `:checkhealth pg_procrustes`
- Documented config-only editor integration for Zed (built-in external formatter), VS Code (via the Custom Local Formatters extension), and DataGrip/JetBrains IDEs (via File Watchers) in the README and on the website
- Documented config-only editor integration for Emacs (via apheleia.el or reformatter.el) and Helix (via `languages.toml`) in the README and on the website
- Documented config-only editor integration for Sublime Text (via the Fmt package) in the README and on the website
- Documented config-only editor integration for Vim (via ALE) in the README and on the website
- New "Automation & CI" section in the README and on the website, documenting a pre-commit local hook and a GitHub Actions CI job

### Security

- Bumped indirect dependency `google.golang.org/protobuf` to v1.33.0, fixing a denial-of-service in `protojson.Unmarshal` (infinite loop on certain invalid JSON)

## [0.2.1] — 2026-07-14

### Changed

- Moved `internal/config` and `internal/formatter` to top-level `config` and `formatter` packages so they can be imported as a public API from other Go modules (e.g. `omnidb-go-server`); no behavior change

## [0.2.0] — 2026-06-30

### Added

- `exceptions` list on all casing sections (`reserved_keywords`, `keywords`, `data_types`, `literals`, `operators`, `schemas`, `tables`, `functions`, `conditional_functions`, `system_functions`, `aliases`, `columns`, `plpgsql_variables`, `plpgsql_keywords`) — individual identifiers can be pinned to a specific case regardless of the section setting
- `operator_spacing: compact` — removes all spaces around symbolic operators (`=`, `!=`, `<>`, `<`, `>`, `<=`, `>=`, `||`); complements the existing `normalize` option
- `comma_spacing: compact` — removes all spaces around commas; complements `normalize`
- `layout.content.first_item` — new orthogonal setting controlling where the first item lands when content breaks (`break` = new indented line, `inline` = keyword line); replaces the former `first_inline` break mode
- `--backup` CLI flag — saves the original file with a given extension before overwriting in-place (`-w`)
- `--out-dir` CLI flag — writes formatted files into a target directory instead of stdout

### Changed

- `layout.content.break: first_inline` removed; replaced by `layout.content.first_item: inline` combined with any non-preserve break mode — existing configs using `first_inline` must be migrated

### Fixed

- Dollar-quoted blank line handling: blank line rules now apply only inside function bodies (after `AS` or `DO`), not to every `$$`-delimited string
- `order_asc: add` scanned past a semicolon into the next statement's SELECT list and inserted spurious ASC keywords there
- `cast_style: operator` emitted a double space when a CAST expression immediately followed a symbolic operator such as `||`
- `layout.union.blank_line` split `UNION ALL` onto two lines because the keyword length was tracked as 1 (only `UNION`), leaving `ALL` as clause content
- `layout.union.blank_line` did not emit the configured blank line when no tokens appeared between a `UNION ALL` and the following `SELECT`

## [0.1.1] — 2026-06-22

### Fixed

- `peekNext` in PL/pgSQL scanner used `break` instead of `continue` when encountering `(` or `[`, causing it to stop rather than skip past the token and find the next real one
- Loop in alias detection (after `AS`) rewritten as a plain `if` — it only ever checked the immediately following token, never iterated
- Removed unused `prefixGap` field from the `statement` struct in the layout layer

## [0.1.0] — 2026-06-22

Initial release.

### Added

**Keyword & identifier casing**
- `reserved_keywords.case` — SELECT, FROM, WHERE, JOIN, …
- `keywords.case` — ANALYZE, CASCADE, VERBOSE, …
- `data_types.case` and `data_types.form` — long / short / long_no_space forms
- `literals.case` — TRUE, FALSE, NULL
- `operators.case` — AND, OR, NOT, IN, IS, LIKE, BETWEEN, …
- `schemas`, `tables`, `columns`, `functions`, `conditional_functions`, `system_functions`, `aliases` casing
- `aliases.as` — add / preserve / remove the AS keyword

**Punctuation & spacing**
- `trailing_whitespace` — strip trailing spaces
- `semicolons` — add / remove statement semicolons
- `inequality_op` — normalize to `!=` or `<>`
- `join_form` — short (JOIN) or long (INNER JOIN, LEFT OUTER JOIN, …)
- `operator_spacing` — normalize spacing around `=`, `!=`, `<>`, `<`, `>`, `<=`, `>=`, `||`
- `blank_lines` — cap consecutive blank lines (max_1 / max_2 / max_3)
- `paren_spacing` — add or remove spaces inside parentheses
- `quoted_identifiers` — remove unnecessary double-quotes (`remove_safe`)
- `schema_qualification` — strip `public.` prefix
- `cast_style` — normalize between `::` and `CAST(… AS …)`
- `order_asc` — make ASC explicit or implicit
- `not_in` — normalize between `NOT IN` and `<> ALL`

**Layout**
- `layout.clauses.break` — put each SQL clause on its own line (preserve / never / always / auto)
- `layout.clauses.align` — clause keyword alignment (same / indent)
- Per-clause overrides: `from`, `join`, `where`, `group_by`, `having`, `order_by`, `limit`, `offset`, `values`, `on_conflict`, `set`, `using`, `returning`, `with`, `exception`
- `layout.content.break` — break SELECT list at commas, WHERE/HAVING/JOIN at AND/OR
- `layout.content.align` — content alignment (same / indent)
- Per-section overrides: `select_list`, `where_conds`, `join_on`, `group_list`, `order_list`, `set_list`, `values_list`
- `layout.union.blank_line` — blank lines around UNION / INTERSECT / EXCEPT
- `layout.indent` — spaces / tab, size, normalization
- `layout.case.break` — expand or collapse SQL `CASE … END` expressions (never / always / auto)
- `layout.case.indent` — WHEN/ELSE indentation relative to CASE

**PL/pgSQL**
- Dollar-quoted bodies (`$$`, `$func$`, …) detected and formatted recursively
- `layout.dollar_quote.plpgsql.keyword_indent` — DECLARE / BEGIN / END indentation
- `layout.dollar_quote.plpgsql.declare` — DECLARE section indent and blank lines
- `layout.dollar_quote.plpgsql.begin_body` — BEGIN body indent and blank lines
- `layout.dollar_quote.plpgsql.declare_when_empty` — keep or remove empty DECLARE
- `layout.dollar_quote.plpgsql.end_semicolon` — semicolon after final END
- `control_flow.if` — IF / ELSIF / ELSE / END IF body indent and blank lines
- `control_flow.loop` — LOOP / FOR … LOOP / WHILE … LOOP / END LOOP body indent and blank lines
- `control_flow.case.simple` — CASE expr WHEN … formatting (when_indent, then_break, then_indent, body_break, body_indent)
- `control_flow.case.searched` — CASE WHEN … formatting (same settings)
- EXCEPTION section body formatted with the same control_flow rules

**CLI**
- Format files in-place (`-w`), print to stdout, or read from stdin
- `--check` mode for CI / pre-commit hooks (exit 1 if any file would change)
- `--diff` mode — print unified diff without writing
- `-v` / `--version` — print version
- `-c` — explicit config file path; auto-detection walks up from the current directory
- Glob expansion (`migrations/**/*.sql`)
