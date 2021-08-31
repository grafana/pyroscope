#!/usr/bin/env bash

set -x
set -euo pipefail

export DOCKER_BUILDKIT=1

# Pull latest docker image
# This is required since we use a :latest image
./with-env.sh docker-compose pull

# Start the docker containers
./with-env.sh docker-compose up -d --force-recreate


#
## Run some load against both
## ./with-env.sh go run main.go load-test
#go run cmd/main.go loadgen --server-address="http://localhost:4040" &
#go run cmd/main.go loadgen --server-address="http://localhost:4041" &
#
#
#start=$(date +%s)
## wait 5 minutes
#sleep 60s
#
#end=$(date +%s)
## query prometheus
#go run cmd/main.go promquery $start $end &

sleep Infinity

# Query prometheus

#{
#  "request": {
#    "url": "api/datasources/proxy/1/api/v1/query_range?query=sum(rate(pyroscope_http_request_duration_seconds_count%7Binstance%3D%22pyroscope%3A4040%22%2C%20handler!%3D%22%2Fmetrics%22%2C%20handler!%3D%22%2Fhealthz%22%7D%5B60s%5D))%20by%20(handler)&start=1630338180&end=1630338480&step=30",
#    "method": "GET",
#    "hideFromInspector": false
#  },
#  "response": {
#    "status": "success",
#    "data": {
#      "resultType": "matrix",
#      "result": [
#        {
#          "metric": {
#            "handler": "/ingest"
#          },
#          "values": [
#            [
#              1630338180,
#              "0"
#            ],
#            [
#              1630338210,
#              "0"
#            ],
#            [
#              1630338240,
#              "0"
#            ],
#            [
#              1630338270,
#              "0"
#            ],
#            [
#              1630338300,
#              "0"
#            ],
#            [
#              1630338330,
#              "0"
#            ],
#            [
#              1630338360,
#              "0"
#            ],
#            [
#              1630338390,
#              "67.79661016949153"
#            ],
#            [
#              1630338420,
#              "135.59322033898306"
#            ],
#            [
#              1630338450,
#              "67.79661016949153"
#            ],
#            [
#              1630338480,
#              "0"
#            ]
#          ]
#        }
#      ]
#    }
#  }
#}
