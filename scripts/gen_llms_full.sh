#!/usr/bin/env bash
# Generates docs/llms-full.txt — the full plain-text documentation for LLMs
# (https://llmstxt.org/), built from README.md and the reference .pg_procrustes.yaml.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="$ROOT/docs/llms-full.txt"

{
	cat <<'EOF'
# pg_procrustes — full documentation

> A fast, flexible PostgreSQL SQL formatter written in Go, driven by the native
> PostgreSQL parser (libpg_query). This file contains the complete README and the
> fully commented reference configuration. Short index: https://pg_procrustes.80.cz/llms.txt

Website: https://pg_procrustes.80.cz/
Config Builder: https://pg_procrustes.80.cz/config-builder.html
Source: https://github.com/heptau/pg_procrustes
Releases: https://github.com/heptau/pg_procrustes/releases
Changelog: https://github.com/heptau/pg_procrustes/blob/main/CHANGELOG.md

EOF
	# README without the H1 title and the CI/badge lines.
	sed -e '1{/^# /d;}' -e '/^\[!\[/d' "$ROOT/README.md" | sed -e '/./,$!d'
	printf '\n## Reference configuration (.pg_procrustes.yaml)\n\n'
	printf 'Every option with its default value and all allowed values in comments.\n\n'
	printf '```yaml\n'
	cat "$ROOT/.pg_procrustes.yaml"
	printf '```\n'
} >"$OUT"

echo "wrote $OUT"
