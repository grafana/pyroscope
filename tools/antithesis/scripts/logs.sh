#!/usr/bin/env bash
# Download logs for an Antithesis test run.
# See https://antithesis.com/docs/reference/rest_api/
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
INPUT_HASH=""
VTIME=""
BUILD=false
OUTPUT=""

usage() {
	cat <<EOF
usage: $(basename "$0") [options] <run_id>

Download logs for a test run as NDJSON. Moment logs require an input hash and
vtime identifying a moment in the run; by default the run's failure moment is
used (fetched from the run details), so a run without failures needs an
explicit --input-hash/--vtime, or --build for the build logs. Requires
ANTITHESIS_API_KEY.

options:
  --input-hash <hash>   input hash of the moment to fetch logs for
  --vtime <vtime>       vtime of the moment to fetch logs for
  --build               download the run's build logs instead of moment logs
  -o, --output <file>   output file, '-' for stdout
                        (default: <run_id>-logs.ndjson or <run_id>-build-logs.ndjson)
  -h, --help            show this help

environment (also loaded from $DIR/.env,
with the inherited environment taking precedence):
  ANTITHESIS_API_KEY    API key, provided by Antithesis (required)
  ANTITHESIS_HOST       API host (default: $HOST)
EOF
}

RUN_ID=""
while [[ $# -gt 0 ]]; do
	case "$1" in
	--input-hash) INPUT_HASH="$2"; shift 2 ;;
	--vtime) VTIME="$2"; shift 2 ;;
	--build) BUILD=true; shift ;;
	-o | --output) OUTPUT="$2"; shift 2 ;;
	-h | --help) usage; exit 0 ;;
	-*) echo "unknown option: $1" >&2; usage >&2; exit 1 ;;
	*)
		if [[ -n "$RUN_ID" ]]; then
			echo "unexpected argument: $1" >&2; usage >&2; exit 1
		fi
		RUN_ID="$1"; shift ;;
	esac
done

if [[ -z "$RUN_ID" ]]; then
	usage >&2
	exit 1
fi
if [[ -z "${ANTITHESIS_API_KEY:-}" ]]; then
	echo "ANTITHESIS_API_KEY is required" >&2
	exit 1
fi

api() {
	curl --fail-with-body -sS -H "Authorization: Bearer $ANTITHESIS_API_KEY" "$@"
}

if $BUILD; then
	OUTPUT="${OUTPUT:-$RUN_ID-build-logs.ndjson}"
	api -o "$OUTPUT" "https://$HOST/api/v0/runs/$RUN_ID/build_logs"
else
	if [[ -z "$INPUT_HASH" || -z "$VTIME" ]]; then
		MOMENT="$(api "https://$HOST/api/v0/runs/$RUN_ID" | jq '.failure_moment')"
		if [[ "$MOMENT" == "null" ]]; then
			echo "run $RUN_ID has no failure moment; pass --input-hash and --vtime" \
				"(e.g. from a property counterexample), or --build for build logs" >&2
			exit 1
		fi
		INPUT_HASH="${INPUT_HASH:-$(jq -r '.input_hash' <<<"$MOMENT")}"
		VTIME="${VTIME:-$(jq -r '.vtime' <<<"$MOMENT")}"
		echo "using failure moment: input_hash=$INPUT_HASH vtime=$VTIME" >&2
	fi
	OUTPUT="${OUTPUT:-$RUN_ID-logs.ndjson}"
	api -G -o "$OUTPUT" "https://$HOST/api/v0/runs/$RUN_ID/logs" \
		--data-urlencode "input_hash=$INPUT_HASH" \
		--data-urlencode "vtime=$VTIME"
fi

if [[ "$OUTPUT" != "-" ]]; then
	echo "wrote $(wc -l <"$OUTPUT" | tr -d ' ') log lines to $OUTPUT" >&2
fi
