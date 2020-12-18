GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOBUILD=go build -trimpath
GODEBUG=asyncpreemptoff=1

ENABLED_SPIES ?= "rbspy,pyspy"
EMBEDDED_ASSETS_DEPS ?= "assets"
EMBEDDED_ASSETS ?= ""
EXTRA_LDFLAGS ?= ""


ifeq ("$(FPM_ARCH)", "linux/arm64")
	ifeq ("$(FPM_FORMAT)", "deb")
		LINUX_ARCH = "arm64"
	else
		LINUX_ARCH = "aarch64"
	endif
endif
ifeq ("$(FPM_ARCH)", "linux/amd64")
	ifeq ("$(FPM_FORMAT)", "deb")
		LINUX_ARCH = "amd64"
	else
		LINUX_ARCH = "x86_64"
	endif
endif

ifeq ("$(FPM_FORMAT)", "deb")
	PACKAGE_DEPENDENCIES = ""
endif
ifeq ("$(FPM_FORMAT)", "rpm")
	PACKAGE_DEPENDENCIES = "--rpm-os linux"
endif


# packaging
DOCKER_ARCHES ?= "linux/amd64,linux/arm64"
PACKAGE_TYPES ?= "deb rpm"
VERSION ?= $(shell git tag --points-at HEAD | grep '^v' | sort | tail -n 1 | tr -d 'v')
DOCKER_TAG ?= $(VERSION)
ifeq ("$(DOCKER_TAG)", "")
	DOCKER_TAG = "dev"
endif
ifeq ("$(VERSION)", "")
	VERSION = "0.0.1"
endif

ITERATION ?= "0"
VENDOR = "Pyroscope"
URL = "https://pyroscope.io"
LICENSE = "Apache 2"
MAINTAINER = "contact@pyroscope.io"
DESCRIPTION = "pyroscope is open source continuous profiling software"

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH)
	export GOPATH
endif

.PHONY: all
all: build

.PHONY: build
build:
	$(GOBUILD) -tags $(ENABLED_SPIES) -ldflags "$(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $EMBEDDED_ASSETS)" -o ./bin/pyroscope ./cmd/pyroscope

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

.PHONY: print-build-vars
print-build-vars:
	@echo ""
	@echo "VERSION:    $(VERSION)"
	@echo "DOCKER_TAG: $(DOCKER_TAG)"
	@echo "ITERATION:  $(ITERATION)"
	@echo "FPM_ARCH:   $(FPM_ARCH)"
	@echo "LINUX_ARCH: $(LINUX_ARCH)"
	@echo "FPM_FORMAT: $(FPM_FORMAT)"
	@echo ""

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
		--tag pyroscope/pyroscope:latest \
		--push \
		--platform $(DOCKER_ARCHES) \
		.

install-fpm:
	which fpm || gem install fpm

.PHONY: build-package
build-package:
	$(eval OUTPUT := "tmp/pyroscope-$(VERSION)-$(shell echo $(FPM_ARCH) | tr '/' '-').$(FPM_FORMAT)")
	fpm -f -s dir --log error \
		--vendor $(VENDOR) \
		--url $(URL) \
		--config-files /etc/pyroscope/server.yml \
		--after-install scripts/packages/post-install.sh \
		--after-remove scripts/packages/post-uninstall.sh \
		--license $(LICENSE) \
		--maintainer $(MAINTAINER) \
		--description $(DESCRIPTION) \
		--directories /var/log/pyroscope \
		--directories /var/lib/pyroscope \
		$(shell echo $(PACKAGE_DEPENDENCIES))\
		--name pyroscope \
		-a $(LINUX_ARCH) \
		-t $(FPM_FORMAT) \
		--version $(VERSION) \
		--iteration $(ITERATION) \
		-C $(DIR) \
		-p $(OUTPUT)
	# gh release upload v$(VERSION) --clobber $(OUTPUT)


ifeq ("$(FPM_FORMAT)", "rpm")
	scripts/packages/sign-rpm $(OUTPUT) || true
endif


.PHONY: build-arch
build-arch: print-build-vars
	$(eval DIR := "tmp/$(shell echo $(FPM_ARCH) | tr '/' '-')")

	rm -rf $(DIR)
	mkdir -p $(DIR)/usr/bin
	mkdir -p $(DIR)/etc/pyroscope
	mkdir -p $(DIR)/var/log/pyroscope
	mkdir -p $(DIR)/var/lib/pyroscope
	mkdir -p $(DIR)/usr/lib/pyroscope/scripts
	chmod -R 755 $(DIR)

	@echo "FPM_ARCH: $(FPM_ARCH) $(DIR)"

	docker pull \
		--platform $(FPM_ARCH) \
		pyroscope/pyroscope:$(shell echo $(DOCKER_TAG))
	# docker run \
	# 	--platform $(FPM_ARCH) \
	# 	--rm \
	# 	--entrypoint /bin/sh \
	# 	pyroscope/pyroscope:$(shell echo $(DOCKER_TAG)) \
	# 	-c "/usr/bin/pyroscope"
	# docker run \
	# 	--platform $(FPM_ARCH) \
	# 	--rm \
	# 	--entrypoint /bin/sh \
	# 	pyroscope/pyroscope:$(shell echo $(DOCKER_TAG)) \
	# 	-c "uname -a"
	docker run \
		--platform $(FPM_ARCH) \
		--rm \
		--entrypoint /bin/sh \
		pyroscope/pyroscope:$(shell echo $(DOCKER_TAG)) \
		-c "cat /usr/bin/pyroscope" \
		> $(DIR)/usr/bin/pyroscope

	$(eval OUTPUT := "tmp/pyroscope-$(VERSION)-$(shell echo $(FPM_ARCH) | tr '/' '-')-source.tar.gz")
	tar czf $(OUTPUT) $(DIR)/usr/bin/*
	chmod 755 $(DIR)/usr/bin/pyroscope

	# gh release upload v$(VERSION) --clobber $(OUTPUT)

	cp scripts/packages/server.yml $(DIR)/etc/pyroscope/server.yml
	cp scripts/packages/init.sh $(DIR)/usr/lib/pyroscope/scripts/init.sh
	cp scripts/packages/pyroscope-server.service $(DIR)/usr/lib/pyroscope/scripts/pyroscope-server.service
	chmod 644 $(DIR)/etc/pyroscope/server.yml
	chmod 644 $(DIR)/usr/lib/pyroscope/scripts/init.sh
	chmod 644 $(DIR)/usr/lib/pyroscope/scripts/pyroscope-server.service

	for PACKAGE_FORMAT in $(shell echo $(PACKAGE_TYPES)); do \
		DIR=$(DIR) FPM_FORMAT=$$PACKAGE_FORMAT make build-package ; \
	done

.PHONY: build-all-arches
build-all-arches: install-fpm
	for ARCH in $(shell echo $(DOCKER_ARCHES) | tr ',' ' '); do \
		FPM_ARCH=$$ARCH make build-arch ; \
	done

.PHONY: github-make-release
github-make-release:
	gh release create v$(VERSION) --title '' --notes ''

.PHONY: print-versions
print-versions:
	@echo "current versions:"
	@echo $(shell git tag | grep '^v' | sort | tr -d 'v')
	@echo ""

update-brew-package:
	brew bump-formula-pr --url https://github.com/pyroscope-io/pyroscope/archive/0.0.3.tar.gz pyroscope-io/brew/pyroscope

.PHONY: new-version-release
new-version-release: print-versions
	# ifeq ($(VERSION), "")
	$(eval VERSION := $(shell read -p 'enter new version (without v):' ver; echo $$ver))
	# endif
	@echo "Buidling version $(VERSION)"

	VERSION=$(VERSION) make github-make-release || true
	VERSION=$(VERSION) make docker-build-all-arches
	VERSION=$(VERSION) make build-all-arches


