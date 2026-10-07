#!/bin/bash
# Render and assemble all three scenes into ../../images/.
# write-path runs a longer loop (5 flush cycles), so it gets more frames.
set -e
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

build() { # scene frames width fps
  "$DIR/render.sh" "$1" "$2"
  "$DIR/assemble.sh" "$1" "$3" "$4"
}

build write-path 156 1000 12
build compaction 168 1000 12
build read-path  162 1000 12
