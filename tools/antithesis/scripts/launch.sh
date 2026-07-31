#!/usr/bin/env bash
# Launch an Antithesis test run (basic_k8s_test) with the images built and
# pushed by `make images push`. See https://antithesis.com/docs/reference/rest_api/
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
ROOT="$(cd "$DIR/../.." && pwd)"

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
TENANT="${ANTITHESIS_TENANT:-grafana}"
TAG="${IMAGE_TAG:-}"
DURATION=15
DESCRIPTION=""
RECIPIENTS=""
SOURCE=""
EPHEMERAL=false
DRY_RUN=false

usage() {
	cat <<EOF
usage: $(basename "$0") [options]

Launch an Antithesis test run. Requires ANTITHESIS_API_KEY.

options:
  --duration <minutes>   test duration (default: $DURATION)
  --description <text>   run description (default: "pyroscope <tag>")
  --recipients <emails>  semicolon-delimited report recipients
  --tag <tag>            image tag used with 'make images push'
                         (default: IMAGE_TAG env or tools/image-tag output)
  --tenant <name>        registry tenant (default: $TENANT)
  --source <name>        source identifier for property history separation
  --ephemeral            exclude the run from reports
  --dry-run              print the request payload instead of launching
  -h, --help             show this help

environment (also loaded from $DIR/.env,
with the inherited environment taking precedence):
  ANTITHESIS_API_KEY     API key, provided by Antithesis (required)
  ANTITHESIS_HOST        API host (default: $HOST)
  ANTITHESIS_TENANT      registry tenant (default: $TENANT)
  IMAGE_TAG              default for --tag
EOF
}

while [[ $# -gt 0 ]]; do
	case "$1" in
	--duration) DURATION="$2"; shift 2 ;;
	--description) DESCRIPTION="$2"; shift 2 ;;
	--recipients) RECIPIENTS="$2"; shift 2 ;;
	--tag) TAG="$2"; shift 2 ;;
	--tenant) TENANT="$2"; shift 2 ;;
	--source) SOURCE="$2"; shift 2 ;;
	--ephemeral) EPHEMERAL=true; shift ;;
	--dry-run) DRY_RUN=true; shift ;;
	-h | --help) usage; exit 0 ;;
	*) echo "unknown argument: $1" >&2; usage >&2; exit 1 ;;
	esac
done

TAG="${TAG:-$("$ROOT/tools/image-tag")}"
DESCRIPTION="${DESCRIPTION:-pyroscope $TAG}"
CREATOR="$(git config user.email 2>/dev/null | cut -d@ -f1 || true)"
REGISTRY="us-central1-docker.pkg.dev/molten-verve-216720/$TENANT-repository"

# All images pulled into the (air-gapped) test environment ahead of the run:
# the Pyroscope and test-client images built from the working tree, plus the
# public images referenced by the rendered manifests. Passing our images
# explicitly lets the tag override whatever was baked into the committed
# manifests.
IMAGES="$({
	echo "$REGISTRY/pyroscope:$TAG"
	echo "$REGISTRY/pyroscope-test-client:$TAG"
	sed -n 's/^[[:space:]]*image:[[:space:]]*//p' "$DIR"/manifests/*.yaml |
		tr -d "'\"" |
		grep -v "^$REGISTRY/" | sort -u
} | paste -sd';' -)"

PAYLOAD="$(jq -n \
	--arg description "$DESCRIPTION" \
	--arg duration "$DURATION" \
	--arg config_image "$REGISTRY/pyroscope-config:$TAG" \
	--arg images "$IMAGES" \
	--arg ephemeral "$EPHEMERAL" \
	--arg creator "$CREATOR" \
	--arg recipients "$RECIPIENTS" \
	--arg source "$SOURCE" \
	'{
		"antithesis.description": $description,
		"antithesis.duration": $duration,
		"antithesis.config_image": $config_image,
		"antithesis.images": $images,
		"antithesis.is_ephemeral": $ephemeral,
		"run.creator_name": $creator,
		"antithesis.report.recipients": $recipients,
		"antithesis.source": $source
	} | with_entries(select(.value != "")) | {params: .}')"

if $DRY_RUN; then
	echo "POST https://$HOST/api/v1/launch/basic_k8s_test"
	jq . <<<"$PAYLOAD"
	exit 0
fi

if [[ -z "${ANTITHESIS_API_KEY:-}" ]]; then
	echo "ANTITHESIS_API_KEY is required" >&2
	exit 1
fi

RESPONSE="$(curl --fail-with-body -sS \
	-X POST "https://$HOST/api/v1/launch/basic_k8s_test" \
	-H "Authorization: Bearer $ANTITHESIS_API_KEY" \
	-H "Content-Type: application/json" \
	-d "$PAYLOAD")"
jq . <<<"$RESPONSE"

RUN_ID="$(jq -r '.runId // empty' <<<"$RESPONSE")"
if [[ -n "$RUN_ID" ]]; then
	echo
	echo "check status with: $(dirname "$0")/status.sh $RUN_ID"
fi
