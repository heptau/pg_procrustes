# AGENTS.md — pg_procrustes

Context for AI agents working on this codebase.

## What this project is

A PostgreSQL SQL formatter in Go. It reads `.sql` files (or stdin), applies style rules from `.procrustes.yaml`, and writes normalized SQL back. It has two independent processing layers:

1. **Formatter** (`internal/formatter/`) — token-level: keyword casing, identifier casing, operator normalization, PL/pgSQL control flow restructuring.
2. **Layout** (`internal/layout/`) — clause-level: line breaking, indentation, CASE expression expansion/collapsing.

The formatter runs first; layout runs on its output.

## Key files

| File | Purpose |
|---|---|
| `internal/config/config.go` | All config structs, defaults (`defaultLayout`, `defaultFormatter`), `Validate()` |
| `internal/formatter/formatter.go` | `Format()` entry point; token loop; routes to sub-formatters |
| `internal/formatter/plpgsql.go` | Dollar-quote detection; `reformatPLpgSQL`; `reformatControlFlow` (event scanner); `restructurePLpgSQL` |
| `internal/formatter/keywords.go` | Keyword and identifier classification and case rewriting |
| `internal/formatter/operators.go` | Operator normalization (spacing, `!=`/`<>`, cast style) |
| `internal/layout/layout.go` | `Apply()` — all layout logic including `rebuildStatement`, `splitAtAndOr`, `reformatCaseExprs` |
| `internal/formatter/*_test.go` | Formatter tests (keyword, PL/pgSQL, operators, …) |
| `internal/layout/case_test.go` | Layout CASE expression tests |
| `internal/layout/layout_test.go` | Layout clause/content break tests |
| `.procrustes.yaml` | Reference config with all options and inline comments |

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
- PL/pgSQL tests are in `internal/formatter/plpgsql_test.go` and cover IF/ELSIF/ELSE, LOOP, FOR, WHILE, CASE (simple and searched), nested blocks, EXCEPTION section.

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

## Running tests

```bash
go test ./... -count=1                          # all tests
go test ./internal/formatter/... -v -run PLSQL  # PL/pgSQL subset
go test ./internal/layout/... -v -run Case      # CASE layout subset
```

All tests must pass before committing. The idempotence tests are part of the normal test suite and will catch regressions in most formatting logic.
