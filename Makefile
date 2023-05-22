GOBUILD=go build -trimpath

ARCH ?= $(shell uname -m)
OS ?= $(shell uname)


# if you change the name of this variable please change it in generate-git-info.sh file
PHPSPY_VERSION ?= 66b6fdb2f9da1d87912b46b7faf68796d471c209

ifeq ("$(OS)", "Darwin")
	ifeq ("$(ARCH)", "arm64")
# on a mac it's called arm64 which rust doesn't know about
# see https://unix.stackexchange.com/questions/461179/what-is-the-difference-between-different-implemetation-of-arm64-aarch64-for-linu
		ARCH=aarch64
# this makes it work better on M1 machines
		GODEBUG=asyncpreemptoff=1
	endif
endif

ALL_SPIES = ebpfspy,dotnetspy,phpspy,debugspy
ifeq ("$(OS)", "Linux")
	ENABLED_SPIES_RELEASE ?= ebpfspy,phpspy,dotnetspy
else
	ENABLED_SPIES_RELEASE ?= dotnetspy
endif
ENABLED_SPIES ?= none

ifeq ("$(OS)", "Linux")
	THIRD_PARTY_DEPENDENCIES ?= "build-phpspy-dependencies"
else
	THIRD_PARTY_DEPENDENCIES ?= ""
endif

EXTRA_GO_TAGS ?=
CGO_CFLAGS ?=
CGO_LDFLAGS ?=
EXTRA_CGO_CFLAGS ?=
EXTRA_CGO_LDFLAGS ?=
GO_TAGS = $(ENABLED_SPIES)$(EXTRA_GO_TAGS)
ALPINE_TAG =

ifneq (,$(findstring ebpfspy,$(GO_TAGS)))
	EXTRA_CGO_CFLAGS := $(EXTRA_CGO_CFLAGS) -I$(abspath ./third_party/libbpf/lib/include) \
		-I$(abspath ./third_party/bcc/lib/include)
	EXTRA_CGO_LDFLAGS := $(EXTRA_CGO_LDFLAGS)  -L$(abspath ./third_party/libbpf/lib/lib64) -lbpf \
		-L$(abspath ./third_party/bcc/lib/lib) -lbcc-syms -lstdc++ -lelf -lz
	THIRD_PARTY_DEPENDENCIES := $(THIRD_PARTY_DEPENDENCIES) build-profile-bpf build-bcc build-libbpf
endif

ifeq ("$(OS)", "Linux")
	ifeq ("$(shell cat /etc/os-release | grep ^ID=)", "ID=alpine")
		GO_TAGS := $(GO_TAGS),musl
		ALPINE_TAG := ,musl
	else

	endif
else
	ifeq ("$(OS)", "Darwin")

	endif
endif

OPEN=
ifeq ("$(OS)", "Linux")
	OPEN=xdg-open
else
  OPEN=open
endif

EMBEDDED_ASSETS_DEPS ?= "assets-release"
EXTRA_LDFLAGS ?=

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

.PHONY: build-phpspy-static-library
build-phpspy-static-library: ## builds phpspy static library
	mkdir -p ./out
	$(GOBUILD) -tags nogospy,phpspy,clib$(ALPINE_TAG) -ldflags "$(shell scripts/generate-build-flags.sh false)" -buildmode=c-archive -o "./out/libpyroscope.phpspy.a" ./pkg/agent/clib
ifeq ("$(OS)", "Linux")
	LC_CTYPE=C LANG=C strip --strip-debug ./out/libpyroscope.phpspy.a
	ranlib ./out/libpyroscope.phpspy.a
endif

.PHONY: install-go-dependencies
install-go-dependencies: ## installs golang dependencies
	go mod download

.PHONY: build
build: ## Builds the binary
	CGO_CFLAGS="$(CGO_CFLAGS) $(EXTRA_CGO_CFLAGS)" \
		CGO_LDFLAGS="$(CGO_LDFLAGS) $(EXTRA_CGO_LDFLAGS)" \
		$(GOBUILD) -tags "$(GO_TAGS)" -ldflags "$(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh)" -o ./bin/pyroscope ./cmd/pyroscope

.PHONY: build-release
build-release: embedded-assets ## Builds the release build
	EXTRA_GO_TAGS=,embedassets,$(ENABLED_SPIES_RELEASE) $(MAKE) build

.PHONY: build-panel
build-panel:
	NODE_ENV=production $(shell yarn bin webpack) --config scripts/webpack/webpack.panel.js

.PHONY: build-phpspy-dependencies
build-phpspy-dependencies: ## Builds the PHP dependency
	cd third_party && cd phpspy_src || (git clone https://github.com/pyroscope-io/phpspy.git phpspy_src && cd phpspy_src)
	cd third_party/phpspy_src && git checkout $(PHPSPY_VERSION)
	cd third_party/phpspy_src && make clean static
	cp third_party/phpspy_src/libphpspy.a third_party/phpspy/libphpspy.a

.PHONY: build-libbpf
build-libbpf:
	$(MAKE) -C third_party/libbpf

.PHONY: build-bcc
build-bcc:
	$(MAKE) -C third_party/bcc

.PHONY: build-profile-bpf
build-profile-bpf: build-libbpf
	CFLAGS="-I$(abspath ./third_party/libbpf/lib/include)" $(MAKE) -C pkg/agent/ebpfspy/bpf


.PHONY: build-third-party-dependencies
build-third-party-dependencies: $(shell echo $(THIRD_PARTY_DEPENDENCIES)) ## Builds third party dep

.PHONY: test
test: ## Runs the test suite
	go test -race -tags debugspy $(shell go list ./... | grep -v /examples/)

.PHONY: coverage
coverage: ## Runs the test suite with coverage
	go test -race -tags debugspy -coverprofile=coverage -covermode=atomic $(shell go list ./... | grep -v /examples/)

.PHONY: server
server: ## Start the Pyroscope Server
	bin/pyroscope server $(SERVERPARAMS)

.PHONY: install-web-dependencies
install-web-dependencies: ## Install the web dependencies
	yarn install --ignore-engines

.PHONY: install-build-web-dependencies
install-build-web-dependencies: ## Install web dependencies only necessary for a build
	NODE_ENV=production yarn install --frozen-lockfile --ignore-engines

.PHONY: assets
assets: install-web-dependencies ## deprecated
	@echo "This command is deprecated, please use `make dev` to develop locally"
	exit 1
	# yarn dev

.PHONY: assets-watch
assets-watch: install-web-dependencies ## deprecated
	@echo "This command is deprecated, please use `make dev` to develop locally"
	exit 1
	# yarn dev -- --watch

.PHONY: assets-release
assets-release: ## Configure the assets for release
	rm -rf webapp/public/assets
	rm -rf webapp/public/*.html
	yarn build

.PHONY: assets-size-build
assets-size-build: assets-release ## Build assets for the size report
	mv webapp/public/assets/app*.js webapp/public/assets/app.js

.PHONY: embedded-assets
embedded-assets: install-dev-tools $(shell echo $(EMBEDDED_ASSETS_DEPS)) ## Configure the assets along with dev tools

.PHONY: lint
lint: ## Run the lint across the codebase
	go run "$(shell scripts/pinned-tool.sh github.com/mgechev/revive)" -config revive.toml -exclude ./pkg/agent/pprof/... -exclude ./vendor/... -exclude ./examples/... -formatter stylish ./...

.PHONY: lint-summary
lint-summary: ## Get the lint summary
	$(MAKE) lint | grep 'https://revive.run' | sed 's/[ ()0-9,]*//' | sort

.PHONY: ensure-logrus-not-used
ensure-logrus-not-used: ## Verify if logrus not used in codebase
	@! go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -nostdlib -s ./pkg/agent/profiler/ | grep ' -> "github.com/sirupsen/logrus' \
		|| (echo "\n^ ERROR: make sure ./pkg/agent/profiler/ does not depend on logrus. We don't want users' logs to be tainted. Talk to @petethepig if have questions\n" &1>2; exit 1)

	@! go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -nostdlib -s ./pkg/agent/clib/ | grep ' -> "github.com/sirupsen/logrus' \
		|| (echo "\n^ ERROR: make sure ./pkg/agent/clib/ does not depend on logrus. We don't want users' logs to be tainted. Talk to @petethepig if have questions\n" &1>2; exit 1)

.PHONY: clib-deps
clib-deps:
	go run "$(shell scripts/pinned-tool.sh github.com/kisielk/godepgraph)" -tags nogospy ./pkg/agent/clib/ | dot -Tsvg -o ./tmp/clib-deps.svg

.PHONY: unused
unused: ## Staticcheck for unused code
	staticcheck -f stylish -tags $(ALL_SPIES) -unused.whole-program ./...

.PHONY: install-dev-tools
install-dev-tools: ## Install dev tools
	go install github.com/cosmtrek/air@latest
	cat tools/tools.go | grep _ | awk -F'"' '{print $$2}' | xargs -tI {} go install {}

.PHONY: web-bootstrap
web-bootstrap: install-web-dependencies
	yarn bootstrap
# build webapp just to get its dependencies built
# otherwise when first running, the webapp will fail to build since its deps don't exist yet
	yarn build:webapp > /dev/null

.PHONY: dev
dev: install-web-dependencies ## Start webpack and pyroscope server. Use this one for working on pyroscope
	PYROSCOPE_ANALYTICS_OPT_OUT=true goreman -exit-on-error -f scripts/dev-procfile start

.PHONY: godoc
godoc: ## Generate godoc
	sleep 5 && $(OPEN) http://localhost:8090/pkg/github.com/pyroscope-io/pyroscope/ &
	godoc -http :8090

.PHONY: go-deps-graph
go-deps-graph: ## Generate the deps graph
	sh scripts/dependency-graph.sh
	open -a "/Applications/Google Chrome.app" tmp/go-deps-graph.svg

.PHONY: clean
clean: ## Clean up storage
	rm -rf tmp/pyroscope-storage
	$(MAKE) -C third_party/bcc clean
	$(MAKE) -C third_party/libbpf clean
	$(MAKE) -C pkg/agent/ebpfspy/bpf clean

.PHONY: update-contributors
update-contributors: ## Update the contributors
	$(shell yarn bin contributor-faces) \
		-e pyroscopebot \
		-l 100 \
		.

.PHONY: preview-changelog
preview-changelog: ## Update the changelog
	$(shell yarn bin conventional-changelog) -i CHANGELOG.md -p angular -u

.PHONY: update-changelog
update-changelog: ## Update the changelog
	$(shell yarn bin conventional-changelog) -i CHANGELOG.md -p angular -s
	sed -i '/Updates the list of contributors in README/d' CHANGELOG.md
	sed -i '/docs: updates the list of contributors in README/d' CHANGELOG.md
	sed -i '/Update README.md/d' CHANGELOG.md

.PHONY: update-protobuf
update-protobuf: ## Update the protobuf
	go install google.golang.org/protobuf/cmd/protoc-gen-go
	protoc --go_out=. pkg/convert/profile.proto

.PHONY: docker-dev
docker-dev: ## Build the docker dev
	docker build . --tag pyroscope/pyroscope:dev --progress=plain

.PHONY: windows-dev
windows-dev: ## Build the windows dev
	docker build --platform linux/amd64 -f Dockerfile.windows --progress=plain --output type=local,dest=out .

.PHONY: print-deps-error-message
print-deps-error-message:
	@echo ""
	@echo "  NOTE: you can still build pyroscope without spies by adding ENABLED_SPIES=none before the build command:"
	@echo "  $$ ENABLED_SPIES=none make build"
	@echo ""
	exit 1

.PHONY: e2e-build
e2e-build: build assets-release

.PHONY: test-merge
test-merge:
	curl --data 'foo;bar 100' http://localhost:4040/ingest?name="foo%7Bprofile_id=id1%7D"
	curl --data 'foo;baz 200' http://localhost:4040/ingest?name="foo%7Bprofile_id=id2%7D"
	@echo http://localhost:4040/tracing?queryID=$(shell curl --data '{"appName":"foo", "profiles":["id1", "id2"], "startTime":"now-1h", "endTime":"now"}' http://localhost:4040/merge | jq .queryID | tr -d '"')

help: ## Show this help
	@egrep '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | sed 's/Makefile://' | awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n\nTargets:\n"} /^[a-z0-9A-Z_-]+:.*?##/ { printf "  \033[36m%-30s\033[0m %s\n", $$1, $$2 }'
