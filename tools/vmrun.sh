#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Authors of Cilium

set -ex

IMG=$1
BIN=$2
HOST_ROOT=$(pwd)

if [ -z "$IMG"  ]; then
  echo "Usage: $0 <image> <binary>"
  exit 1
fi
if [ -z "${HOST_ROOT}/${BIN}" ]; then
  echo "Usage: $0 <image> <binary>"
  exit 1
fi

connect="ssh -p 2222 -o StrictHostKeyChecking=no root@localhost"

kill_vm() {
  pkill qemu-system-x86 || true
}

run_vm() {
  kill_vm
  qemu-system-x86_64 \
                  -nodefaults \
                  -display none \
                  -no-reboot \
                  -smp 2 \
                  -m 1G \
                  -enable-kvm \
                  -cpu kvm64 \
                  -hda ${IMG} \
                  -netdev user,id=user.0,hostfwd=tcp::2222-:22 \
                  -device virtio-net-pci,netdev=user.0 \
                  -serial mon:stdio \
                  -device virtio-serial-pci \
                  -fsdev local,id=host_id,path=${HOST_ROOT},security_model=none \
                  -device virtio-9p-pci,fsdev=host_id,mount_tag=host_mount  &
}

wait_for_ssh() {
  local retries=0
  while ! (${connect} true); do
    if [[ "${retries}" -gt 30 ]]; then
      echo "SSH connection failed after 30 retries"
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
