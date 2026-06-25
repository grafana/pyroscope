#!/bin/bash
# Capture frames for one scene by rendering <scene>.html at evenly spaced t values.
# Usage: ./render.sh <scene> [frames]
# Override the Chrome/Chromium binary with the CHROME env var if needed, e.g.
#   CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" ./render.sh write-path

# Internal worker mode (one frame), invoked in parallel by xargs below.
if [ "$1" = "--shoot" ]; then
  i=$2; n=$3; chrome=$4; html=$5; outdir=$6; win=$7
  t=$(awk "BEGIN{printf \"%.5f\", $i/$n}")
  out=$(printf "%s/f_%03d.png" "$outdir" "$i")
  "$chrome" --headless --disable-gpu --hide-scrollbars \
    --force-device-scale-factor=1 --window-size="$win" \
    --screenshot="$out" "$html?t=$t" >/dev/null 2>&1
  exit 0
fi

set -e
SCENE="${1:?usage: ./render.sh <scene> [frames]}"
N="${2:-84}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHROME="${CHROME:-chrome-headless-shell}"
HTML="file://$DIR/$SCENE.html"
OUTDIR="$DIR/frames/$SCENE"

# read the canvas size from the scene's <svg> so each scene can have its own
WH=$(sed -n 's/.*<svg id="stage" width="\([0-9]*\)" height="\([0-9]*\)".*/\1,\2/p' "$DIR/$SCENE.html" | head -1)
WIN="${WH/,/x}"

rm -rf "$OUTDIR"; mkdir -p "$OUTDIR"
seq 0 $((N-1)) | xargs -P 6 -I{} "$0" --shoot {} "$N" "$CHROME" "$HTML" "$OUTDIR" "$WIN"
echo "rendered $(ls "$OUTDIR"/*.png 2>/dev/null | wc -l | tr -d ' ') frames to $OUTDIR"
