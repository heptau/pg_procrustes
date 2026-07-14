# Contributing to pg_procrustes

## Project structure

```
pg_procrustes/
├── cmd/pg_procrustes/      # CLI entry point
├── internal/
│   ├── config/
│   │   └── config.go       # All config structs, defaults, validation
│   ├── formatter/
│   │   ├── formatter.go    # Main Format() entry point
│   │   ├── keywords.go     # Keyword/identifier casing
│   │   ├── operators.go    # Operator normalization
│   │   ├── plpgsql.go      # Dollar-quote detection, PL/pgSQL reformatter
│   │   └── *_test.go
│   └── layout/
│       ├── layout.go       # Apply() — clause breaking, content breaking, CASE
│       └── *_test.go
└── .pg_procrustes.yaml        # Reference config (all options, all commented)
```

## Architecture: two-layer pipeline

```
Input SQL
    │
    ▼
formatter.Format()       ← keyword casing, operator spacing, identifier casing,
    │                       dollar-quote reformatting, PL/pgSQL control flow
    ▼
layout.Apply()           ← clause breaks, content breaks, indent normalisation,
    │                       CASE expression reformatting
    ▼
Output SQL
```

The two layers are **independent**: formatter produces clean token-level text; layout works on that output to control whitespace structure.

## Building and testing

```bash
go build ./...
go test ./... -count=1
go test ./formatter/... -run TestPLpgSQL -v   # specific subset
go test ./internal/layout/... -run TestSQLCase -v
```

## Adding a new config option

1. **Add the struct field** in `config/config.go`. Use a new named type (`type MyMode string`) with `const` values if the option is an enum.
2. **Set the default** in `defaultLayout()` or `defaultFormatter()` (usually `preserve`).
3. **Validate** in `validate()` — a `switch` checking all legal values.
4. **Implement** in the relevant layer:
   - Formatter options: edit the appropriate file under `formatter/`.
   - Layout options: edit `internal/layout/layout.go`.
5. **Add tests** covering at least: preserve (no-op), each non-trivial value, and idempotence (applying the same config twice produces the same result).
6. **Document** in `.pg_procrustes.yaml` with inline comments and in `README.md` under the relevant section.

## Formatter layer internals

`formatter.Format()` tokenizes SQL using `pg_query.Scan()` and rewrites tokens in a single left-to-right pass. Each token is classified (reserved keyword, identifier, literal, operator, …) and rewritten per config.

Dollar-quoted blocks (`$$…$$`, `$func$…$func$`) are detected by `formatDollarQuoted()`. If the content is PL/pgSQL, `reformatPLpgSQL()` applies recursive formatting plus control-flow restructuring.

### PL/pgSQL control flow

`reformatControlFlow()` is an event-based scanner. It walks lines, emits events (`evIF`, `evELSIF`, `evELSE`, `evTHEN`, `evENDIF`, `evLOOP`, `evENDLOOP`, `evCASE`, `evWHEN`, `evELSECASE`, `evENDCASE`), and groups lines into blocks. The block accumulator then applies indent and blank-line rules from config.

`restructurePLpgSQL()` applies the block structure into the final string. Depth tracking uses a counter incremented on block-open events and decremented on block-close events.

Key invariants:
- The loop header (everything before `LOOP`) is never touched — only body lines are restructured.
- Every scanner event must correspond to a unique line prefix match to avoid false positives (e.g., `END IF` must not match a column named `end_if`).
- `reformatCaseExprs()` in layout is separate from PL/pgSQL CASE — they handle different constructs.

## Layout layer internals

`layout.Apply()`:
1. Calls `pg_query.Scan()` to get a token list.
2. Groups tokens into statements (`;` boundary) and clauses (keyword boundary).
3. Decides break/no-break for each clause.
4. Calls `rebuildStatement()` to assemble the final string, applying content-break and CASE reformatting.

### CASE expression reformatting

`reformatCaseExprs(item, baseIndent, cfg, indentUnit, lineLen)` scans a content string for `CASE` tokens at depth 0 (no paren wrapping) and rewrites each `CASE…END` block:
- `break: never` → `flattenCaseExpr()`: joins all tokens with single spaces.
- `break: always` → `expandCaseExpr()`: emits each WHEN/ELSE on its own line, indented per `indent` setting.
- `break: auto` → flat if the token-joined length ≤ `lineLen`, else expand.

Simple (`CASE expr WHEN`) vs searched (`CASE WHEN`) form is detected by checking whether any tokens exist between CASE and the first WHEN.

`splitAtAndOr()` tracks both `parenDepth` and `caseDepth` so that AND/OR inside a CASE condition are not treated as WHERE/HAVING condition separators.

## Test patterns

Tests use `assertFormat(t, input, cfg, want)` in formatter tests and `assertLayoutFormat(t, sql, cfg, want)` in layout tests.

All formatter tests check idempotence by applying the same format twice:
```go
pass1 := mustFormat(t, input, cfg)
pass2 := mustFormat(t, pass1, cfg)
require.Equal(t, pass1, pass2, "not idempotent")
```

Layout tests build configs inline:
```go
cfg := &config.LayoutConfig{
    LineLength: 80,
    Indent:     config.IndentCfg{Size: 2, Type: config.IndentTypeSpaces, ...},
    Clauses:    config.ClausesCfg{Break: config.BreakAlways, Align: config.AlignSame},
    ...
}
```

## Known limitations

- The formatter makes a single left-to-right token pass; context beyond the current token is limited. Multi-word data type normalization uses a small look-ahead but is not a full parser.
- PL/pgSQL control-flow parsing is line-oriented. Statements spread across multiple lines that start with a keyword (e.g. `IF` as part of a longer identifier) can cause false event matches — the scanner uses keyword-boundary checks to mitigate this.
- CASE reformatting in layout operates on tokenized content items; it does not parse subqueries or function call bodies that may appear inside CASE branches.
