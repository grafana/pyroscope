GO ?= go
RIDESHARE_REPO ?= korniltsev
RIDESHARE="testdata/rideshare-flask-no-pip"
TMP_EBPF := $(shell pwd)/.tmp/ebpf

ifeq ($(shell uname -s),Linux)
EBPF_GO_TEST_FLAGS = -v -race -cover
EBPF_CGO_ENABLED = 1
else
EBPF_GO_TEST_FLAGS = -v
EBPF_CGO_ENABLED = 0
endif

EBPF_RUN_IN_VM ?= 0


.phony: python/dwarfdump
python/dwarfdump:
	#git submodule update --init --recursive

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
	#git submodule update --init --recursive

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

.PHONY: ebpf.test
ebpf.test:
	CGO_ENABLED=$(EBPF_CGO_ENABLED) \
		GOOS=linux GOARCH=amd64 \
		$(GO) test -c $(EBPF_GO_TEST_FLAGS) ./

ifeq ($(EBPF_RUN_IN_VM),1)

$(TMP_EBPF)/vm_image: Makefile
	mkdir -p $(TMP_EBPF)
	docker run -v $(TMP_EBPF):/mnt/images \
		quay.io/lvh-images/kind:6.0-main \
		cp /data/images/kind_6.0.qcow2.zst /mnt/images/vm_image.zst
	zstd -f -d $(TMP_EBPF)/vm_image.zst


.PHONY: go/test
go/test: $(TMP_EBPF)/vm_image ebpf.test
	bash ../tools/vmrun.sh $(TMP_EBPF)/vm_image ebpf.test

else

.PHONY: go/test
go/test: ebpf.test
	whoami | grep root
	./ebpf.test

endif

.phony: rideshare/gen
rideshare/gen:
	git submodule update --init --recursive
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.8-slim        --build-arg="PYTHON_VERSION=3.8-slim"       $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.9-slim        --build-arg="PYTHON_VERSION=3.9-slim"       $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.10-slim       --build-arg="PYTHON_VERSION=3.10-slim"      $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.11-slim       --build-arg="PYTHON_VERSION=3.11-slim"      $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.12-slim       --build-arg="PYTHON_VERSION=3.12-slim"      $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.13-rc-slim    --build-arg="PYTHON_VERSION=3.13-rc-slim"   $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.8-alpine      --build-arg="PYTHON_VERSION=3.8-alpine"     $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.9-alpine      --build-arg="PYTHON_VERSION=3.9-alpine"     $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.10-alpine     --build-arg="PYTHON_VERSION=3.10-alpine"    $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.11-alpine     --build-arg="PYTHON_VERSION=3.11-alpine"    $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.12-alpine     --build-arg="PYTHON_VERSION=3.12-alpine"    $(RIDESHARE)
	docker build --push -t $(RIDESHARE_REPO)/ebpf-testdata-rideshare:3.13-rc-alpine  --build-arg="PYTHON_VERSION=3.13-rc-alpine" $(RIDESHARE)