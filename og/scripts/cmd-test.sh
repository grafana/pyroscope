#!/bin/bash

export RUST_BACKTRACE=1

go build -o ./tmp/pyroscope ./cmd/pyroscope && tmp/pyroscope "$@"
