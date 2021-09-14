#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# Each variable or function that is created or modified is given the export attribute
# and marked for export to the environment of subsequent commands. 
set -o allexport
source "$SCRIPT_DIR/run-parameters.env"
set +o allexport

$@
