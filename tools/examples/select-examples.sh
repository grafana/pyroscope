#!/usr/bin/env bash
#
# Prints the repository-relative directories of the docker-compose examples that
# should be tested, one per line.
#
# Selection logic:
#   - If TARGET is set, select every example at or under that path prefix
#     (e.g. TARGET=examples/tracing selects all tracing examples).
#   - Else if BASE_REF is set, select every example that has a changed file in
#     the diff against BASE_REF (used for pull requests). If a top-level Go file
#     under examples/ changes, select all examples because those files implement
#     the shared test harness.
#   - Else select all examples.
#
# Environment:
#   TARGET    Optional path prefix to scope the selection to.
#   BASE_REF  Optional git ref to diff against (e.g. origin/main).

set -euo pipefail

# All example directories (those containing a docker-compose.yml).
# Exclude _templates/: its docker-compose files are single-service stubs, not runnable examples.
mapfile -t all_dirs < <(git ls-files 'examples/**/docker-compose.yml' 'examples/**/docker-compose.yaml' | grep -v '/_templates/' | xargs -n1 dirname | sort -u)

emit_under_prefix() {
  local prefix="${1%/}"
  local d
  for d in "${all_dirs[@]}"; do
    if [[ "$d" == "$prefix" || "$d" == "$prefix"/* ]]; then
      echo "$d"
    fi
  done
}

if [[ -n "${TARGET:-}" ]]; then
  emit_under_prefix "$TARGET"
  exit 0
fi

if [[ -n "${BASE_REF:-}" ]]; then
  mapfile -t changed < <(git diff --name-only "${BASE_REF}"...HEAD -- examples)
  for f in "${changed[@]:-}"; do
    if [[ "$f" =~ ^examples/[^/]+\.go$ ]]; then
      printf '%s\n' "${all_dirs[@]}"
      exit 0
    fi
  done
  for d in "${all_dirs[@]}"; do
    for f in "${changed[@]:-}"; do
      if [[ "$f" == "$d"/* ]]; then
        echo "$d"
        break
      fi
    done
  done
  exit 0
fi

printf '%s\n' "${all_dirs[@]}"
