#/bin/sh
if [ -z "${RIDESHARE_LISTEN_PORT}" ]; then
  RIDESHARE_LISTEN_PORT=5000
fi
exec rails s -b 0.0.0.0 -p ${RIDESHARE_LISTEN_PORT}