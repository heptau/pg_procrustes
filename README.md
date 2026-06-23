# pg_procrustes

[![CI](https://github.com/heptau/pg_procrustes/actions/workflows/ci.yml/badge.svg)](https://github.com/heptau/pg_procrustes/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/heptau/pg_procrustes)](https://goreportcard.com/report/github.com/heptau/pg_procrustes)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A fast, flexible PostgreSQL formatter in Go. Driven by the native PostgreSQL parser, it understands your code like the DB itself. You design the Procrustean bed via rich config, and it shapes your queries with precision. 🛏️📐

## The name

In Greek mythology, Procrustes was a bandit who operated an iron bed on the road to Athens. He invited travellers to spend the night — then made sure they fit the bed perfectly. If a guest was too tall, he cut off their legs. If too short, he stretched them until they matched. Either way, everyone fit.

pg_procrustes works the same way. You define the bed — the formatting rules — via `.procrustes.yaml`. Your SQL will fit. Whether it likes it or not.

## Installation

```bash
go install github.com/heptau/pg_procrustes@latest
```

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
```

## Configuration

pg_procrustes looks for `.procrustes.yaml` starting in the current directory and walking up to the filesystem root. Specify a custom path with `-c path/to/config.yaml`. All settings default to `preserve` (no change).

> **Note:** The `.procrustes.yaml` in this repository is the configuration used to format pg_procrustes' own source files — not a default template. Copy and adjust it for your own project.

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
`functions`, `conditional_functions`, `system_functions`, `aliases`, and
`columns`.

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

operator_spacing: normalize  # preserve | normalize
                             #   normalize: exactly one space around = != <> < > <= >= ||

blank_lines: preserve        # preserve | max_3 | max_2 | max_1

paren_spacing: remove        # preserve | add | remove

quoted_identifiers: remove_safe  # preserve | remove_safe

schema_qualification: preserve   # preserve | remove_public

cast_style: preserve         # preserve | double_colon | cast_function

order_asc: preserve          # preserve | explicit | implicit

not_in: preserve             # preserve | not_in | not_equals_any
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
    # per-section overrides:
    # select_list, where_conds, join_on, group_list, order_list, set_list, values_list
    # Example:
    # where_conds: { break: always }

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

```sql
SELECT
  u.id,
  u.name,
  u.email
FROM users u
WHERE
  u.active = TRUE
  AND u.account_type = 'premium'
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

## Status

Work in progress. Here be dragons.

## License

MIT
