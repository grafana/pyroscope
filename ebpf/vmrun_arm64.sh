#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Authors of Cilium

set -ex


KERNEL=$1
INITRD=$2
IMG=$3
BIN=$4
HOST_ROOT=$(pwd)

function usage() {
  echo "Usage: $0 <kernel> <initrd> <image> <binary>"
  exit 1
}
if [ -z "$IMG"  ]; then
  usage
fi
if [ -z "$KERNEL"  ]; then
  usage
fi
if [ -z "$INITRD"  ]; then
  usage
fi
if [ -z "${HOST_ROOT}/${BIN}" ]; then
  usage
fi


connect="ssh -p 2222 -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@localhost"

kill_vm() {
  pkill qemu-system || true
}

run_vm() {
  kill_vm
  qemu-system-aarch64 \
    -no-reboot \
    -M virt -cpu cortex-a57 -smp 4  -m 4G \
    -initrd ${INITRD} \
    -kernel ${KERNEL} \
    -append "root=/dev/vda2 console=ttyAMA0" \
    -drive if=virtio,file=${IMG},format=qcow2,id=hd \
    -net user,hostfwd=tcp::2222-:22 -net nic \
    -device intel-hda -device hda-duplex \
    -nographic \
    -fsdev local,id=host_id,path=${HOST_ROOT},security_model=none \
    -device virtio-9p-pci,fsdev=host_id,mount_tag=host_mount >/dev/null 2>/dev/null &

    sleep 15
}

wait_for_ssh() {
  local retries=0
  while ! (${connect} true); do
    if [[ "${retries}" -gt 30 ]]; then
      echo "SSH connection failed after 30 retries"
      kill_vm
      exit 1
    fi
    retries=$((retries + 1))
    sleep 1
  done
}

run_vm
wait_for_ssh

${connect} "/host/${BIN}"

kill_vm
