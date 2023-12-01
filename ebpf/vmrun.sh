#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Authors of Cilium

set -ex


if [ -z $ARCH ]; then
  ARCH=amd64
fi

if [ -z $KERNEL ]; then
  ARG_KERNEL=""
else
  ARG_KERNEL="-kernel ${KERNEL}"
fi

if [ -z $INITRD ]; then
  ARG_INITRD=""
else
  ARG_INITRD="-initrd ${INITRD}"
fi

if [ -z $IMG ]; then
  ARG_IMG=""
else
  ARG_IMG="-hda ${IMG}"
fi

if [[ -z "${GITHUB_ACTIONS}" && -e "/dev/kvm" ]]; then
  KVM_ARGS="-enable-kvm -cpu kvm64"
fi


connect="ssh -p 2222 -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@localhost"

kill_vm() {
  pkill qemu-system || true
}

run_vm() {
  kill_vm
  if [[ "${ARCH}" -eq "amd64" ]] ; then
    qemu-system-x86_64 \
      -nodefaults \
      -nographic \
      -no-reboot \
      -smp 4 \
      -m 4G \
      ${ARG_INITRD} \
      ${ARG_KERNEL} \
      ${ARG_IMG} \
      ${KVM_ARGS} \
      -netdev user,id=user.0,hostfwd=tcp::2222-:22 \
      -device virtio-net-pci,netdev=user.0 \
      -append "root=/dev/sda console=ttyS0" \
      -serial mon:stdio
  else
    qemu-system-aarch64 \
      -nodefaults \
      -nographic \
      -no-reboot \
      -M virt -cpu cortex-a57 -smp 4  -m 4G \
      ${ARG_INITRD} \
      ${ARG_KERNEL} \
      ${ARG_IMG} \
      -append "root=/dev/vda2 console=ttyAMA0" \
      -net user,hostfwd=tcp::2222-:22 -net nic \
      -device intel-hda -device hda-duplex \
      sleep 15
  fi
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
