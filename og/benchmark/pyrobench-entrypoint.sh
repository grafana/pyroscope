#!/bin/sh

start="$(date +%s%3N)"
echo "pyrobench start: $start"

./pyrobench loadgen

end="$(date +%s%3N)"
echo "pyrobench end: $end"

echo "generating meta report"
./pyrobench report meta > /tmp/report.md

echo "generating image report"
./pyrobench report image --from="$start" --to="$end" >> /tmp/report.md

echo "generating table report"
./pyrobench report table >> /tmp/report.md

cat /tmp/report.md

cat /tmp/report.md > "/report/pr-report.md"
