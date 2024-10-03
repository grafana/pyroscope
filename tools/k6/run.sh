#! /bin/bash

set -euo pipefail

usage() {
  echo "Usage: $0 [-c] file"
  echo "    file        File name of the test to run"
  echo "      -c        Run the test in the cloud"
  echo "      -h        Print help message and list all available tests"
}

tests() {
  echo "Available tests:"
  for file in tools/k6/tests/*.js; do
    echo "  $(basename "${file}")"
  done
}

IS_CLOUD=
while getopts "hc" opt; do
  case "${opt}" in
    c)
      IS_CLOUD=1
      ;;
    h)
      usage
      echo # Add a newline.
      tests
      exit 0
      ;;
    *)
      usage
      exit 1
      ;;
  esac
done
shift $((OPTIND-1))

if [ "$#" -lt 1 ]; then
  usage
  exit 1
fi

TEST=$1
if [ -z "${TEST}" ]; then
  usage
  exit 1
fi

DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" ; pwd -P)
source "${DIR}"/.env

if [ -n "${IS_CLOUD}" ]; then
  k6 cloud run tools/k6/tests/"${TEST}" \
    -e "K6_BASE_URL=${K6_BASE_URL}" \
    -e "K6_READ_TOKEN=${K6_READ_TOKEN}" \
    -e "K6_TENANT_ID=${K6_TENANT_ID}" \
    -e "K6_QUERY_SERVICE_NAME=${K6_QUERY_SERVICE_NAME}" \
    -e "K6_QUERY_DURATIONS=${K6_QUERY_DURATIONS}"
else
  K6_BASE_URL="${K6_BASE_URL}" \
  K6_READ_TOKEN="${K6_READ_TOKEN}" \
  K6_TENANT_ID="${K6_TENANT_ID}" \
  K6_QUERY_SERVICE_NAME="${K6_QUERY_SERVICE_NAME}" \
  K6_QUERY_DURATIONS="${K6_QUERY_DURATIONS}" \
  k6 run tools/k6/tests/"${TEST}"
fi
