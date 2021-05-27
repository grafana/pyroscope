set -e

echo "waiting for message on port 30014"
nc -l 30014
echo "got a message on port 30014"

NAME="$(date -u +%FT%TZ | tr ':' '-').png"

echo "taking a screenhost $NAME"
/phantomjs/bin/phantomjs /rasterize.js "http://grafana:80/d/65gjqY3Mk/main?orgId=1&refresh=5s" "$NAME" "$@" 1280px 2000 1
