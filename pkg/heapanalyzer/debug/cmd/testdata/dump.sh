#!/usr/bin/env bash

set -ex
GO=/home/korniltsev/sdk/go1.21.7/bin/go
BIN=test_exe
DUMP=coredump


id | grep root
killall "$BIN" || true

"$GO" build -o "$BIN" main.go
"$GO" tool objdump -gnu -s main.loop "$BIN"  > "$BIN.gnu.s"
#"$GO" tool compile -S main.go   > "$BIN.go.s"
"$GO" build -gcflags=-S main.go  2> "$BIN.go.s"
chmod 0666 "$BIN.go.s"
chmod 0666 "$BIN.gnu.s"

"./$BIN" &
sleep 1
PID=$(pgrep $BIN)
gcore -o "$DUMP" "$PID"
mv "$DUMP.$PID" "$DUMP"

