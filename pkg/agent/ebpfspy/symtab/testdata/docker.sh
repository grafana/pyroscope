#!/bin/bash
set -ex

gcc src.c -o elf
gcc src.c -no-pie -o elf.nopie
objcopy --only-keep-debug elf elf.debug
strip elf -o elf.stripped
objcopy --add-gnu-debuglink=elf.debug elf.stripped elf.debuglink

strip --remove-section .note.gnu.build-id elf.debuglink -o elf.debuglink


build_id=$(readelf -n elf | grep 'Build ID' | awk '{print $3}')
dir=${build_id:0:2}
file=${build_id:2}
mkdir -p "/usr/lib/debug/.build-id/$dir"
cp elf.debug "/usr/lib/debug/.build-id/$dir/$file.debug"