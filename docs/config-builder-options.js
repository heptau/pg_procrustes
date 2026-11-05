// Option definitions of the Config Builder (English source text + sample SQL).
// Result examples are generated from the real formatter: make builder-examples
window.CB_OPTIONS = {
 "groups": [
  {
   "id": "general",
   "title": "General",
   "opts": [
    {
     "key": "syntax_errors",
     "label": "Syntax errors",
     "type": "select",
     "def": "fail",
     "desc": "What to do with SQL that does not parse. fail reports every bad statement as file:line:col, writes nothing and exits 1. skip formats the statements that parse and leaves the others exactly as written (with a warning). ignore applies the token-level rules without checking; output is only guaranteed safe for valid SQL. psql meta-commands (\\set, \\connect) are always left as written.",
     "values": [
      [
       "fail",
       "refuse invalid SQL"
      ],
      [
       "skip",
       "leave invalid statements as written"
      ],
      [
       "ignore",
       "format without checking"
      ]
     ],
     "sample": {
      "in": "select 1 from a;\nselect from from;",
      "with": {
       "reserved_keywords.case": "upper"
      }
     }
    },
    {
     "key": "line_endings",
     "label": "Line endings",
     "type": "select",
     "def": "preserve",
     "desc": "preserve keeps CRLF files CRLF and LF files LF. lf / crlf convert the whole output, including line breaks inside multi-line string literals.",
     "values": [
      [
       "preserve",
       "keep the input style"
      ],
      [
       "lf"
      ],
      [
       "crlf"
      ]
     ],
     "example": "-- lf: SELECT 1;\\n\n-- crlf: SELECT 1;\\r\\n"
    },
    {
     "key": "trailing_newline",
     "label": "Trailing newline",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether the output ends with a newline character.",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "ensure newline at EOF"
      ],
      [
       "strip",
       "remove newline at EOF"
      ]
     ],
     "example": "-- add: output always ends with \\n\n-- strip: output never ends with \\n"
    }
   ]
  },
  {
   "id": "casing",
   "title": "Casing",
   "opts": [
    {
     "key": "reserved_keywords.case",
     "label": "Reserved keywords",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of reserved SQL keywords: SELECT, FROM, WHERE, JOIN, AND, OR, etc.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "keywords.case",
     "label": "Other keywords",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of non-reserved keywords such as BEGIN, END, DECLARE, LANGUAGE, RETURNS.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "begin;\nlock table t in share mode;\nCOMMIT;\nVacuum verbose t;\nVACUUM VERBOSE t;",
      "with": {}
     }
    },
    {
     "key": "data_types.case",
     "label": "Data types — case",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of data type names: integer, varchar, timestamp, etc.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "create table items (id integer, name Character Varying(100), created TIMESTAMP WITH TIME ZONE, price double precision, qty INTEGER, ok Boolean);",
      "with": {}
     }
    },
    {
     "key": "data_types.form",
     "label": "Data types — form",
     "type": "select",
     "def": "preserve",
     "desc": "Controls canonical form of data type names.",
     "values": [
      [
       "preserve"
      ],
      [
       "long",
       "full SQL name"
      ],
      [
       "short",
       "shortest alias"
      ],
      [
       "long_no_space",
       "long without spaces"
      ]
     ],
     "sample": {
      "in": "create table items (id int4, name varchar(100), created timestamptz, price double precision, big bigint);",
      "with": {}
     }
    },
    {
     "key": "literals.case",
     "label": "Literals (TRUE/FALSE/NULL)",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of literal keywords: TRUE, FALSE, NULL.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "operators.case",
     "label": "Operators (IS, IN, LIKE…)",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of word-form operators: IS, IN, NOT, LIKE, BETWEEN, EXISTS, ANY, ALL.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "schemas.case",
     "label": "Schema names",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of schema names in qualified references: public.users, myschema.table.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "tables.case",
     "label": "Table names",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of table (relation) names.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "functions.case",
     "label": "Function names",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of user-defined and built-in function calls.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "conditional_functions.case",
     "label": "Conditional functions (COALESCE, NULLIF…)",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of conditional functions: COALESCE, NULLIF, GREATEST, LEAST, NVL.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "builtin_functions.case",
     "label": "Built-in functions (count, sum, date_trunc…)",
     "type": "select",
     "def": "preserve",
     "desc": "Overrides the functions casing for PostgreSQL built-in functions. User-defined functions keep following functions.case.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "inherit": "functions.case",
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {
       "functions.case": "lower"
      }
     }
    },
    {
     "key": "system_functions.case",
     "label": "System functions (NOW, CURRENT_DATE…)",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of system/catalog functions: NOW(), CURRENT_DATE, CURRENT_TIMESTAMP, etc.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "aliases.case",
     "label": "Aliases — case",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of table and column aliases.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "aliases.as",
     "label": "Aliases — AS keyword",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether the AS keyword appears before aliases (column, table and derived-table aliases such as FROM (SELECT …) AS x).",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "always insert AS"
      ],
      [
       "remove",
       "always remove AS"
      ]
     ],
     "sample": {
      "in": "select id as user_id, name full_name from users as u join orders o on o.user_id = u.id;",
      "with": {}
     }
    },
    {
     "key": "columns.case",
     "label": "Column names",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of column references.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "select u.id, count(*) as cnt, coalesce(u.name, 'x') as nm, current_date as ts, my_func(u.id) as f\nfrom public.users as u\nwhere u.active = true and u.deleted_at is null and u.id not in (1, 2);\n\nSELECT O.ID, COUNT(*) AS CNT, COALESCE(O.NAME, 'x') AS NM, CURRENT_DATE AS TS, MY_FUNC(O.ID) AS F\nFROM PUBLIC.ORDERS AS O\nWHERE O.ACTIVE = TRUE AND O.DELETED_AT IS NULL AND O.ID NOT IN (1, 2);\n\nSelect I.Id, Count(*) As Cnt, Coalesce(I.Name, 'x') As Nm, Current_Date As Ts, My_Func(I.Id) As F\nFrom Public.Items As I\nWhere I.Active = True And I.Deleted_At Is Null And I.Id Not In (1, 2);",
      "with": {}
     }
    },
    {
     "key": "plpgsql_variables.case",
     "label": "PL/pgSQL variables",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of user-defined PL/pgSQL identifiers inside function/procedure bodies: local variables declared in DECLARE, function parameters, and INTO targets. Does not affect SQL keywords or column/table names inside the body.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "create function trg() returns trigger as $$\nbegin\n  if Tg_Op = 'INSERT' then\n    return NEW;\n  elsif tg_op = 'UPDATE' then\n    return new;\n  end if;\n  raise notice '%', SQLERRM;\n  return Old;\nend;\n$$ language plpgsql;",
      "with": {}
     }
    },
    {
     "key": "plpgsql_keywords.case",
     "label": "PL/pgSQL procedural keywords",
     "type": "select",
     "def": "preserve",
     "desc": "Controls casing of PL/pgSQL-specific keywords inside function bodies: RAISE, NOTICE, RETURN, EXCEPTION, FOUND, etc.",
     "values": [
      [
       "preserve"
      ],
      [
       "upper"
      ],
      [
       "lower"
      ],
      [
       "capitalize",
       "First letter only"
      ]
     ],
     "sample": {
      "in": "create function f() returns integer as $$\nbegin\n  raise notice 'a';\n  RAISE NOTICE 'b';\n  Raise Notice 'c';\n  return 1;\nexception when others then\n  RAISE EXCEPTION 'failed';\nend;\n$$ language plpgsql;",
      "with": {}
     }
    }
   ]
  },
  {
   "id": "punctuation",
   "title": "Punctuation & Spacing",
   "opts": [
    {
     "key": "trailing_whitespace",
     "label": "Trailing whitespace",
     "type": "select",
     "def": "preserve",
     "desc": "Controls trailing whitespace at the end of each line.",
     "values": [
      [
       "preserve"
      ],
      [
       "strip",
       "remove trailing spaces"
      ]
     ],
     "sample": {
      "in": "select id, name   \nfrom users;  ",
      "with": {}
     }
    },
    {
     "key": "semicolons",
     "label": "Semicolons",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether statements end with a semicolon.",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "always add"
      ],
      [
       "remove",
       "always remove"
      ]
     ],
     "sample": {
      "in": "select 1;\nselect 2",
      "with": {}
     }
    },
    {
     "key": "inequality_op",
     "label": "Inequality operator",
     "type": "select",
     "def": "preserve",
     "desc": "Normalises the inequality operator to either <> (ANSI) or != (C-style).",
     "values": [
      [
       "preserve"
      ],
      [
       "ansi",
       "use <>"
      ],
      [
       "c",
       "use !="
      ]
     ],
     "sample": {
      "in": "select 1 from t where status <> 'deleted' and a != 1;",
      "with": {}
     }
    },
    {
     "key": "join_form",
     "label": "JOIN form",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether JOIN keywords are written in short or long form.",
     "values": [
      [
       "preserve"
      ],
      [
       "short",
       "JOIN / LEFT JOIN"
      ],
      [
       "long",
       "INNER JOIN / LEFT OUTER JOIN"
      ]
     ],
     "sample": {
      "in": "select 1 from a join b on b.id = a.id left join c on c.id = a.id inner join d on d.id = a.id left outer join e on e.id = a.id;",
      "with": {}
     }
    },
    {
     "key": "operator_spacing",
     "label": "Operator spacing",
     "type": "select",
     "def": "preserve",
     "desc": "Controls spaces around symbolic comparison and concatenation operators: =, !=, <>, <, >, <=, >=, ||, plus := and =>. Arithmetic operators are controlled separately by arithmetic_spacing.",
     "values": [
      [
       "preserve"
      ],
      [
       "normalize",
       "one space each side"
      ],
      [
       "compact",
       "no spaces around operators"
      ]
     ],
     "sample": {
      "in": "select first_name||' '|| last_name from t where salary>50000 and name != 'bot';",
      "with": {}
     }
    },
    {
     "key": "blank_lines",
     "label": "Limit consecutive blank lines",
     "type": "select",
     "def": "preserve",
     "desc": "Caps runs of consecutive blank lines anywhere in the file (also inside statements); it only removes blank lines, never adds them. To set an exact number between statements, use blank_lines_between_statements.",
     "values": [
      [
       "preserve"
      ],
      [
       "max_1",
       "at most 1 blank line"
      ],
      [
       "max_2",
       "at most 2"
      ],
      [
       "max_3",
       "at most 3"
      ]
     ],
     "sample": {
      "in": "select 1;\n\n\n\n\nselect 2;\n\n\nselect 3;",
      "with": {}
     }
    },
    {
     "key": "call_paren_spacing",
     "label": "Space before call paren",
     "type": "select",
     "def": "preserve",
     "desc": "Controls the space between a function name and its opening parenthesis. Applies to function calls only (not IN (…), VALUES (…)).",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "count (*)"
      ],
      [
       "remove",
       "count(*)"
      ]
     ],
     "sample": {
      "in": "select count (*), my_func(b), coalesce (a, b) from t;",
      "with": {}
     }
    },
    {
     "key": "paren_spacing",
     "label": "Paren spacing",
     "type": "select",
     "def": "preserve",
     "desc": "Controls spaces immediately inside parentheses.",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "space inside parens"
      ],
      [
       "remove",
       "no space inside parens"
      ]
     ],
     "sample": {
      "in": "select count( id ), f(x) from t where id in ( 1, 2 );",
      "with": {}
     }
    },
    {
     "key": "quoted_identifiers",
     "label": "Quoted identifiers",
     "type": "select",
     "def": "preserve",
     "desc": "Removes unnecessary double-quote wrappers from identifiers that are safe without them (lowercase, no spaces, not reserved).",
     "values": [
      [
       "preserve"
      ],
      [
       "remove_safe",
       "remove unnecessary quotes"
      ]
     ],
     "sample": {
      "in": "select \"id\", \"Order\", \"select\" from \"users\";",
      "with": {}
     }
    },
    {
     "key": "blank_lines_between_statements",
     "label": "Exact blank lines between statements",
     "type": "select",
     "def": "preserve",
     "desc": "Exactly this many blank lines between statements (after the semicolon). Comments directly above a statement stay with it; psql meta-commands and the text after the last statement are not touched. Any number from 0 to 10 works in the YAML file. Unlike blank_lines, it only looks at the gap after each statement and can also add blank lines.",
     "values": [
      [
       "preserve"
      ],
      [
       "0",
       "none"
      ],
      [
       "1"
      ],
      [
       "2"
      ],
      [
       "3"
      ]
     ],
     "sample": {
      "in": "select 1;\n\n\n\nselect 2;\nselect 3;",
      "with": {}
     }
    },
    {
     "key": "statement_break",
     "label": "Statements on one line",
     "type": "select",
     "def": "preserve",
     "desc": "always starts every statement on its own line when several share a line (select 1; select 2;). Blank lines are not touched; a comment on the same line as a ';' stays with that statement. blank_lines_between_statements also separates them and takes precedence.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "always",
       "every statement on its own line"
      ]
     ],
     "sample": {
      "in": "select 1; select 2; -- note\nselect 3;\n\n\nselect 4; /* c */ select 5;   select 6;",
      "with": {}
     }
    },
    {
     "key": "comments.space_after_marker",
     "label": "Space after --",
     "type": "select",
     "def": "preserve",
     "desc": "Adds a space after -- when the comment text starts right behind it. Separator lines (-----) and markers such as --+ or --! are left alone; block comments are never touched.",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "--note → -- note"
      ]
     ],
     "sample": {
      "in": "select 1; --note\n--another\nselect 2; ----- keep\n--+ hint",
      "with": {}
     }
    },
    {
     "key": "comments.align_trailing",
     "label": "Align trailing comments",
     "type": "select",
     "def": "preserve",
     "desc": "align starts the trailing -- comments of consecutive lines in one column (a single trailing comment is left alone, runs whose code contains a tab are skipped); none leaves exactly one space before every trailing comment.",
     "values": [
      [
       "preserve"
      ],
      [
       "align",
       "one column"
      ],
      [
       "none",
       "one space before every comment"
      ]
     ],
     "sample": {
      "in": "select a,    -- first\n  bb,  -- second\n  ccc -- third\nfrom t;",
      "with": {}
     }
    },
    {
     "key": "arithmetic_spacing",
     "label": "Arithmetic spacing",
     "type": "select",
     "def": "preserve",
     "desc": "Controls spaces around binary arithmetic operators + - * / % ^. Only operators that are provably binary are changed; unary signs (x = -1), wildcards (SELECT *, count(*), t.*) and %TYPE are never touched.",
     "values": [
      [
       "preserve",
       "leave as written"
      ],
      [
       "normalize",
       "one space each side"
      ],
      [
       "compact",
       "no spaces around operators"
      ]
     ],
     "sample": {
      "in": "select price*qty, total - 1 from t where n+1 > x and y = -1;",
      "with": {}
     }
    },
    {
     "key": "comma_spacing",
     "label": "Comma spacing",
     "type": "select",
     "def": "preserve",
     "desc": "Controls spaces around commas in SELECT lists, function arguments, VALUES, etc.",
     "values": [
      [
       "preserve"
      ],
      [
       "normalize",
       "space after, none before"
      ],
      [
       "compact",
       "no spaces around commas"
      ]
     ],
     "sample": {
      "in": "select a,b , c from t;\nselect coalesce(x,'n/a') from t;",
      "with": {}
     }
    },
    {
     "key": "order_asc",
     "label": "ORDER BY ASC keyword",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether ORDER BY columns have an explicit ASC keyword.",
     "values": [
      [
       "preserve"
      ],
      [
       "add",
       "always add ASC"
      ],
      [
       "remove",
       "remove explicit ASC"
      ]
     ],
     "sample": {
      "in": "select id from t order by name asc, created_at desc, id;",
      "with": {}
     }
    },
    {
     "key": "cast_style",
     "label": "CAST style",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether type casts are written as CAST(x AS type) or x::type. SQL has two equivalent syntaxes — the function form (ANSI standard) and the :: operator form (PostgreSQL extension).",
     "values": [
      [
       "preserve"
      ],
      [
       "operator",
       "use x::type form"
      ]
     ],
     "sample": {
      "in": "select cast(price as numeric(10,2)), created_at::date from orders;",
      "with": {}
     }
    },
    {
     "key": "not_in",
     "label": "NOT IN form",
     "type": "select",
     "def": "preserve",
     "desc": "Normalises the NOT IN predicate to either NOT IN (…) or <> ALL (ARRAY[…]).",
     "values": [
      [
       "preserve"
      ],
      [
       "not_in",
       "use NOT IN"
      ],
      [
       "not_equals_all",
       "use <> ALL"
      ]
     ],
     "sample": {
      "in": "select 1 from t where id not in (1, 2, 3) and x <> all(array[4, 5]);",
      "with": {}
     }
    },
    {
     "key": "schema_qualification",
     "label": "Schema qualification",
     "type": "select",
     "def": "preserve",
     "desc": "Removes the public. schema prefix from object names when it is the default schema.",
     "values": [
      [
       "preserve"
      ],
      [
       "remove_public",
       "drop public. prefix"
      ]
     ],
     "sample": {
      "in": "select * from public.users u join public.orders o on o.uid = u.id;",
      "with": {}
     }
    },
    {
     "key": "inline_spacing",
     "label": "Inline spacing",
     "type": "select",
     "def": "preserve",
     "desc": "Collapses two or more consecutive horizontal spaces between tokens on the same line to a single space. Does not touch newlines or indentation.",
     "values": [
      [
       "preserve"
      ],
      [
       "normalize",
       "collapse multiple spaces to one"
      ]
     ],
     "sample": {
      "in": "select  a,  b   from   t  where x  = 1;",
      "with": {}
     }
    }
   ]
  },
  {
   "id": "layout-basics",
   "title": "Layout — Basics",
   "opts": [
    {
     "key": "layout.line_length",
     "label": "Line length",
     "type": "number",
     "def": "128",
     "desc": "Target line length used by auto break mode to decide when to break clauses or content items onto new lines.",
     "min": 40,
     "max": 500,
     "nums": [
      "40",
      "60",
      "80",
      "128",
      "200"
     ],
     "sample": {
      "in": "select customer_id, order_date, total_amount, shipping_address, billing_address from orders;",
      "with": {
       "layout.content.break": "auto"
      }
     }
    },
    {
     "key": "layout.indent.size",
     "label": "Indent size",
     "type": "number",
     "def": "2",
     "desc": "Number of spaces (or tabs) per indent level.",
     "min": 1,
     "max": 8,
     "nums": [
      "1",
      "2",
      "3",
      "4",
      "6",
      "8"
     ],
     "sample": {
      "in": "select id, name from users where active = true;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.indent.type",
     "label": "Indent type",
     "type": "select",
     "def": "spaces",
     "desc": "Whether to indent with spaces or a tab character.",
     "values": [
      [
       "spaces"
      ],
      [
       "tab"
      ]
     ],
     "sample": {
      "in": "select id, name from users where active = true;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.indent.normalize",
     "label": "Indent normalize",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether existing leading whitespace (indent) is re-written to match indent.size and indent.type.",
     "values": [
      [
       "preserve",
       "keep existing indent"
      ],
      [
       "change",
       "re-indent to configured style"
      ]
     ],
     "sample": {
      "in": "select\n  id,\n  name\nfrom users\nwhere\n  active = true;",
      "with": {
       "layout.indent.type": "tab",
       "layout.indent.size": "2"
      }
     }
    },
    {
     "key": "layout.indent.remainder",
     "label": "Indent remainder handling",
     "type": "select",
     "def": "keep",
     "desc": "When re-indenting (normalize=change), controls what happens when existing indent does not divide evenly by indent.size. Example: 5 spaces with size=3 leaves a remainder of 2.",
     "values": [
      [
       "keep",
       "preserve partial indent"
      ],
      [
       "add",
       "round up to next level"
      ],
      [
       "remove",
       "round down"
      ],
      [
       "round",
       "round to nearest"
      ]
     ],
     "sample": {
      "in": "   select 1;\n     select 2;\n        select 3;",
      "with": {
       "layout.indent.normalize": "change",
       "layout.indent.type": "tab"
      }
     }
    }
   ]
  },
  {
   "id": "layout-clauses",
   "title": "Layout — Clauses",
   "opts": [
    {
     "key": "layout.clauses.break",
     "label": "Default clause break",
     "type": "select",
     "def": "preserve",
     "desc": "Default break mode for all SQL clauses (FROM, WHERE, GROUP BY, etc.). Per-clause settings override this.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "sample": {
      "in": "select id, name from users where active = true order by name;",
      "with": {
       "layout.line_length": "40"
      }
     }
    },
    {
     "key": "layout.clauses.align",
     "label": "Default clause align",
     "type": "select",
     "def": "same",
     "desc": "Default alignment mode: same keeps content on the same indent as the keyword; indent pushes content one level deeper.",
     "values": [
      [
       "same",
       "keyword indented same as clause"
      ],
      [
       "indent",
       "content indented under clause"
      ]
     ],
     "sample": {
      "in": "select id, name from users where active = true;",
      "with": {}
     }
    },
    {
     "key": "layout.clauses.from.break",
     "label": "from clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for FROM. Overrides layout.clauses.break for this clause only.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select id, name from users where active = true;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.join.break",
     "label": "join clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for JOIN (and LEFT JOIN, INNER JOIN, etc.).",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select u.id, o.total from users as u join orders as o on o.user_id = u.id left join products as p on p.id = o.product_id;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.where.break",
     "label": "where clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for WHERE.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select id from users where active = true and role = 'admin';",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.group_by.break",
     "label": "group by clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for GROUP BY.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select dept, count(*) from employees group by dept;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.having.break",
     "label": "having clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for HAVING.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select dept, count(*) as cnt from employees group by dept having count(*) > 5;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.order_by.break",
     "label": "order by clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for ORDER BY.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select id, name from users order by name, created_at desc;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.limit.break",
     "label": "limit clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for LIMIT.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select id from users order by name limit 10;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.offset.break",
     "label": "offset clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for OFFSET.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "select id from users order by name limit 10 offset 20;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.values.break",
     "label": "values clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for VALUES in INSERT statements.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "insert into users (name, email) values ('Alice', 'alice@example.com') returning id;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.set.break",
     "label": "set clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for SET in UPDATE statements.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "update users set name = 'Bob', email = 'bob@example.com' where id = 1;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.returning.break",
     "label": "returning clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for RETURNING.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "insert into users (name) values ('Alice') returning id, name, created_at;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.with.break",
     "label": "with clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for WITH (CTEs).",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "example": "-- always\nWITH active AS (\n   SELECT id FROM users WHERE active = TRUE\n)\nSELECT * FROM active;\n\n-- never\nWITH active AS (SELECT id FROM users WHERE active = TRUE) SELECT * FROM active;"
    },
    {
     "key": "layout.clauses.merge.break",
     "label": "merge clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for MERGE INTO (the target table). Overrides layout.clauses.break for this clause only.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "example": "-- always\nMERGE INTO accounts AS tgt\nUSING staging AS src\nON tgt.id = src.id\nWHEN MATCHED THEN UPDATE SET balance = src.balance;"
    },
    {
     "key": "layout.clauses.merge_on.break",
     "label": "merge on clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for the ON condition of a MERGE.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "merge into t using s on t.id = s.id and t.ok when matched then delete;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.when.break",
     "label": "merge when clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for the WHEN [NOT] MATCHED branches of a MERGE.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "merge into t using s on t.id = s.id when matched then update set a = s.a when not matched then insert (id) values (s.id);",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    },
    {
     "key": "layout.clauses.on_conflict.break",
     "label": "on conflict clause break",
     "type": "select",
     "def": "",
     "desc": "Per-clause break override for ON CONFLICT in INSERT … ON CONFLICT DO UPDATE.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.clauses.break",
     "sample": {
      "in": "insert into products (id, name, price) values (1, 'Widget', 9.99) on conflict (id) do update set price = excluded.price;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "20"
      }
     }
    }
   ]
  },
  {
   "id": "layout-content",
   "title": "Layout — Content",
   "opts": [
    {
     "key": "layout.content.break",
     "label": "Default content break",
     "type": "select",
     "def": "preserve",
     "desc": "Default break mode for items within clauses (SELECT list, WHERE conditions, etc.). Controls WHETHER to break — see first_item for HOW. Per-section settings override this.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "sample": {
      "in": "select id, name, email from users;\n\nselect\n   id,\n   name,\n   email\nfrom users;",
      "with": {}
     }
    },
    {
     "key": "layout.content.first_item",
     "label": "Default first item placement",
     "type": "select",
     "def": "break",
     "desc": "Controls WHERE the first item lands when content breaks. Applies when break=always or break=auto triggers. Per-section settings override this.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "sample": {
      "in": "select id, name, email from users where active = true and role = 'admin';",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.align_aliases",
     "label": "Align AS aliases",
     "type": "select",
     "def": "preserve",
     "desc": "align pads the expressions of a broken SELECT list so the AS keywords line up; none removes such padding and leaves a single space before AS. Items spanning several lines, with comments or CASE are left alone.",
     "values": [
      [
       "preserve"
      ],
      [
       "align",
       "pad so every AS lines up"
      ],
      [
       "none",
       "one space, no padding"
      ]
     ],
     "sample": {
      "in": "select\n   id,\n   long_column_name   as x,\n   count(*)           as c\nfrom t;\n\nselect id, long_column_name as x, count(*) as c from u;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.align_set",
     "label": "Align SET assignments",
     "type": "select",
     "def": "preserve",
     "desc": "align pads the targets of a broken UPDATE … SET list so the = signs line up; none removes such padding and leaves a single space before =.",
     "values": [
      [
       "preserve"
      ],
      [
       "align",
       "pad so every = lines up"
      ],
      [
       "none",
       "one space, no padding"
      ]
     ],
     "sample": {
      "in": "update t set\n   a         = 1,\n   long_name = 2;\n\nupdate u set a = 1, long_name = 2;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.wrap_expressions",
     "label": "Wrap long expressions",
     "type": "select",
     "def": "preserve",
     "desc": "Breaks a list item that is longer than line_length at the commas of its first parenthesised group (function arguments, IN lists, rows), one element per line, closing parenthesis on its own line. CASE, subqueries and items with comments are left alone.",
     "values": [
      [
       "preserve"
      ],
      [
       "wrap",
       "break items longer than line_length"
      ]
     ],
     "sample": {
      "in": "select coalesce(first_name, last_name, 'unknown') as n, id from t;",
      "with": {
       "layout.content.break": "always",
       "layout.line_length": "40"
      }
     }
    },
    {
     "key": "layout.content.comma_position",
     "label": "Comma position",
     "type": "select",
     "def": "preserve",
     "desc": "Where the comma goes when a list (SELECT items, GROUP BY, INSERT columns, VALUES rows, …) is broken over several lines. preserve (default) keeps leading commas if the list already has them and uses trailing commas otherwise; start puts the comma at the start of the line, in the indent, so the items themselves stay aligned.",
     "values": [
      [
       "preserve",
       "keep what the input uses"
      ],
      [
       "end",
       "a,"
      ],
      [
       "start",
       ", b"
      ]
     ],
     "sample": {
      "in": "select\n   a\n , b\n , c\nfrom t;\n\nselect x, y, z from u;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.logical_operator_position",
     "label": "AND / OR position",
     "type": "select",
     "def": "preserve",
     "desc": "Where AND / OR go when WHERE, HAVING, JOIN ON or MERGE ON conditions are broken over several lines. preserve (default) keeps them at the end of the line if the input already ends lines with AND / OR and puts them first otherwise. The AND of BETWEEN … AND … is never treated as a separator.",
     "values": [
      [
       "preserve",
       "keep what the input uses"
      ],
      [
       "start",
       "AND b = 2"
      ],
      [
       "end",
       "a = 1 AND"
      ]
     ],
     "sample": {
      "in": "select 1 from t where a = 1 and\n  b = 2 or\n  c = 3;\n\nselect 1 from t where x = 1 and y = 2;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.join_on_position",
     "label": "JOIN … ON position",
     "type": "select",
     "def": "preserve",
     "desc": "Where ON goes when a JOIN is broken over several lines. preserve (default) keeps ON on its own line if the input already starts a line with it and inline otherwise. newline keeps the table on the JOIN line and puts ON (and the following AND / OR) on indented lines below it. Combine with clauses.join.align: indent to indent JOIN under FROM.",
     "values": [
      [
       "preserve",
       "keep what the input uses"
      ],
      [
       "inline",
       "JOIN t ON a = b"
      ],
      [
       "newline",
       "ON on its own line"
      ]
     ],
     "sample": {
      "in": "select 1 from a\njoin b\n   on b.id = a.id\n   and b.ok;\n\nselect 1 from a join c on c.id = a.id and c.ok;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.align",
     "label": "Default content align",
     "type": "select",
     "def": "preserve",
     "desc": "Alignment of the content items within clauses. preserve (default) keeps the indent the input uses (items level with the keyword, or indented below it; indented when the input has none), same puts them level with the keyword, indent one level deeper.",
     "values": [
      [
       "preserve",
       "keep what the input uses"
      ],
      [
       "same",
       "keyword indented same as clause"
      ],
      [
       "indent",
       "content indented under clause"
      ]
     ],
     "sample": {
      "in": "select\nid,\nname\nfrom t;\n\nselect\n   id,\n   name\nfrom u;",
      "with": {
       "layout.content.break": "always"
      }
     }
    },
    {
     "key": "layout.content.select_list.break",
     "label": "select list — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break the SELECT column list. Overrides layout.content.break for SELECT only.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "select id, name, email from users;\n\nselect\n   id,\n   name,\n   email\nfrom users;",
      "with": {}
     }
    },
    {
     "key": "layout.content.select_list.first_item",
     "label": "select list — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first SELECT column lands when breaking. Overrides layout.content.first_item for SELECT only.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "select id, name, email from users;",
      "with": {
       "layout.content.select_list.break": "always"
      }
     }
    },
    {
     "key": "layout.content.where_conds.break",
     "label": "where conditions — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break WHERE conditions. Overrides layout.content.break for WHERE only.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "select id from users where active = true and role = 'admin';\n\nselect id from users\nwhere\n   active = true\n   and role = 'admin';",
      "with": {}
     }
    },
    {
     "key": "layout.content.where_conds.first_item",
     "label": "where conditions — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first WHERE condition lands when breaking. Overrides layout.content.first_item for WHERE only.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "select id from users where active = true and role = 'admin';",
      "with": {
       "layout.content.where_conds.break": "always"
      }
     }
    },
    {
     "key": "layout.content.having_conds.break",
     "label": "having conditions — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break HAVING conditions. Overrides layout.content.break for HAVING only.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "select dept from t group by dept having count(*) > 5 and avg(salary) > 50000;\n\nselect dept from t group by dept\nhaving\n   count(*) > 5\n   and avg(salary) > 50000;",
      "with": {}
     }
    },
    {
     "key": "layout.content.having_conds.first_item",
     "label": "having conditions — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first HAVING condition lands when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "select dept from t group by dept having count(*) > 5 and avg(salary) > 50000;",
      "with": {
       "layout.content.having_conds.break": "always"
      }
     }
    },
    {
     "key": "layout.content.join_on.break",
     "label": "join on conditions — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break JOIN ON conditions. Overrides layout.content.break for JOIN ON only.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "select 1 from users u join orders o on o.user_id = u.id and o.status = 'active';\n\nselect 1 from users u\njoin orders o\n   on o.user_id = u.id\n   and o.status = 'active';",
      "with": {}
     }
    },
    {
     "key": "layout.content.join_on.first_item",
     "label": "join on conditions — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first ON condition lands when breaking JOIN ON.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "select 1 from users u join orders o on o.user_id = u.id and o.status = 'active';",
      "with": {
       "layout.content.join_on.break": "always"
      }
     }
    },
    {
     "key": "layout.content.group_list.break",
     "label": "group by list — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break the GROUP BY column list.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "select 1 from t group by department, year, quarter;\n\nselect 1 from t group by\n   department,\n   year,\n   quarter;",
      "with": {}
     }
    },
    {
     "key": "layout.content.group_list.first_item",
     "label": "group by list — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first GROUP BY column lands when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "select 1 from t group by department, year, quarter;",
      "with": {
       "layout.content.group_list.break": "always"
      }
     }
    },
    {
     "key": "layout.content.order_list.break",
     "label": "order by list — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break the ORDER BY column list.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "select 1 from t order by name asc, created_at desc;\n\nselect 1 from t order by\n   name asc,\n   created_at desc;",
      "with": {}
     }
    },
    {
     "key": "layout.content.order_list.first_item",
     "label": "order by list — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first ORDER BY column lands when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "select 1 from t order by name asc, created_at desc;",
      "with": {
       "layout.content.order_list.break": "always"
      }
     }
    },
    {
     "key": "layout.content.set_list.break",
     "label": "set assignments — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break SET assignments in UPDATE.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "update users set name = 'Bob', email = 'bob@example.com' where id = 1;\n\nupdate users set\n   name = 'Bob',\n   email = 'bob@example.com'\nwhere id = 1;",
      "with": {}
     }
    },
    {
     "key": "layout.content.set_list.first_item",
     "label": "set assignments — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first SET assignment lands when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "update users set name = 'Bob', email = 'bob@example.com' where id = 1;",
      "with": {
       "layout.content.set_list.break": "always"
      }
     }
    },
    {
     "key": "layout.content.insert_columns.break",
     "label": "insert columns — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break the column list in INSERT INTO table (col1, col2, …).",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "insert into users (id, name, email) values (1, 'a', 'b');\n\ninsert into users (\n   id,\n   name,\n   email\n) values (1, 'a', 'b');",
      "with": {}
     }
    },
    {
     "key": "layout.content.insert_columns.first_item",
     "label": "insert columns — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first column lands in the INSERT column list when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "insert into users (id, name, email) values (1, 'a', 'b');",
      "with": {
       "layout.content.insert_columns.break": "always"
      }
     }
    },
    {
     "key": "layout.content.values_list.break",
     "label": "values rows — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break rows in VALUES.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "insert into t (a, b) values (1, 'Alice'), (2, 'Bob');\n\ninsert into t (a, b) values\n   (1, 'Alice'),\n   (2, 'Bob');",
      "with": {}
     }
    },
    {
     "key": "layout.content.values_list.first_item",
     "label": "values rows — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first VALUES row lands when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "insert into t (a, b) values (1, 'Alice'), (2, 'Bob');",
      "with": {
       "layout.content.values_list.break": "always"
      }
     }
    },
    {
     "key": "layout.content.returning_list.break",
     "label": "returning list — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break the RETURNING column list.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "insert into t (a) values (1) returning id, name, created_at;\n\ninsert into t (a) values (1) returning\n   id,\n   name,\n   created_at;",
      "with": {}
     }
    },
    {
     "key": "layout.content.returning_list.first_item",
     "label": "returning list — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first RETURNING column lands when breaking.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "insert into t (a) values (1) returning id, name, created_at;",
      "with": {
       "layout.content.returning_list.break": "always"
      }
     }
    },
    {
     "key": "layout.content.merge_on.break",
     "label": "merge on conditions — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break the AND/OR conditions of a MERGE ON clause.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "merge into t using s on tgt.id = src.id and tgt.active when matched then delete;\n\nmerge into t using s on\n   tgt.id = src.id\n   and tgt.active\nwhen matched then delete;",
      "with": {
       "layout.clauses.merge_on.break": "always"
      }
     }
    },
    {
     "key": "layout.content.merge_on.first_item",
     "label": "merge on conditions — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first condition lands when breaking a MERGE ON clause.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "merge into t using s on tgt.id = src.id and tgt.active when matched then delete;",
      "with": {
       "layout.content.merge_on.break": "always",
       "layout.clauses.merge_on.break": "always"
      }
     }
    },
    {
     "key": "layout.content.when_actions.break",
     "label": "merge when action — break",
     "type": "select",
     "def": "preserve",
     "desc": "Whether the action of a MERGE WHEN branch goes on its own line below THEN. Defaults to preserve (the action stays where the input has it) and does not inherit layout.content.break.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "sample": {
      "in": "merge into t using s on t.id = s.id when matched and s.amount > 0 then update set balance = s.amount;\n\nmerge into t using s on t.id = s.id when matched then\n   delete;",
      "with": {
       "layout.clauses.when.break": "always"
      }
     }
    },
    {
     "key": "layout.content.with_list.break",
     "label": "with cte list — break",
     "type": "select",
     "def": "",
     "desc": "Whether to break multiple CTEs in a WITH clause.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "inherit": "layout.content.break",
     "sample": {
      "in": "with active as (select id from users), recent as (select user_id from orders) select * from active;\n\nwith\n   active as (select id from users),\n   recent as (select user_id from orders)\nselect * from active;",
      "with": {}
     }
    },
    {
     "key": "layout.content.with_list.first_item",
     "label": "with cte list — first item",
     "type": "select",
     "def": "",
     "desc": "Where the first CTE lands when breaking a WITH clause.",
     "values": [
      [
       "break",
       "first item on new indented line"
      ],
      [
       "inline",
       "first item on keyword line"
      ]
     ],
     "inherit": "layout.content.first_item",
     "sample": {
      "in": "with active as (select id from users), recent as (select user_id from orders) select * from active;",
      "with": {
       "layout.content.with_list.break": "always"
      }
     }
    }
   ]
  },
  {
   "id": "layout-other",
   "title": "Layout — Other",
   "opts": [
    {
     "key": "layout.ddl.create_function.params",
     "label": "CREATE FUNCTION — parameters",
     "type": "select",
     "def": "preserve",
     "desc": "Breaks the parameter list of CREATE FUNCTION / PROCEDURE: one parameter per line, the closing parenthesis on its own line. Statements with a -- comment in the header or a BEGIN ATOMIC body are left alone.",
     "values": [
      [
       "preserve"
      ],
      [
       "never",
       "one line"
      ],
      [
       "always",
       "one parameter per line"
      ],
      [
       "auto",
       "when longer than line_length"
      ]
     ],
     "sample": {
      "in": "create function add_user(p_name text, p_age integer default 18) returns bigint language sql immutable as $$ select 1::bigint $$;",
      "with": {}
     }
    },
    {
     "key": "layout.ddl.create_function.options",
     "label": "CREATE FUNCTION — options",
     "type": "select",
     "def": "preserve",
     "desc": "Puts RETURNS, LANGUAGE, IMMUTABLE/STABLE/VOLATILE, SECURITY, PARALLEL, COST, SET … and the AS body of CREATE FUNCTION / PROCEDURE on their own lines.",
     "values": [
      [
       "preserve"
      ],
      [
       "never",
       "one line"
      ],
      [
       "always",
       "one option per line"
      ],
      [
       "auto",
       "when longer than line_length"
      ]
     ],
     "sample": {
      "in": "create function add_user(p_name text, p_age integer default 18) returns bigint language sql immutable as $$ select 1::bigint $$;\n\ncreate function add_user(p_name text, p_age integer default 18)\nreturns bigint\nlanguage sql\nimmutable as $$ select 1::bigint $$;",
      "with": {}
     }
    },
    {
     "key": "layout.ddl.create_index.break",
     "label": "CREATE INDEX — break",
     "type": "select",
     "def": "preserve",
     "desc": "Puts the ON, USING, INCLUDE, NULLS [NOT] DISTINCT, WITH (…), TABLESPACE and WHERE parts of CREATE INDEX on their own indented lines. Statements with a -- comment are left alone.",
     "values": [
      [
       "preserve"
      ],
      [
       "never",
       "one line"
      ],
      [
       "always",
       "one part per line"
      ],
      [
       "auto",
       "when longer than line_length"
      ]
     ],
     "sample": {
      "in": "create unique index idx_users_email on public.users using btree (lower(email)) include (name) where deleted_at is null;\n\ncreate unique index idx_users_email\n   on public.users\n   using btree (lower(email))\n   include (name)\n   where deleted_at is null;",
      "with": {
       "layout.line_length": "40"
      }
     }
    },
    {
     "key": "layout.ddl.create_table.break",
     "label": "CREATE TABLE — break",
     "type": "select",
     "def": "preserve",
     "desc": "Breaks the column list of CREATE TABLE (…): one column or table constraint per line, the closing parenthesis on its own line, options such as WITH (…) after it. comma_position and the indent size apply.",
     "values": [
      [
       "preserve"
      ],
      [
       "never",
       "one line"
      ],
      [
       "always",
       "one definition per line"
      ],
      [
       "auto",
       "when longer than line_length"
      ]
     ],
     "sample": {
      "in": "create table users (id bigint not null, name text, constraint pk primary key (id));\n\ncreate table t (\n   a int,\n   b int\n);",
      "with": {
       "layout.line_length": "40"
      }
     }
    },
    {
     "key": "layout.ddl.create_table.align",
     "label": "CREATE TABLE — align columns",
     "type": "select",
     "def": "preserve",
     "desc": "Aligns the column definitions of a broken CREATE TABLE. Table constraints are not aligned.",
     "values": [
      [
       "preserve"
      ],
      [
       "none",
       "single spaces"
      ],
      [
       "types",
       "align the types"
      ],
      [
       "all",
       "align types and constraints"
      ]
     ],
     "sample": {
      "in": "create table users (id bigint not null, created_at timestamp with time zone not null, name text);",
      "with": {
       "layout.ddl.create_table.break": "always"
      }
     }
    },
    {
     "key": "layout.ddl.alter_table.break",
     "label": "ALTER TABLE — break",
     "type": "select",
     "def": "preserve",
     "desc": "Puts each action of ALTER TABLE on its own indented line below the table name. auto breaks only when there is more than one action or the statement is too long.",
     "values": [
      [
       "preserve"
      ],
      [
       "never",
       "one line"
      ],
      [
       "always",
       "one action per line"
      ],
      [
       "auto",
       "several actions or longer than line_length"
      ]
     ],
     "sample": {
      "in": "alter table users add column notes text, drop column old_col;\nalter table users rename to accounts;\n\nalter table users\n   add column notes text,\n   drop column old_col;",
      "with": {}
     }
    },
    {
     "key": "layout.union.blank_line",
     "label": "UNION blank line",
     "type": "select",
     "def": "preserve",
     "desc": "Controls blank lines around UNION / UNION ALL / INTERSECT / EXCEPT operators.",
     "values": [
      [
       "preserve"
      ],
      [
       "none",
       "no blank lines around UNION"
      ],
      [
       "before",
       "blank line before UNION"
      ],
      [
       "after",
       "blank line after UNION"
      ],
      [
       "both",
       "blank lines before and after"
      ]
     ],
     "sample": {
      "in": "select id from a union all select id from b;",
      "with": {}
     }
    },
    {
     "key": "layout.case.break",
     "label": "CASE expression break",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether WHEN/ELSE branches of a SQL CASE expression break onto new lines.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "sample": {
      "in": "select case when status = 'active' then 1 when status = 'pending' then 2 else 0 end as s from t;\n\nselect case\n   when x = 1 then 'a'\n   else 'b'\nend from t;",
      "with": {}
     }
    },
    {
     "key": "layout.case.indent",
     "label": "CASE expression indent",
     "type": "select",
     "def": "indent",
     "desc": "Controls indentation of WHEN/ELSE inside a SQL CASE expression.",
     "values": [
      [
       "preserve"
      ],
      [
       "none",
       "WHEN/ELSE at CASE level"
      ],
      [
       "indent",
       "WHEN/ELSE indented under CASE"
      ]
     ],
     "sample": {
      "in": "select case when x = 1 then 'a' else 'b' end from t;",
      "with": {
       "layout.case.break": "always"
      }
     }
    },
    {
     "key": "layout.subquery.break",
     "label": "Subquery break",
     "type": "select",
     "def": "preserve",
     "desc": "Controls whether parenthesised subqueries and CTE bodies — (select …), (with …), (values …) — open onto their own lines: \"(\" stays, the body is laid out one level deeper with the same rules and \")\" gets a line of its own. never joins them onto one line; auto opens only those on a line longer than layout.line_length. Subqueries containing a -- comment are left as written.",
     "values": [
      [
       "preserve",
       "leave as-is"
      ],
      [
       "never",
       "keep on one line"
      ],
      [
       "always",
       "break every item"
      ],
      [
       "auto",
       "break if too long"
      ]
     ],
     "sample": {
      "in": "select * from (select id, name from users where active) s where id in (select id from orders);\n\nselect * from (\n  select 1\n) t;",
      "with": {
       "layout.clauses.break": "always",
       "layout.line_length": "40"
      }
     }
    }
   ]
  }
 ]
};
