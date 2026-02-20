#!/usr/bin/env bash
set -euo pipefail

INTERACTIVE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    -i|--interactive) INTERACTIVE=true; shift ;;
    *) echo "Usage: $0 [-i|--interactive]"; exit 1 ;;
  esac
done

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
PYROSCOPE_PID=""
PYROSCOPE_DATA_DIR=$(mktemp -d)
echo "Pyroscope data dir: $PYROSCOPE_DATA_DIR"

cleanup() {
  echo "Cleaning up..."
  if [ -n "$PYROSCOPE_PID" ] && kill -0 "$PYROSCOPE_PID" 2>/dev/null; then
    kill "$PYROSCOPE_PID" 2>/dev/null || true
    wait "$PYROSCOPE_PID" 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true
  rm -rf "$PYROSCOPE_DATA_DIR"
}
trap cleanup EXIT

echo "=== Stage 1: Build Pyroscope and profilecli ==="
go build -o "$SCRIPT_DIR/pyroscope" "$ROOT_DIR/cmd/pyroscope"
go build -o "$SCRIPT_DIR/profilecli" "$ROOT_DIR/cmd/profilecli"

echo "=== Stage 2: Start Tempo ==="
docker compose -f "$COMPOSE_FILE" up -d

echo "Waiting for Tempo to be ready..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:3200/ready > /dev/null 2>&1; then
    echo "Tempo is ready."
    break
  fi
  if [ "$i" -eq 30 ]; then
    echo "ERROR: Tempo did not become ready in time."
    docker compose -f "$COMPOSE_FILE" logs tempo
    exit 1
  fi
  sleep 2
done

echo "=== Stage 3: Start Pyroscope ==="
JAEGER_SERVICE_NAME=pyroscope-test \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
OTEL_TRACES_EXPORTER=otlp \
PYROSCOPE_V2=true \
  "$SCRIPT_DIR/pyroscope" \
    --config.file="$SCRIPT_DIR/pyroscope.yml" \
    --target=all \
    --write-path=segment-writer \
    --enable-query-backend=true \
    --segment-writer.min-ready-duration=10s \
    --storage.backend=filesystem \
    --storage.filesystem.dir="$PYROSCOPE_DATA_DIR/bucket" \
    --metastore.data-dir="$PYROSCOPE_DATA_DIR/metastore/data" \
    --metastore.raft.dir="$PYROSCOPE_DATA_DIR/metastore/raft" \
    --self-profiling.disable-push=true \
    > "$SCRIPT_DIR/pyroscope.log" 2>&1 &
PYROSCOPE_PID=$!

echo "Waiting for Pyroscope to be ready (PID: $PYROSCOPE_PID)..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:4040/ready > /dev/null 2>&1; then
    echo "Pyroscope is ready."
    break
  fi
  if ! kill -0 "$PYROSCOPE_PID" 2>/dev/null; then
    echo "ERROR: Pyroscope process died."
    cat "$SCRIPT_DIR/pyroscope.log"
    exit 1
  fi
  if [ "$i" -eq 60 ]; then
    echo "ERROR: Pyroscope did not become ready in time."
    cat "$SCRIPT_DIR/pyroscope.log"
    exit 1
  fi
  sleep 2
done

echo "=== Stage 4: Upload profile (write path) ==="
"$SCRIPT_DIR/profilecli" upload \
  --override-timestamp \
  "$ROOT_DIR/pkg/pprof/testdata/go.cpu.labels.pprof"

echo "=== Stage 5: Query profiles (read path) ==="
"$SCRIPT_DIR/profilecli" query merge \
   > /dev/null 2>&1 || echo "Query returned no data"

echo "=== Stage 6: Verify traces in Tempo ==="

# search_traces queries Tempo via TraceQL for traces matching a span name.
# Returns the count on stdout; all other output goes to stderr.
search_traces() {
  local label="$1" query="$2"
  local encoded_query
  encoded_query=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$query")
  local result
  result=$(curl -sf "http://localhost:3200/api/search?q=${encoded_query}&limit=5" 2>/dev/null || echo '{"traces":[]}')
  local count
  count=$(echo "$result" | python3 -c "import sys,json; data=json.load(sys.stdin); print(len(data.get('traces',[])))" 2>/dev/null || echo "0")
  echo "  $label: $count trace(s)" >&2
  if [ "$count" -gt 0 ]; then
    echo "$result" | python3 -m json.tool 2>/dev/null | head -30 >&2
  fi
  echo "$count"
}

WRITE_QUERY='{resource.service.name="pyroscope-test" && name="Distributor.Push"}'
READ_QUERY='{resource.service.name="pyroscope-test" && name="SelectMergeProfile"}'
MAX_ATTEMPTS=12
RETRY_DELAY=5

WRITE_COUNT=0
READ_COUNT=0

for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  echo "Attempt $attempt/$MAX_ATTEMPTS: searching for traces..."

  if [ "$WRITE_COUNT" -eq 0 ]; then
    WRITE_COUNT=$(search_traces "Write path" "$WRITE_QUERY")
  fi
  if [ "$READ_COUNT" -eq 0 ]; then
    READ_COUNT=$(search_traces "Read path" "$READ_QUERY")
  fi

  if [ "$WRITE_COUNT" -gt 0 ] && [ "$READ_COUNT" -gt 0 ]; then
    break
  fi

  if [ "$attempt" -lt "$MAX_ATTEMPTS" ]; then
    echo "  Waiting ${RETRY_DELAY}s before next attempt..."
    sleep "$RETRY_DELAY"
  fi
done

FAILED=false

echo ""
if [ "$WRITE_COUNT" -gt 0 ]; then
  echo "PASS: Write-path traces found (Distributor.Push)."
else
  echo "FAIL: No write-path traces found (Distributor.Push)."
  FAILED=true
fi

if [ "$READ_COUNT" -gt 0 ]; then
  echo "PASS: Read-path traces found (SelectMergeProfile)."
else
  echo "FAIL: No read-path traces found (SelectMergeProfile)."
  FAILED=true
fi

echo ""
if [ "$FAILED" = true ]; then
  echo "FAILURE: Not all expected traces were found."
  echo ""
  echo "Pyroscope logs (last 50 lines):"
  tail -50 "$SCRIPT_DIR/pyroscope.log"
fi

if [ "$INTERACTIVE" = true ]; then
  echo ""
  echo "Grafana is available at http://localhost:3000/explore"
  echo "Pyroscope is available at http://localhost:4040"
  echo ""
  echo "Press any key to tear down and exit..."
  read -r -n 1 -s
fi

if [ "$FAILED" = true ]; then
  exit 1
else
  echo "SUCCESS: Both write-path and read-path traces verified."
  exit 0
fi
