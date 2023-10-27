
.phony: python/dwarfdump
python/dwarfdump:
	git submodule update --init --recursive
	go run cmd/python_dwarfdump/main.go $(shell find testdata -name libpy\*.so\*) > python/versions_gen.go
	go fmt python/versions_gen.go


.phony: bpf/gen
bpf/gen:
	go generate pyrobpf/gen.go
	go generate python/gen.go

RIDESHARE="../examples/python/rideshare/flask"
.phony: rideshare/gen
rideshare/gen:
	docker build -t ebpf-testdata-rideshare-glibc-3.8  --build-arg="PYTHON_VERSION=3.8-slim"  $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.9  --build-arg="PYTHON_VERSION=3.9-slim"  $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.10 --build-arg="PYTHON_VERSION=3.10-slim" $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.11 --build-arg="PYTHON_VERSION=3.11-slim" $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.12 --build-arg="PYTHON_VERSION=3.12-slim" $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.8  --build-arg="PYTHON_VERSION=3.8-slim"  $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.9  --build-arg="PYTHON_VERSION=3.9-slim"  $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.10 --build-arg="PYTHON_VERSION=3.10-slim" $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.11 --build-arg="PYTHON_VERSION=3.11-slim" $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare-glibc-3.12 --build-arg="PYTHON_VERSION=3.12-slim" $(RIDESHARE)