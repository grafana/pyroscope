#!/bin/bash

# this is basically $(yarn bin <command name>) alternative for golang

go run "$(go list -m -f '{{.Dir}}' $1)" "${@:2}"
