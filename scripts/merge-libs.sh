#!/bin/bash

set -e

DIR1=$(mktemp -d -t ci-XXXXXXXXXX)
DIR2=$(mktemp -d -t ci-XXXXXXXXXX)

IN1="$(realpath $1)"
IN2="$(realpath $2)"
OUT="$(realpath $3)"

cd "$DIR1" && ar x IN1
cd "$DIR2" && ar x IN2

ar c "$OUT" "$IN1/*.o" "$IN2/*.o"

rm -rf $DIR1
rm -rf $DIR2
