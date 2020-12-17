GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOBUILD=go build -trimpath
ENABLED_SPIES ?= "rbspy,pyspy"
EMBEDDED_ASSETS_DEPS ?= "assets"
EMBEDDED_ASSETS ?= ""
DOCKER_ARCHES ?= "linux/amd64,linux/arm64"
GODEBUG=asyncpreemptoff=1
DOCKER_TAG ?= "dev"

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH)
	export GOPATH
endif

.PHONY: all
all: build

.PHONY: build
build:
	$(GOBUILD) -tags $(ENABLED_SPIES) -ldflags "$(shell scripts/generate-build-flags.sh $EMBEDDED_ASSETS)" -o ./bin/pyroscope ./cmd/pyroscope

third_party/rbspy/librbspy.a:
	cd ../rbspy/ && make build
	cp ../rbspy/target/release/librbspy.a third_party/rbspy/librbspy.a

third_party/pyspy/libpyspy.a:
	cd ../py-spy/ && make build
	cp ../py-spy/target/release/libpy_spy.a third_party/pyspy/libpyspy.a

.PHONY: build-rust-dependencies
build-rust-dependencies: third_party/rbspy/librbspy.a third_party/pyspy/libpyspy.a

.PHONY: test
test:
	go list ./... | xargs -I {} sh -c "go test {} || exit 255"

.PHONY: server
server:
	bin/pyroscope server --log-level debug --badger-log-level error

.PHONY: agent
agent:
	bin/pyroscope agent

.PHONY: install-web-dependencies
install-web-dependencies:
	yarn install

.PHONY: assets
assets: install-web-dependencies
	$(shell yarn bin webpack) --config scripts/webpack/webpack.js

.PHONY: assets-watch
assets-watch: install-web-dependencies
	$(shell yarn bin webpack) --config scripts/webpack/webpack.js --watch

.PHONY: lint
lint:
	revive -config revive.toml -formatter stylish ./...

.PHONY: unused
unused:
	staticcheck -f stylish -unused.whole-program ./...

.PHONY: install-dev-tools
install-dev-tools:
	cat tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI {} go install {}

.PHONY: clear-storage
clear-storage:
	rm -rf tmp/pyroscope-storage

.PHONY: dev
dev:
	goreman -exit-on-error -f scripts/dev-procfile start

.PHONY: godoc
godoc:
	sleep 5 && open http://localhost:8090/pkg/github.com/pyroscope-io/pyroscope/ &
	godoc -http :8090

.PHONY: go-deps-graph
go-deps-graph:
	sh scripts/dependency-graph.sh
	open -a "/Applications/Google Chrome.app" tmp/go-deps-graph.svg

.PHONY: clean
clean:
	rm -rf tmp/pyroscope-storage

.PHONY: update-contributors
update-contributors:
	$(shell yarn bin contributor-faces) .

# Release-related tasks:

.PHONY: embedded-assets
embedded-assets: install-dev-tools $(EMBEDDED_ASSETS_DEPS)
	pkger -o pkg/server

.PHONY: build-release
build-release: embedded-assets
	EMBEDDED_ASSETS=true $(MAKE) build

.PHONY: docker-build
docker-build:
	docker build .

.PHONY: docker-build-x-setup
docker-build-x-setup:
	docker buildx create --use

.PHONY: docker-build-all-arches
docker-build-all-arches:
	docker buildx build \
		--tag pyroscope/pyroscope:$(DOCKER_TAG) \
		--push \
		--platform $(DOCKER_ARCHES) \
		.

