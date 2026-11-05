#!/bin/bash
# Assemble a scene's frames into a looping GIF in ../../images/.
# Usage: ./assemble.sh <scene> [width] [fps] [colors] [bayer_scale]
set -e
SCENE="${1:?usage: ./assemble.sh <scene> [width] [fps] [colors] [bayer_scale]}"
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
W=${2:-1000}
FPS=${3:-14}
COLORS=${4:-192}
BS=${5:-3}
OUT="$DIR/../../images/pyroscope-v2-$SCENE.gif"
ffmpeg -y -framerate "$FPS" -i "$DIR/frames/$SCENE/f_%03d.png" \
  -vf "scale=$W:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=$COLORS[p];[s1][p]paletteuse=dither=bayer:bayer_scale=$BS" \
  -loop 0 "$OUT" >/dev/null 2>&1
echo "$OUT: $(du -h "$OUT" | cut -f1), $(magick identify "$OUT" 2>/dev/null | wc -l | tr -d ' ') frames, $(magick identify -format '%wx%h' "$OUT[0]" 2>/dev/null)"
