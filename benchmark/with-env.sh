#!/usr/bin/env bash

set -euo pipefail

# Each variable or function that is created or modified is given the export attribute
# and marked for export to the environment of subsequent commands. 
set -o allexport
source ./run-parameters.env
set +o allexport

$@
