GO ?= go
RIDESHARE_REPO ?= korniltsev
RIDESHARE="testdata/rideshare-flask-no-pip"
TMP_EBPF := $(shell pwd)/.tmp/ebpf

ifeq ($(shell uname -s),Linux)
ifeq ($(shell uname -m),x86_64)
EBPF_GO_TEST_FLAGS_AMD64 = -v -race -cover
EBPF_CGO_ENABLED_AMD64 = 1
EBPF_GO_TEST_FLAGS_ARM64 = -v
EBPF_CGO_ENABLED_ARM64 = 0
else
EBPF_GO_TEST_FLAGS_AMD64 = -v
EBPF_CGO_ENABLED_AMD64 = 0
EBPF_GO_TEST_FLAGS_ARM64 = -v -race -cover
EBPF_CGO_ENABLED_ARM64 = 1
endif # $(shell uname -m),x86_64
else
EBPF_GO_TEST_FLAGS_AMD64 = -v
EBPF_CGO_ENABLED_AMD64 = 0
EBPF_GO_TEST_FLAGS_ARM64 = -v
EBPF_CGO_ENABLED_ARM64 = 0
endif # $(shell uname -s),Linux

EBPF_RUN_IN_VM ?= 0


.phony: python/dwarfdump
python/dwarfdump:
	git submodule update --init --recursive

	echo "//go:build amd64 && linux" > python/python_offsets_gen_amd64.go
	go run cmd/python_dwarfdump/main.go $(shell find testdata/python-x64 -name libpy\*.so\*) \
	 	$(shell find testdata/python-x64  | grep -E "/python3\\.[0-9]+") >> python/python_offsets_gen_amd64.go
	go fmt python/python_offsets_gen_amd64.go

	echo "//go:build arm64 && linux" > python/python_offsets_gen_arm64.go
	go run cmd/python_dwarfdump/main.go $(shell find testdata/python-arm64 -name libpy\*.so\*) \
	 	$(shell find testdata/python-arm64  | grep -E "/python3\\.[0-9]+") >> python/python_offsets_gen_arm64.go
	go fmt python/python_offsets_gen_arm64.go


.phony: glibc/dwarfdump
glibc/dwarfdump:
	git submodule update --init --recursive

	echo "//go:build amd64 && linux" > python/glibc_offsets_gen_amd64.go
	go run cmd/glibc_dwarfdump/main.go $(shell find testdata/glibc-x64 -name libc.so.6 ) >> python/glibc_offsets_gen_amd64.go
	go fmt python/glibc_offsets_gen_amd64.go

	echo "//go:build arm64 && linux" > python/glibc_offsets_gen_arm64.go
	go run cmd/glibc_dwarfdump/main.go $(shell find testdata/glibc-arm64 -name libc.so.6 ) >> python/glibc_offsets_gen_arm64.go
	go fmt python/glibc_offsets_gen_arm64.go

.phony: bpf/gen
bpf/gen:
	go generate pyrobpf/gen.go
	go generate python/gen.go

.PHONY: ebpf.amd64.test
ebpf.amd64.test:
	CGO_ENABLED=$(EBPF_CGO_ENABLED_AMD64) GOOS=linux GOARCH=amd64 \
		$(GO) test -c $(EBPF_GO_TEST_FLAGS_AMD64) -o ebpf.amd64.test ./

.PHONY: ebpf.arm64.test
ebpf.arm64.test:
	CGO_ENABLED=$(EBPF_CGO_ENABLED_ARM64) GOOS=linux GOARCH=arm64 \
		$(GO) test -c $(EBPF_GO_TEST_FLAGS_ARM64) -o ebpf.arm64.test ./

ifeq ($(EBPF_RUN_IN_VM),1)

$(TMP_EBPF)/vm_image_amd64: Makefile
	mkdir -p $(TMP_EBPF)
	docker run -v $(TMP_EBPF):/mnt/images \
		quay.io/lvh-images/kind:6.0-main \
		cp /data/images/kind_6.0.qcow2.zst /mnt/images/vm_image_amd64.zst
	zstd -f -d $(TMP_EBPF)/vm_image_amd64.zst

$(TMP_EBPF)/vm_image_arm64: Makefile
	mkdir -p $(TMP_EBPF)
	docker run -v $(TMP_EBPF):/mnt/images \
		pyroscope/ebpf-test-vm-image:debian-5.10-aarch64 \
		cp debian-5.10-aarch64.qcow2.zst vmlinuz-5.10.0-26-arm64 initrd.img-5.10.0-26-arm64 /mnt/images
	zstd -f -d $(TMP_EBPF)/debian-5.10-aarch64.qcow2.zst
	mv $(TMP_EBPF)/debian-5.10-aarch64.qcow2.zst $(TMP_EBPF)/vm_image_arm64


.PHONY: go/test/amd64
go/test/amd64: $(TMP_EBPF)/vm_image_amd64 ebpf.amd64.test
	bash ../tools/ebpf_vmrun_amd64.sh $(TMP_EBPF)/vm_image_amd64 ebpf.amd64.test

.PHONY: go/test/arm64
go/test/arm64: $(TMP_EBPF)/vm_image_arm64 ebpf.arm64.test
	bash ../tools/ebpf_vmrun_arm64.sh \
		$(TMP_EBPF)/vmlinuz-5.10.0-26-arm64 \
		$(TMP_EBPF)/initrd.img-5.10.0-26-arm64  \
		$(TMP_EBPF)/debian-5.10-aarch64.qcow2 \
		ebpf.arm64.test
else

.PHONY: go/test/amd64
go/test/amd64: ebpf.amd64.test
	whoami | grep root
	uname -m | grep x86_64
	./ebpf.amd64.test


.PHONY: go/test/arm64
go/test/arm64: ebpf.arm64.test
	whoami | grep root
	uname -m | grep aarch64
	./ebpf.arm64.test

endif

.phony: rideshare/gen
rideshare/gen:
	git submodule update --init --recursive
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.8-slim        --build-arg="PYTHON_VERSION=3.8-slim"       $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.9-slim        --build-arg="PYTHON_VERSION=3.9-slim"       $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.10-slim       --build-arg="PYTHON_VERSION=3.10-slim"      $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.11-slim       --build-arg="PYTHON_VERSION=3.11-slim"      $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.12-slim       --build-arg="PYTHON_VERSION=3.12-slim"      $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.13-rc-slim    --build-arg="PYTHON_VERSION=3.13-rc-slim"   $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.8-alpine      --build-arg="PYTHON_VERSION=3.8-alpine"     $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.9-alpine      --build-arg="PYTHON_VERSION=3.9-alpine"     $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.10-alpine     --build-arg="PYTHON_VERSION=3.10-alpine"    $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.11-alpine     --build-arg="PYTHON_VERSION=3.11-alpine"    $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.12-alpine     --build-arg="PYTHON_VERSION=3.12-alpine"    $(RIDESHARE)
	docker buildx build --platform=linux/amd64,linux/arm64 --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.13-rc-alpine  --build-arg="PYTHON_VERSION=3.13-rc-alpine" $(RIDESHARE)