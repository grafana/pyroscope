
.phony: python/dwarfdump
python/dwarfdump:
	git submodule update --init --recursive
	go run cmd/python_dwarfdump/main.go $(shell find testdata -name libpy\*.so\*) \
	 	$(shell find .  | grep python/ | grep -E "/python3\\.[0-9]+") > python/versions_gen.go
	go fmt python/versions_gen.go


.phony: bpf/gen
bpf/gen:
	go generate pyrobpf/gen.go
	go generate python/gen.go

RIDESHARE="testdata/rideshare-flask-no-pip"

.phony: rideshare/gen
rideshare/gen:
	git submodule update --init --recursive
	docker build -t ebpf-testdata-rideshare:3.8-slim        --build-arg="PYTHON_VERSION=3.8-slim"       $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.9-slim        --build-arg="PYTHON_VERSION=3.9-slim"       $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.10-slim       --build-arg="PYTHON_VERSION=3.10-slim"      $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.11-slim       --build-arg="PYTHON_VERSION=3.11-slim"      $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.12-slim       --build-arg="PYTHON_VERSION=3.12-slim"      $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.13-rc-slim    --build-arg="PYTHON_VERSION=3.13-rc-slim"   $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.8-alpine      --build-arg="PYTHON_VERSION=3.8-alpine"     $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.9-alpine      --build-arg="PYTHON_VERSION=3.9-alpine"     $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.10-alpine     --build-arg="PYTHON_VERSION=3.10-alpine"    $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.11-alpine     --build-arg="PYTHON_VERSION=3.11-alpine"    $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.12-alpine     --build-arg="PYTHON_VERSION=3.12-alpine"    $(RIDESHARE)
	docker build -t ebpf-testdata-rideshare:3.13-rc-alpine  --build-arg="PYTHON_VERSION=3.13-rc-alpine" $(RIDESHARE)