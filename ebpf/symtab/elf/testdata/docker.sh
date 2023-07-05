#!/bin/bash
set -ex

gcc lib.c -o libexample.so -shared
gcc src.c -o elf -lexample -L. -Wl,-rpath=.
gcc src.c -no-pie -o elf.nopie -lexample -L. -Wl,-rpath=.
objcopy --only-keep-debug elf elf.debug
strip elf -o elf.stripped
objcopy --add-gnu-debuglink=elf.debug elf.stripped elf.debuglink

strip --remove-section .note.gnu.build-id elf.debuglink -o elf.debuglink
objcopy --remove-section .note.gnu.build-id elf elf.nobuildid


build_id=$(readelf -n elf | grep 'Build ID' | awk '{print $3}')
dir=${build_id:0:2}
file=${build_id:2}
mkdir -p "/usr/lib/debug/.build-id/$dir"
cp elf.debug "/usr/lib/debug/.build-id/$dir/$file.debug"