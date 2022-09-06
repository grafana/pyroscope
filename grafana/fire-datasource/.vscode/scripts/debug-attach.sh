#!/bin/bash

# so dlv and mage are in the path
export PATH=$(go env GOPATH)/bin:$PATH

PLUGIN_NAME="gpx_fire-datasource_darwin_arm64"
PLUGIN_PID=`pgrep ${PLUGIN_NAME}`
PORT="${2:-3222}"

dlv attach ${PLUGIN_PID} --headless --listen=:${PORT} --api-version 2 --log
