#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
# Copyright Authors of Cilium

set -ex



export IMG=/home/korniltsev/pyro/pyroscope/ebpf/.tmp/ebpf/vm_image_amd64
export IMG=/Users/korniltsev/pyro/pyroscope/ebpf/.tmp/ebpf/vm_image_amd64


if [ -z $ARCH ]; then
  ARCH=amd64
fi

if [ ${ARCH} == "amd64" ]; then
  QEMU=qemu-system-x86_64
elif [ ${ARCH} == "arm64" ]; then
  QEMU=qemu-system-aarch64
else
  echo "Unsupported architecture: ${ARCH}"
  exit 1
fi


if [ -z $KERNEL ]; then
  ARG_KERNEL=""
else
  if [ ${ARCH} == "amd64" ]; then
    ARG_KERNEL="-kernel ${KERNEL} "
  else
    ARG_KERNEL="-kernel ${KERNEL} -append \"root=/dev/vda2 console=ttyAMA0\""
  fi
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
  ARG_KVM="-enable-kvm -cpu kvm64"
fi


connect="ssh -p 2222 -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@localhost"

kill_vm() {
  pkill qemu-system || true
}
#KERNEL=/home/korniltsev/github/linus/bzImageNoModules
#KERNEL=/home/korniltsev/github/linus/linux/arch/x86/boot/bzImage

#    -kernel ${KERNEL} \
#    -append 'root=/dev/sda console=ttyS0 nokaslr' \

run_vm() {
  #kill_vm
  ${QEMU} \
    -nodefaults \
    -nographic \
    -no-reboot \
    -s \
    -smp 4 -m 4G \
    ${ARG_INITRD} \
    ${ARG_IMG} \
    ${ARG_KVM} \
    -net user,hostfwd=tcp::2222-:22 \
    -net nic \
    -serial mon:stdio
  if [ "${ARCH}" == "arm64" ] ; then
    sleep 15
  fi
}

wait_for_ssh() {
  local retries=0
  while ! (${connect} true); do
    if [[ "${retries}" -gt 60 ]]; then
      echo "SSH connection failed after 30 retries"
      kill_vm
      exit 1
    fi
    retries=$((retries + 1))
    sleep 1
  done
}

run_vm
#wait_for_ssh
