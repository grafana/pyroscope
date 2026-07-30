#!/usr/bin/env bash
# Show the status of an Antithesis test run, or list recent runs when no run
# id is given. See https://antithesis.com/docs/reference/rest_api/
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"

# Load environment variables from a .env file if present; variables already
# set in the environment take precedence.
if [[ -f "$DIR/.env" ]]; then
	SAVED_ENV="$(export -p)"
	set -a
	. "$DIR/.env"
	set +a
	eval "$SAVED_ENV"
fi

HOST="${ANTITHESIS_HOST:-grafanalabs.antithesis.com}"

usage() {
	cat <<EOF
usage: $(basename "$0") [run_id]

Print the full details of a test run as JSON, or, without a run id, list
recent runs. Requires ANTITHESIS_API_KEY.

environment (also loaded from $DIR/.env,
with the inherited environment taking precedence):
  ANTITHESIS_API_KEY   API key, provided by Antithesis (required)
  ANTITHESIS_HOST      API host (default: $HOST)
EOF
}

case "${1:-}" in
-h | --help) usage; exit 0 ;;
esac

if [[ -z "${ANTITHESIS_API_KEY:-}" ]]; then
	echo "ANTITHESIS_API_KEY is required" >&2
	exit 1
fi

if [[ $# -eq 0 ]]; then
	curl --fail-with-body -sS "https://$HOST/api/v0/runs?limit=25" \
		-H "Authorization: Bearer $ANTITHESIS_API_KEY" |
		jq -r '["RUN ID", "STATUS", "CREATED", "DESCRIPTION"],
			(.data[] | [.run_id, .status, .created_at, .parameters["antithesis.description"] // ""]) |
			@tsv' |
		column -t -s $'\t'
	exit 0
fi

curl --fail-with-body -sS "https://$HOST/api/v0/runs/$1" \
	-H "Authorization: Bearer $ANTITHESIS_API_KEY" | jq .
