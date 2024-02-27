GO ?= go
RIDESHARE_REPO ?= korniltsev
RIDESHARE="testdata/rideshare-flask-no-pip"

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

.phony: musl/dwarfdump
musl/dwarfdump:
	git submodule update --init --recursive

	echo "//go:build amd64 && linux" > python/musl_offsets_gen_amd64.go
	go run cmd/musl_dwarfdump/main.go $(shell find testdata/alpine-amd64 -name ld-musl-x86_64.so.1.debug ) >> python/musl_offsets_gen_amd64.go
	go fmt python/musl_offsets_gen_amd64.go

	echo "//go:build arm64 && linux" > python/musl_offsets_gen_arm64.go
	go run cmd/musl_dwarfdump/main.go $(shell find testdata/alpine-arm64 -name ld-musl-aarch64.so.1.debug) >> python/musl_offsets_gen_arm64.go
	go fmt python/musl_offsets_gen_arm64.go

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