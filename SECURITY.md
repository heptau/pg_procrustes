# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in `pg_procrustes`, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Contact: [security@80.cz](mailto:security@80.cz)

You can also refer to the machine-readable security policy at:
[https://pg_procrustes.80.cz/.well-known/security.txt](https://pg_procrustes.80.cz/.well-known/security.txt)

We aim to respond within 5 business days and will coordinate a fix and disclosure timeline with you.

## Scope

`pg_procrustes` is a local formatting tool — it reads `.sql` files (or stdin) and writes reformatted SQL. It does not connect to any database, execute queries, or handle credentials.

Relevant areas for security review:
- File path handling in the CLI (`-w`, `--out-dir`, `--backup` flags) and potential path traversal issues
- Processing of untrusted `.sql` input passed via stdin or file arguments
- The YAML config loader and parsing of `.pg_procrustes.yaml`
