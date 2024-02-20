TMP_EBPF := $(shell pwd)/.tmp/ebpf
QEMU_ARCH ?= amd64

KVM_ARGS ?= -enable-kvm -cpu host
ifeq ($(QEMU_ARCH),amd64)
QEMU_BIN = qemu-system-x86_64 -M pc   -append "root=/dev/vda console=ttyS0  noresume init=/usr/sbin/init"
KERNEL = testdata/qemu_img/amd64/vmlinuz-6.1.0-18-amd64
INITRD = testdata/qemu_img/amd64/initrd.img-6.1.0-18-amd64
DISK = testdata/qemu_img/amd64/disk.ext4
else ifeq ($(QEMU_ARCH),arm64)
QEMU_BIN=qemu-system-aarch64 -M virt -cpu cortex-a57  -append "root=/dev/vda2 console=ttyAMA0 noresume"
#INITRD=$(TMP_EBPF)/initrd.img-5.10.0-26-arm64
#KERNEL=$(TMP_EBPF)/vmlinuz-5.10.0-26-arm64
#DISK=$(TMP_EBPF)/debian-5.10-aarch64.qcow2
else
$(error "Unknown QEMU_ARCH: $(QEMU_ARCH)")
endif

#$(TMP_EBPF)/vm_image_amd64: Makefile
#	mkdir -p $(TMP_EBPF)
#	docker run -v $(TMP_EBPF):/mnt/images \
#		quay.io/lvh-images/kind:6.0-main \
#		cp /data/images/kind_6.0.qcow2.zst /mnt/images/vm_image_amd64.zst
#	zstd -f -d $(TMP_EBPF)/vm_image_amd64.zst
#
#$(TMP_EBPF)/vm_image_arm64: Makefile
#	mkdir -p $(TMP_EBPF)
#	docker run -v $(TMP_EBPF):/mnt/images \
#		pyroscope/ebpf-test-vm-image:debian-5.10-aarch64 \
#		cp debian-5.10-aarch64.qcow2.zst vmlinuz-5.10.0-26-arm64 initrd.img-5.10.0-26-arm64 /mnt/images
#	zstd -f -d $(TMP_EBPF)/debian-5.10-aarch64.qcow2.zst
#	mv -f $(TMP_EBPF)/debian-5.10-aarch64.qcow2.zst $(TMP_EBPF)/vm_image_arm64


#.PHONY: go/test/amd64
#go/test/amd64: $(TMP_EBPF)/vm_image_amd64 ebpf.amd64.test
#	bash vmrun_amd64.sh $(TMP_EBPF)/vm_image_amd64 ebpf.amd64.test
#
#.PHONY: go/test/arm64
#go/test/arm64: $(TMP_EBPF)/vm_image_arm64 ebpf.arm64.test
#	bash vmrun_arm64.sh \
#		$(TMP_EBPF)/vmlinuz-5.10.0-26-arm64 \
#		$(TMP_EBPF)/initrd.img-5.10.0-26-arm64  \
#		$(TMP_EBPF)/debian-5.10-aarch64.qcow2 \
#		ebpf.arm64.test

.PHONY: qemu/kill
qemu/kill:
	pkill qemu-system || true

.PHONY: qemu/start
qemu/start: # todo add dependencies
	  $(QEMU_BIN) $(KVM_ARGS) \
        -smp 4  \
        -m 4G \
        -initrd $(INITRD) \
        -kernel $(KERNEL) \
        -drive if=virtio,file=${DISK},format=raw,id=hd \
        -net user,hostfwd=tcp::2222-:22 \
        -net nic \
        -device intel-hda \
        -device hda-duplex \
        -nographic \
         &


SSH_CMD="ssh -p 2222 -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no root@localhost"
#
#wait_for_ssh() {
#  local retries=0
#  while ! (${connect} true); do
#    if [[ "${retries}" -gt 30 ]]; then
#      echo "SSH connection failed after 30 retries"
#      exit 1
#    fi
#    retries=$((retries + 1))
#    sleep 1
#  done
#}

.PHONY: qemu/wait
qemu/wait:
	@echo "Waiting for SSH to be available"
	@${SSH_CMD} true
	@echo "SSH is available"