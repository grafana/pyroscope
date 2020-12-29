GOBUILD=go build -trimpath
GODEBUG=asyncpreemptoff=1

ifeq ("$(shell go env GOARCH || true)", "arm64")
	ENABLED_SPIES ?= "pyspy"
else
	ENABLED_SPIES ?= "rbspy,pyspy"
endif

EMBEDDED_ASSETS ?= ""
EMBEDDED_ASSETS_DEPS ?= "assets-release"
EXTRA_LDFLAGS ?= ""

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH || true)
	export GOPATH
endif

.PHONY: all
all: build

.PHONY: build
build:
	$(GOBUILD) -tags $(ENABLED_SPIES) -ldflags "$(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o ./bin/pyroscope ./cmd/pyroscope

.PHONY: build-release
build-release: embedded-assets
	EMBEDDED_ASSETS=true $(MAKE) build

.PHONY: build-rust-dependencies
build-rust-dependencies:
	cd third_party/rustdeps && cargo build --release

.PHONY: test
test:
	go list ./... | xargs -I {} sh -c "go test {} || exit 255"

.PHONY: server
server:
	bin/pyroscope server --log-level debug --badger-log-level error --storage-path tmp/pyroscope-storage

.PHONY: agent
agent:
	bin/pyroscope agent

.PHONY: install-web-dependencies
install-web-dependencies:
	yarn install

.PHONY: assets
assets: install-web-dependencies
	$(shell yarn bin webpack) --config scripts/webpack/webpack.dev.js

.PHONY: assets-watch
assets-watch: install-web-dependencies
	$(shell yarn bin webpack) --config scripts/webpack/webpack.dev.js --watch

.PHONY: assets
assets-release: install-web-dependencies
	rm -rf /webapp/public
	$(shell yarn bin webpack) --config scripts/webpack/webpack.prod.js

.PHONY: embedded-assets
embedded-assets: install-dev-tools $(shell echo $(EMBEDDED_ASSETS_DEPS))
	$(GOPATH)/bin/pkger -o pkg/server

.PHONY: lint
lint:
	revive -config revive.toml -formatter stylish ./...

.PHONY: unused
unused:
	staticcheck -f stylish -unused.whole-program ./...

.PHONY: install-dev-tools
install-dev-tools:
	cat tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI {} go install {}

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

.PHONY: docker-dev
docker-dev:
	docker build . --tag pyroscope/pyroscope:dev
