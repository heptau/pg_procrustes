# Changelog

All notable changes to pg_procrustes are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

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
