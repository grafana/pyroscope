#!/usr/bin/env bash
set -euo pipefail

INTERACTIVE=false
while [[ $# -gt 0 ]]; do
  case "$1" in
    -i|--interactive) INTERACTIVE=true; shift ;;
    *) echo "Usage: $0 [-i|--interactive]"; exit 1 ;;
  esac
done

log() { echo "[$(date "+%H:%M:%S")] $*"; }

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
PYROSCOPE_PID=""
PYROSCOPE_DATA_DIR=$(mktemp -d)
log "Pyroscope data dir: $PYROSCOPE_DATA_DIR"

cleanup() {
  log "Cleaning up..."
  if [ -n "$PYROSCOPE_PID" ] && kill -0 "$PYROSCOPE_PID" 2>/dev/null; then
    kill "$PYROSCOPE_PID" 2>/dev/null || true
    wait "$PYROSCOPE_PID" 2>/dev/null || true
  fi
  docker compose -f "$COMPOSE_FILE" down -v 2>/dev/null || true
  rm -rf "$PYROSCOPE_DATA_DIR"
}
trap cleanup EXIT

log "=== Stage 1: Build Pyroscope and profilecli ==="
go build -o "$SCRIPT_DIR/pyroscope" "$ROOT_DIR/cmd/pyroscope"
go build -o "$SCRIPT_DIR/profilecli" "$ROOT_DIR/cmd/profilecli"

log "=== Stage 2: Start Tempo ==="
docker compose -f "$COMPOSE_FILE" up -d

log "Waiting for Tempo to be ready..."
for i in $(seq 1 30); do
  if curl -sf http://localhost:3200/ready > /dev/null 2>&1; then
    log "Tempo is ready."
    break
  fi
  if [ "$i" -eq 30 ]; then
    log "ERROR: Tempo did not become ready in time."
    docker compose -f "$COMPOSE_FILE" logs tempo
    exit 1
  fi
  sleep 2
done

log "=== Stage 3: Start Pyroscope ==="
OTEL_SERVICE_NAME=pyroscope-test \
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4317 \
OTEL_EXPORTER_OTLP_PROTOCOL=grpc \
OTEL_TRACES_EXPORTER=otlp \
PYROSCOPE_V2=true \
  "$SCRIPT_DIR/pyroscope" \
    --config.file="$SCRIPT_DIR/pyroscope.yml" \
    --target=all \
    --write-path=segment-writer \
    --enable-query-backend=true \
    --segment-writer.min-ready-duration=0s \
    --ingester.min-ready-duration=0s \
    --metastore.min-ready-duration=0s \
    --storage.backend=filesystem \
    --storage.filesystem.dir="$PYROSCOPE_DATA_DIR/bucket" \
    --metastore.data-dir="$PYROSCOPE_DATA_DIR/metastore/data" \
    --metastore.raft.dir="$PYROSCOPE_DATA_DIR/metastore/raft" \
    --self-profiling.disable-push=true \
    > "$SCRIPT_DIR/pyroscope.log" 2>&1 &
PYROSCOPE_PID=$!

log "Waiting for Pyroscope to be ready (PID: $PYROSCOPE_PID)..."
for i in $(seq 1 60); do
  if curl -sf http://localhost:4040/ready > /dev/null 2>&1; then
    log "Pyroscope is ready."
    break
  fi
  if ! kill -0 "$PYROSCOPE_PID" 2>/dev/null; then
    log "ERROR: Pyroscope process died."
    cat "$SCRIPT_DIR/pyroscope.log"
    exit 1
  fi
  if [ "$i" -eq 60 ]; then
    log "ERROR: Pyroscope did not become ready in time."
    cat "$SCRIPT_DIR/pyroscope.log"
    exit 1
  fi
  sleep 2
done

log "=== Stage 4: Upload profile (write path) ==="
"$SCRIPT_DIR/profilecli" upload \
  --override-timestamp \
  --extra-labels='service_name=my_service' \
  "$ROOT_DIR/pkg/pprof/testdata/go.cpu.labels.pprof"

log "=== Stage 5: Query profiles (read path) ==="
"$SCRIPT_DIR/profilecli" query merge \
  --query='{service_name="my_service"}' > /dev/null 2>&1

log "=== Stage 6: Verify traces in Tempo ==="

# search_traces queries Tempo via TraceQL for traces matching a span name.
# Returns the count on stdout; all other output goes to stderr.
search_traces() {
  local label="$1" query="$2"
  local encoded_query
  encoded_query=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$query")
  local result
  result=$(curl -sf "http://localhost:3200/api/search?q=${encoded_query}&limit=5")
  if [ -z "$result" ]; then
    echo "  $label: Tempo query failed" >&2
    echo "0"
    return
  fi
  local count
  count=$(echo "$result" | python3 -c "import sys,json; data=json.load(sys.stdin); print(len(data.get('traces',[])))" 2>/dev/null || echo "0")
  echo "  $label: $count trace(s)" >&2
  if [ "$count" -gt 0 ]; then
    echo "$result" | python3 -m json.tool 2>/dev/null | head -30 >&2
  fi
  echo "$count"
}

# verify_trace_depth fetches a trace by searching for the query and checks
# that it contains more than one span, proving context propagation works.
verify_trace_depth() {
  local label="$1" query="$2"
  local encoded_query
  encoded_query=$(python3 -c "import urllib.parse,sys; print(urllib.parse.quote(sys.argv[1]))" "$query")
  local result
  result=$(curl -sf "http://localhost:3200/api/search?q=${encoded_query}&limit=1")
  if [ -z "$result" ]; then
    echo "  $label: could not fetch trace list" >&2
    return 1
  fi
  local trace_id
  trace_id=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); traces=d.get('traces',[]); print(traces[0]['traceID'] if traces else '')" 2>/dev/null)
  if [ -z "$trace_id" ]; then
    echo "  $label: no trace ID found" >&2
    return 1
  fi
  local trace_detail
  trace_detail=$(curl -sf "http://localhost:3200/api/traces/$trace_id")
  if [ -z "$trace_detail" ]; then
    echo "  $label: could not fetch trace $trace_id" >&2
    return 1
  fi
  local span_count
  span_count=$(echo "$trace_detail" | python3 -c "
import sys, json
data = json.load(sys.stdin)
# Tempo returns OTLP JSON: resourceSpans[] -> scopeSpans[] -> spans[]
count = 0
for rs in data.get('resourceSpans', data.get('batches', [])):
    for ss in rs.get('scopeSpans', rs.get('instrumentationLibrarySpans', [])):
        count += len(ss.get('spans', []))
print(count)
" 2>/dev/null || echo "0")
  echo "  $label: trace $trace_id has $span_count span(s)" >&2
  if [ "$span_count" -gt 1 ]; then
    return 0
  else
    return 1
  fi
}

WRITE_QUERY='{resource.service.name="pyroscope-test" && name="Distributor.Push"}'
READ_QUERY='{resource.service.name="pyroscope-test" && name="SelectMergeProfile"}'
MAX_ATTEMPTS=12
RETRY_DELAY=5

WRITE_COUNT=0
READ_COUNT=0

for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  log "Attempt $attempt/$MAX_ATTEMPTS: searching for traces..."

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
    log "  Waiting ${RETRY_DELAY}s before next attempt..."
    sleep "$RETRY_DELAY"
  fi
done

FAILED=false

echo ""
if [ "$WRITE_COUNT" -gt 0 ]; then
  log "PASS: Write-path traces found (Distributor.Push)."
else
  log "FAIL: No write-path traces found (Distributor.Push)."
  FAILED=true
fi

if [ "$READ_COUNT" -gt 0 ]; then
  log "PASS: Read-path traces found (SelectMergeProfile)."
else
  log "FAIL: No read-path traces found (SelectMergeProfile)."
  FAILED=true
fi

# Verify that traces contain multiple spans (context propagation works).
if [ "$WRITE_COUNT" -gt 0 ]; then
  if verify_trace_depth "Write path depth" "$WRITE_QUERY"; then
    log "PASS: Write-path trace has multiple spans (context propagation works)."
  else
    log "FAIL: Write-path trace has only 1 span (context propagation may be broken)."
    FAILED=true
  fi
fi

echo ""
if [ "$FAILED" = true ]; then
  log "FAILURE: Not all expected traces were found."
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
  log "SUCCESS: Both write-path and read-path traces verified."
  exit 0
fi
