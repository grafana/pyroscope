GOBUILD=go build -trimpath

ARCH ?= $(shell arch)
OS ?= $(shell uname)

ifeq ("$(OS)", "Darwin")
	ifeq ("$(ARCH)", "arm64")
# on a mac it's called arm64 which rust doesn't know about
# see https://unix.stackexchange.com/questions/461179/what-is-the-difference-between-different-implemetation-of-arm64-aarch64-for-linu
		ARCH=aarch64
# this makes it work better on M1 machines
		GODEBUG=asyncpreemptoff=1
	endif
endif

ALL_SPIES = ebpfspy,rbspy,pyspy,dotnetspy,debugspy
ifeq ("$(OS)", "Linux")
	ENABLED_SPIES ?= ebpfspy,rbspy,pyspy,phpspy,dotnetspy
else
	ENABLED_SPIES ?= rbspy,pyspy
endif

ifeq ("$(OS)", "Linux")
	THIRD_PARTY_DEPENDENCIES ?= "build-rust-dependencies build-phpspy-dependencies"
else
	THIRD_PARTY_DEPENDENCIES ?= "build-rust-dependencies"
endif

GO_TAGS = $(ENABLED_SPIES)

ifeq ("$(OS)", "Linux")
	ifeq ("$(shell cat /etc/os-release | grep ^ID=)", "ID=alpine")
		RUST_TARGET ?= "$(ARCH)-unknown-linux-musl"
		GO_TAGS := $(GO_TAGS),musl
	else
		RUST_TARGET ?= "$(ARCH)-unknown-linux-gnu"
	endif
else
	ifeq ("$(OS)", "Darwin")
		RUST_TARGET ?= "$(ARCH)-apple-darwin"
	endif
endif

EMBEDDED_ASSETS ?= ""
EMBEDDED_ASSETS_DEPS ?= "assets-release"
EXTRA_LDFLAGS ?= ""

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH || true)
	export GOPATH
endif

-include .env
export

PYROSCOPE_LOG_LEVEL ?= debug
PYROSCOPE_BADGER_LOG_LEVEL ?= error
PYROSCOPE_STORAGE_PATH ?= tmp/pyroscope-storage

.PHONY: all 
all: build ## Runs the build target

.PHONY: build   
build: ## Builds the binary
	$(GOBUILD) -tags "$(GO_TAGS)" -ldflags "$(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o ./bin/pyroscope ./cmd/pyroscope

.PHONY: build-release 
build-release: embedded-assets ## Builds the release build
	EMBEDDED_ASSETS=true $(MAKE) build

.PHONY: build-rust-dependencies
build-rust-dependencies: ## Builds the rust dependency
	(cd third_party/rustdeps && RUSTFLAGS="-C target-feature=+crt-static" cargo build --release --target $(RUST_TARGET)) || $(MAKE) print-deps-error-message

.PHONY: build-phpspy-dependencies
build-phpspy-dependencies: ## Builds the PHP dep
	cd third_party && cd phpspy_src || (git clone https://github.com/pyroscope-io/phpspy.git phpspy_src && cd phpspy_src)
	cd third_party/phpspy_src && git checkout 024461fbba5130a1dc7fd4f0b5a458424cf50b3a
	cd third_party/phpspy_src && USE_ZEND=1 make CFLAGS="-DUSE_DIRECT" || $(MAKE) print-deps-error-message
	cp third_party/phpspy_src/libphpspy.a third_party/phpspy/libphpspy.a

.PHONY: build-third-party-dependencies
build-third-party-dependencies: $(shell echo $(THIRD_PARTY_DEPENDENCIES)) ## Builds third party dep

.PHONY: test
test: ## Runs the test suite
	go test -race -tags debugspy ./...

.PHONY: server
server: ## Start the Pyroscope Server
	bin/pyroscope server

.PHONY: install-web-dependencies
install-web-dependencies: ## Install the web dependencies
	yarn install

.PHONY: assets
assets: install-web-dependencies ## Configure the assets
	$(shell yarn bin webpack) --config scripts/webpack/webpack.dev.js

.PHONY: assets-watch
assets-watch: install-web-dependencies ## Configure the assets with live reloading
	$(shell yarn bin webpack) --config scripts/webpack/webpack.dev.js --watch

.PHONY: assets
assets-release: install-web-dependencies ## Configure the assets for release
	rm -rf webapp/public
	NODE_ENV=production $(shell yarn bin webpack) --config scripts/webpack/webpack.prod.js

.PHONY: embedded-assets
embedded-assets: install-dev-tools $(shell echo $(EMBEDDED_ASSETS_DEPS)) ## Configure the assets along with dev tools
	go run "$(shell scripts/pinned-tool.sh github.com/markbates/pkger)/cmd/pkger" -o pkg/server

.PHONY: lint
lint: ## Run the lint across the codebase
	go run "$(shell scripts/pinned-tool.sh github.com/mgechev/revive)" -config revive.toml -exclude ./pkg/agent/pprof/... -exclude ./vendor/... -formatter stylish ./...

.PHONY: lint-summary
lint-summary: ## Get the lint summary
	$(MAKE) lint | grep 'https://revive.run' | sed 's/[ ()0-9,]*//' | sort

.PHONY: ensure-logrus-not-used
ensure-logrus-not-used: ## Verify if logrus not used in codebase
	@! go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -nostdlib -s ./pkg/agent/profiler/ | grep ' -> "github.com/sirupsen/logrus' \
		|| (echo "\n^ ERROR: make sure ./pkg/agent/profiler/ does not depend on logrus. We don't want users' logs to be tainted. Talk to @petethepig if have questions\n" &1>2; exit 1)

.PHONY: unused
unused: ## Staticcheck for unused code
	staticcheck -f stylish -tags $(ALL_SPIES) -unused.whole-program ./...

.PHONY: install-dev-tools
install-dev-tools: ## Install dev tools
	cat tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI {} go install {}

.PHONY: dev
dev: ## dev
	goreman -exit-on-error -f scripts/dev-procfile start

.PHONY: godoc
godoc: ## Generate godoc
	sleep 5 && open http://localhost:8090/pkg/github.com/pyroscope-io/pyroscope/ &
	godoc -http :8090

.PHONY: go-deps-graph
go-deps-graph: ## Generate the deps graph
	sh scripts/dependency-graph.sh
	open -a "/Applications/Google Chrome.app" tmp/go-deps-graph.svg

.PHONY: clean
clean: ## Clean up storage
	rm -rf tmp/pyroscope-storage

.PHONY: update-contributors
update-contributors: ## Update the contributors
	$(shell yarn bin contributor-faces) \
		-e pyroscopebot \
		-l 100 \
		.

.PHONY: update-changelog
update-changelog: ## Update the changelog
	$(shell yarn bin conventional-changelog) -i CHANGELOG.md -s
	sed -i '/Updates the list of contributors in README/d' CHANGELOG.md
	sed -i '/Update README.md/d' CHANGELOG.md

.PHONY: update-protobuf
update-protobuf: ## Update the protobuf
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	protoc --go_out=. pkg/convert/profile.proto

.PHONY: docker-dev
docker-dev: ## Build the docker dev
	docker build . --tag pyroscope/pyroscope:dev

.PHONY: windows-dev
windows-dev: ## Build the windows dev
	docker build --platform linux/amd64 -f Dockerfile.windows --output type=local,dest=out .

.PHONY: print-deps-error-message
print-deps-error-message:
	@echo ""
	@echo "  NOTE: you can still build pyroscope without spies by adding ENABLED_SPIES=none before the build command:"
	@echo "  $$ ENABLED_SPIES=none make build"
	@echo ""
	exit 1

help: ## Show this help
	@egrep '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 }'
