# AGENTS.md — pg_procrustes

Context for AI agents working on this codebase.

## What this project is

A PostgreSQL SQL formatter in Go. It reads `.sql` files (or stdin), applies style rules from `.pg_procrustes.yaml`, and writes normalized SQL back. It has two independent processing layers:

1. **Formatter** (`formatter/`) — token-level: keyword casing, identifier casing, operator normalization, PL/pgSQL control flow restructuring.
2. **Layout** (`internal/layout/`) — clause-level: line breaking, indentation, CASE expression expansion/collapsing.

The formatter runs first; layout runs on its output.

## Key files

| File | Purpose |
|---|---|
| `config/config.go` | All config structs, defaults (`defaultLayout`, `defaultFormatter`), `Validate()` |
| `formatter/formatter.go` | `formatTolerant()`/`formatParsed()` — the token loop; routes to sub-formatters |
| `formatter/syntax.go` | `Format()` / `FormatWithDiagnostics()` entry points: syntax check, `syntax_errors` policy (fail/skip/ignore), statement chunking, `SyntaxErrors` |
| `formatter/plpgsql.go` | Dollar-quote detection; `reformatPLpgSQL`; `reformatControlFlow` (event scanner); `restructurePLpgSQL` |
| `formatter/keywords.go` | Keyword and identifier classification and case rewriting |
| `formatter/operators.go` | Operator normalization (spacing, `!=`/`<>`, cast style) |
| `internal/layout/layout.go` | `Apply()` — all layout logic including `rebuildStatement`, `splitAtAndOr`, `reformatCaseExprs` |
| `formatter/*_test.go` | Formatter tests (keyword, PL/pgSQL, operators, …) |
| `internal/layout/case_test.go` | Layout CASE expression tests |
| `internal/layout/subquery.go` | `layout.subquery.break`: `flattenSubqueries` (before the clause layout) joins each outermost `(select …)` onto one line, `expandSubqueries` (after it) opens them and re-runs `applyCore` on the body one level deeper |
| `internal/layout/layout_test.go` | Layout clause/content break tests |
| `internal/lsp/server.go` | `pg_procrustes --lsp`: JSON-RPC/LSP server over stdio (initialize, full-text sync, formatting, range formatting, syntax-error diagnostics); `Serve(in, out, Options)`; tests in `server_test.go` drive it with scripted messages |
| `editors/vscode/` | VS Code extension: TypeScript client of `pg_procrustes --lsp` (`src/extension.ts`, bundled by esbuild, `npm run package` → `.vsix`); CI builds it |
| `editors/zed/` | Zed extension: Rust → WASM crate (`src/pg_procrustes.rs`, `extension.toml`) that registers `pg_procrustes` as the language server of SQL; CI runs `cargo check --locked` |
| `.pg_procrustes.yaml` | Reference config with all options and inline comments |

## Core patterns

### Config enums

Every enum option is a `type XMode string` with `const` values named `XModeValue`. Examples:
- `config.BreakMode`: `BreakPreserve | BreakNever | BreakAlways | BreakAuto`
- `config.BodyIndentMode`: `BodyIndentPreserve | BodyIndentNone | BodyIndentIndent`
- `config.CaseMode`: `CaseModeUpper | CaseModeLower | CaseModePreserve`

Adding a new enum: define the type + constants in `config.go`, add to the relevant struct field, set a default in `defaultLayout()` or `defaultFormatter()`, validate in `validate()`.

### Tokenization

Both layers use `pg_query.Scan(sql)` which returns a `[]ScanToken` with `Start`, `End`, and `Token` (type code). Extract raw text with `sql[tok.Start:tok.End]`. Token types are pg_query constants (`pg_query.Token_SQL_RESERVED_WORD`, etc.).

Internal helper `rawTok` struct used in layout: `{start, end int, lower string}`.

### Formatter: token rewrite loop

`Format()` iterates tokens left-to-right. For each token it decides the replacement string. Output is built as `strings.Builder`. The formatter does **not** change whitespace structure (that is layout's job) — it only rewrites token text.

### Formatter: PL/pgSQL event scanner

`reformatControlFlow()` is the core of PL/pgSQL formatting. It:
1. Splits the body into lines.
2. Scans each line for control-flow keywords at the start (`IF`, `ELSIF`, `ELSE`, `THEN`, `END IF`, `LOOP`, `END LOOP`, `CASE`, `WHEN`, `ELSE` (case-else), `END CASE`).
3. Emits events (`evIF`, `evTHEN`, `evLOOP`, …).
4. Accumulates lines into segments between events.
5. Applies indent and blank-line rules from config to each segment.

Depth tracking: each `evIF` / `evLOOP` / `evCASE` increments depth; corresponding `END` event decrements it. The target indent for a body line is `baseDepth + depth * indentUnit`.

**Key invariant**: loop headers (`FOR i IN 1..10`) come before the `LOOP` keyword on the same line and are part of the preceding segment — they are never restructured. Only lines between `LOOP` and `END LOOP` are body lines.

### Layout: rebuildStatement

`rebuildStatement(sql, tokens, clauses, cfg)` assembles the output for one SQL statement:
1. For each clause: decide `doBreak` (clause break) and `doContentBreak` (content break).
2. If `!doBreak`: write the original clause text, optionally running `reformatCaseExprs` on the content.
3. If `doBreak`: write the clause keyword, then write content items (split at commas or AND/OR).
4. `reformatCaseExprs` is called on every content string — verbatim path and broken path both apply it.

### Layout: CASE reformatting

`reformatCaseExprs(item, baseIndent, cfg, indentUnit, lineLen)`:
- Tokenizes `item` with `pg_query.Scan`.
- Scans for `CASE` at `caseDepth == 0`, `parenDepth == 0`.
- Finds matching `END` with `findCaseEnd` (tracks nested CASE depth).
- Rewrites the `CASE…END` block via `flattenCaseExpr` or `expandCaseExpr`.
- Splices the rewritten block back into the item string.
- Recursively processes any remaining content after the END.

`expandCaseExpr` distinguishes simple form (has tokens between CASE and first WHEN) from searched form (CASE immediately followed by WHEN). Both produce `\n<baseIndent+indentUnit>WHEN … THEN …` lines and `\n<baseIndent>END`.

### splitAtAndOr

Splits a WHERE/HAVING/JOIN content string at top-level AND/OR. Tracks both `parenDepth` (increments on `(`, decrements on `)`) and `caseDepth` (increments on `CASE`, decrements on `END`). Only splits when both are 0. This prevents AND/OR inside CASE conditions from being treated as condition separators.

## Test approach

### Formatter tests

- Each feature has its own `_test.go` file.
- Tests use `mustFormat(t, input, cfg)` helper.
- Always test: `preserve` (no change), the active value(s), idempotence (two passes produce identical output).
- PL/pgSQL tests are in `formatter/plpgsql_test.go` and cover IF/ELSIF/ELSE, LOOP, FOR, WHILE, CASE (simple and searched), nested blocks, EXCEPTION section.

### Layout tests

- `internal/layout/layout_test.go` — clause break, content break, union blank lines, indent normalization.
- `internal/layout/case_test.go` — SQL CASE expressions: preserve, never, always, auto, indent modes, CASE in WHERE, AND/OR inside CASE, nested CASE, combined clause+CASE break.
- All CASE tests check idempotence.

Helper in case tests:
```go
func caseCfg(brk config.BreakMode, indent config.BodyIndentMode, lineLen int) *config.LayoutConfig
func assertLayoutFormat(t *testing.T, sql string, cfg *config.LayoutConfig, want string)
```

## Important invariants

1. **Idempotence**: applying the same config twice must produce the same output. Break this and the tool is unusable in CI.
2. **Depth formula**: PL/pgSQL body depth = `startDepth + nesting * indentUnit`. Off-by-one here produces wrong indentation for every nested block.
3. **Case depth in splitAtAndOr**: forgetting `caseDepth` causes AND/OR inside CASE to be incorrectly treated as WHERE condition separators.
4. **reformatCaseExprs in verbatim path**: when clauses do not break, content is written via `originalText()`. CASE reformatting must also be applied in this path — not only in the clause-breaking path.
5. **IsNoop()**: must return true only when the config is a true no-op (all values are `preserve`). Used to skip layout entirely when nothing would change. Add new fields here when adding new layout options.

## What to be careful about

- **Don't alter whitespace in the formatter layer** — the formatter produces token text, not layout. Whitespace changes (newlines, indentation) belong in `layout.go`.
- **Don't call `reformatControlFlow` on plain SQL** — it expects PL/pgSQL body content (the text between `$$` delimiters). The `formatDollarQuoted` function already selects the right code path.
- **Dollar-quote delimiter detection**: `formatDollarQuoted` finds the closing delimiter with `strings.Index(s[1:], "$")` to handle named delimiters like `$func$`. Don't change this to a simple `$$` search.
- **PL/pgSQL keyword scanner**: keyword detection uses `strings.HasPrefix(trimmedLine, "KEYWORD")` after uppercasing. Adding a new keyword: ensure it doesn't prefix-match another valid keyword (e.g., `END` must check for `END IF` before `END`).
- **CASE statement vs CASE expression**: PL/pgSQL `CASE` (a statement, ends with `END CASE;`) is handled in `formatter/plpgsql.go`. SQL `CASE` expressions (end with `END`) are handled in `layout/layout.go` via `reformatCaseExprs`. They are completely separate code paths.

## Formatter internals — detailed code map

### rawToken and token scanning

```go
type rawToken struct {
    start, end int           // byte offsets in source SQL
    text       string        // raw text: sql[start:end]
    kind       pg_query.KeywordKind
}
```

`pg_query.Scan(sql)` → `[]ScanToken`. Converted to `[]rawToken` in `Format()`. `kind` values:
`RESERVED_KEYWORD`, `UNRESERVED_KEYWORD`, `COL_NAME_KEYWORD`, `TYPE_FUNC_NAME_KEYWORD`, or 0 (non-keyword).

### tokenClass and classification priority

Every token gets a `tokenClass` in `classifyToken()` (`keywords.go`). Priority (first match wins):

1. `classSchema` / `classTable` / `classFunction*` — byte offset in the corresponding `pos.*` map
2. `classAlias` — offset in `pos.aliases`
3. `classColumn` — offset in `pos.columns`
4. `classDataType` — name in `dataTypeNames` map AND not a keyword-override case
5. `classLiteral` — name in `literalNames` (`true`, `false`, `null`, `unknown`)
6. `classOperator` — name in `operatorKeywords` (`and`, `or`, `not`, `in`, …)
7. `classReservedKeyword` — `kind == RESERVED_KEYWORD`
8. `classKeyword` — any other keyword kind (UNRESERVED, COL_NAME, TYPE_FUNC)
9. `classOther` — punctuation, numbers, symbolic operators

**Trap**: names like `event`, `type`, `status` are `UNRESERVED_KEYWORD` in pg_query, so they fall through to `classKeyword` and get uppercased unless their byte offset appears in `pos.columns`. Always register conflict-target columns and SET assignment targets explicitly.

### astPositions — the pre-scan map

Populated by `collectASTPositions(sql)` before the token rewrite loop. Maps byte offsets → category.

```go
type astPositions struct {
    schemas, tables, functions, conditionalFunctions, systemFunctions map[int]bool
    aliases, columns                                                  map[int]bool
    pendingAliases    []pendingAlias  // SELECT aliases resolved post-scan
    sortByDefaultLocs []int           // ORDER BY ordinal positions (not identifiers)
    castFuncLocs      []int           // CAST(x AS t) keyword positions
    sql               string
}
```

### pendingAliases — two-pass alias resolution

`ResTarget.Location` in pg_query points to the expression START (e.g., the `1` in `1+2 AS total`), not to the alias. So:

1. `walkNode()` adds each `ResTarget` with a non-empty `.Name` to `pos.pendingAliases`.
2. After tree walk, `pos.resolveAliases(tokens)` calls `findAliasToken()` per alias.
3. `findAliasToken()` scans forward from `exprStart` for the token with the alias name that is followed by a clause boundary, `,`, `)`, or EOF.

**Trap**: `walkSetTargetList()` must be used (not `walkNode()`) for UPDATE SET and ON CONFLICT SET lists. In those lists, `ResTarget.Name` is the assignment target column, NOT an alias — it must go to `pos.columns`, not `pendingAliases`. Using `walkNode()` on a SET list causes the column to be registered as an alias, and `findAliasToken` then inserts a spurious `AS` keyword.

### walkNode vs walkSetTargetList

- `walkNode(node, pos)` — generic recursive tree walker. For `Node_ResTarget` always appends to `pendingAliases`. Use for SELECT list.
- `walkSetTargetList(nodes, pos)` — SET-list walker. Registers `ResTarget.Name` directly to `pos.columns` at `r.Location`, then recurses into the value expression with `walkNode`. Use for UPDATE and ON CONFLICT SET clauses.

### resolveOnConflictCols

ON CONFLICT conflict-target columns (`ON CONFLICT (col1, col2) DO UPDATE`) appear as `IndexElem` nodes which have NO `Location` field. The AST walk cannot register them.

`resolveOnConflictCols(tokens, pos)` does a token-stream scan: finds `ON CONFLICT (` sequences, then scans depth-1 to register each depth-1 comma-separated token as `pos.columns`. Called after `pos.resolveAliases(tokens)` in `Format()`.

### restructurePLpgSQL and trimHWS

`restructurePLpgSQL` in `plpgsql.go` rewrites DECLARE/BEGIN/EXCEPTION/END keyword indentation. The `trimHWS` closure strips trailing horizontal whitespace from the `strings.Builder` before writing each keyword; without it, whitespace left by `indentBodyLines` accumulates and the keyword gets extra indentation on every pass (idempotence failure).

```go
var sb strings.Builder
trimHWS := func() {
    s := sb.String(); i := len(s)
    for i > 0 && (s[i-1]==' ' || s[i-1]=='\t') { i-- }
    if i < len(s) { sb.Reset(); sb.Grow(i+64); sb.WriteString(s[:i]) }
}
// Usage: trimHWS(); sb.WriteString(kwIndent); sb.WriteString(body[kwStart:kwEnd])
```

### `-- pg_procrustes: off` / `on` (formatter/verbatim.go)

`FormatWithDiagnostics` swaps every off…on region for a one-line placeholder comment (`-- __pgp_verbatim_N__`) before the whole pipeline runs and puts the original lines back afterwards (error positions are mapped back). Because it works on text, it also covers directives inside `$$` bodies. A placeholder that is not found in the output is an error, never silently dropped text.

### Format() entry points

`Format()` (`syntax.go`) is the public entry. It parses the input once; if it parses it calls `formatParsed()` (the pipeline below, reusing that parse). If not, it splits the script into statements (`splitStatements`, psql meta-commands become verbatim chunks), parses each, and applies `syntax_errors`: `fail` returns `SyntaxErrors`, `skip` formats valid chunks and copies the rest verbatim (whole-text settings — blank_lines, semicolons add, trailing_newline — are applied once at the end via `finishWholeText`), `ignore` skips the check. Internal callers that format fragments (PL/pgSQL bodies, extracted statements) must call `formatTolerant()`, never `Format()`.

### Format pipeline — full sequence

```
Format(sql, cfg):
  1. pg_query.Scan(sql)          → []rawToken
  2. collectASTPositions(sql)    → *astPositions (AST tree walk)
  3. pos.resolveAliases(tokens)  → populate pos.aliases from pendingAliases
  4. resolveOnConflictCols(...)  → populate pos.columns for ON CONFLICT targets
  5. main token loop             → rewrite each token: casing, type normalization,
                                   paren spacing, semicolons, operators, aliases,
                                   dollar-quoted bodies (inline PL/pgSQL reformat)
  6. layout.Apply(result, cfg)  → clause/content breaking, CASE reformatting,
                                   indent normalization, paren indent
```

Dollar-quoted blocks are processed inside step 5: the body is extracted, `reformatPLpgSQL()` is called (which calls `reformatControlFlow()` and `restructurePLpgSQL()`), and the result is spliced back before the token loop continues after the closing `$$`.

### Multi-token type sequences

`multiTokenTypes` maps the first token of a multi-word type (e.g., `character`) to `[]multiTokenSeq`. Each entry has `next []string` (follow-on tokens, lowercase), `longForm`, and `shortForm`. The token loop does greedy longest-match. When matched, follow-on tokens are consumed (skipped) and a single replacement string is emitted. Type rewriting MUST happen in the main token loop because it consumes future tokens.

### Adding a new formatter option

1. Define `type XMode string` + constants in `config/config.go`.
2. Add the field to the relevant config struct and set a default in `defaultFormatter()` / `defaultLayout()`.
3. Add validation in `validate()` if needed.
4. Consume the value in the token loop (formatter) or `layout.go` (layout).
5. Update `layout.IsNoop()` if the new option lives in `LayoutConfig`.
6. Add a golden test case in `formatter/testdata/<name>/` with `input.sql` + optional `config.yaml` + `want.sql`.

## Templates

`templates/*.yaml` are ordinary config files shipped with the program (not embedded in the binary): they are installed next to it (`make install`, release archives, Homebrew formula) and referenced from a config as `template: name` (`config/template.go` resolves names, files, chains and merging). The Config Builder is generated from that folder: `make builder-templates` (also run by `make build`) writes `docs/config-builder-templates.js`; CI runs `go run scripts/gen_builder_templates.go -check`. To add or remove a template just add or remove a file — header comments `# title:`, `# title.<lang>:` and `# description:` give its label. `pg_procrustes --install-templates` (`config/install.go`) downloads the release's `templates.zip` (built by `.github/workflows/release.yml` from `templates/*.yaml`) for users who installed with `go install`. `TestShippedTemplatesLoad` and `TestShippedTemplatesFormatSafely` cover every file in the folder.

## Changelog discipline

Every user-visible change (code, docs, or website) gets an entry in `CHANGELOG.md` under `[Unreleased]`. When a feature or fix touches multiple turns/edits, keep it to **one bullet per logical change**, not one bullet per edit:

- Before adding a new bullet, check whether `[Unreleased]` already has an entry for the same thing (same file/feature). If it does, edit that entry in place to describe the complete, final behavior instead of appending a new one.
- A bullet should read as the net effect a user/release-note reader would care about, not a diary of intermediate edits (e.g. not "added X" followed later by "fixed X" followed later by "restyled X" as three separate lines — fold them into one bullet describing the finished feature).
- Keep the existing Keep-a-Changelog category structure (`Added` / `Changed` / `Fixed` / `Security` / …) and match the terse, specific style of past entries (name the concrete file/option/flag, not just "improved X").

## Checking against real files

`go run scripts/corpus_check.go [-fp] [-t postgresql,ai,modern,none | -t some.yaml,…] [-o report.txt] <dir>` formats every `.sql` file below `<dir>` with the given templates (or config files) and reports output that does not parse (including the SQL inside PL/pgSQL function bodies, which the SQL parser sees only as a string), changes the text of a string constant (`string-changed`; a fingerprint ignores constant values), is not idempotent (`-fp`: or changes the parse tree) or — for `none`, the empty config — differs from the input. Nothing is written to `<dir>`. Run it on a large real schema before a release.

`go run scripts/body_diff.go <config.yaml> <dir>` is a differential check for PL/pgSQL bodies: it formats every single statement of the `.sql` files below `<dir>` at the top level and again wrapped in `DO $$ BEGIN … END $$;`, and reports statements that read differently (whitespace aside). A body is no SQL text as a whole, so anything that depends on the parse tree (aliases, column names spelled like keywords, `AS`) must be recovered by `collectBodyPositions` (formatter.go); this check finds what it misses.

`go run scripts/stmt_matrix.go <dir> <max-reports> <config.yaml>...` runs every single statement of a corpus through the given configs (bare and wrapped in `DO $$ … $$`) and reports output that does not parse or is not idempotent; single statements make small test cases.

### Systematic sweep over the options

`stmt_matrix.go` also checks that the string constants (and dollar-quoted texts that are no function body) come out exactly as they went in (`data-changed`) — the parse-tree fingerprint ignores constant values — and `stmt_matrix.go -check <config> <file.sql>` prints what is wrong with one file. To cover the option space instead of random configs:

```bash
go build -o /tmp/smx scripts/stmt_matrix.go
python3 scripts/option_sweep.py .pg_procrustes.yaml /tmp/sweep 1     # reads the options from the comments of the reference config
/tmp/smx <dir of .sql> 3 $(for f in /tmp/sweep/pair/*.yaml; do echo fp:$f; done)   # every pair of option values (~50 configs)
/tmp/smx <dir of .sql> 3 $(for f in /tmp/sweep/ofat/*.yaml; do echo $f; done)      # every single option value (~300 configs, no tree check needed)
python3 scripts/matrix_triage.py /tmp/smx <dir> <failing configs…>                  # shrinks each failure to the options that matter
```

Rebuild `smx` after every code change (it links the formatter). Fix a failure first, add a regression test that fails without the fix, then repeat until the sweep is clean over several corpora (a large real schema, `pg/src/test/regress`, `pg/contrib`, `pg/src/pl`).

## Running tests

```bash
go test ./... -count=1                          # all tests
go test ./formatter/... -v -run PLSQL  # PL/pgSQL subset
go test ./internal/layout/... -v -run Case      # CASE layout subset
go test ./formatter/... -run TestGolden -update  # regenerate golden files
go test ./formatter/... -run '^$' -bench=. -benchmem  # benchmarks
```

All tests must pass before committing. The idempotence tests are part of the normal test suite and will catch regressions in most formatting logic.
