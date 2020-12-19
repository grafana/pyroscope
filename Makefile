GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
GOBUILD=go build -trimpath
GODEBUG=asyncpreemptoff=1

ENABLED_SPIES ?= "rbspy,pyspy"
EMBEDDED_ASSETS ?= ""
EXTRA_LDFLAGS ?= ""

ifndef $(GOPATH)
	GOPATH=$(shell go env GOPATH)
	export GOPATH
endif

#
#     ____                 __                                 __
#    / __ \___ _   _____  / /___  ____  ____ ___  ___  ____  / /_
#   / / / / _ \ | / / _ \/ / __ \/ __ \/ __ `__ \/ _ \/ __ \/ __/
#  / /_/ /  __/ |/ /  __/ / /_/ / /_/ / / / / / /  __/ / / / /_
# /_____/\___/|___/\___/_/\____/ .___/_/ /_/ /_/\___/_/ /_/\__/
#                             /_/
#


.PHONY: all
all: build

.PHONY: build
build:
	$(GOBUILD) -tags $(ENABLED_SPIES) -ldflags "$(EXTRA_LDFLAGS) $(shell scripts/generate-build-flags.sh $(EMBEDDED_ASSETS))" -o ./bin/pyroscope ./cmd/pyroscope

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


#
#     ____             __
#    / __ \____ ______/ /______ _____ ____  _____
#   / /_/ / __ `/ ___/ //_/ __ `/ __ `/ _ \/ ___/
#  / ____/ /_/ / /__/ ,< / /_/ / /_/ /  __(__  )
# /_/    \__,_/\___/_/|_|\__,_/\__, /\___/____/
#                             /____/
#
# TODO: I think long term a good chunk of this logic should be in a ruby script instead, too many subroutines

EMBEDDED_ASSETS_DEPS ?= "assets"
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

ITERATION ?= "1"
VENDOR = "Pyroscope"
URL = "https://pyroscope.io"
LICENSE = "Apache 2"
MAINTAINER = "contact@pyroscope.io"
DESCRIPTION = "pyroscope is open source continuous profiling software"


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
embedded-assets: install-dev-tools $(shell echo $(EMBEDDED_ASSETS_DEPS))
	$(GOPATH)/bin/pkger -o pkg/server

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

.PHONY: upload-asset
upload-asset:
	$(eval SHA256 := $(shell sha256sum $(OUTPUT) | cut -d " " -f 1))
	@echo $(SHA256)
	gh release upload v$(VERSION) --clobber $(OUTPUT)
	aws s3 cp --metadata sha256=$(SHA256) --acl public-read $(OUTPUT) s3://dl.pyroscope.io/release/$(shell basename $(OUTPUT))

.PHONY: build-package
build-package:
ifeq ("$(FPM_FORMAT)", "rpm")
	$(eval OUTPUT := "tmp/pyroscope-$(VERSION)-$(ITERATION)-$(LINUX_ARCH).$(FPM_FORMAT)")
else
	$(eval OUTPUT := "tmp/pyroscope_$(VERSION)_$(LINUX_ARCH).$(FPM_FORMAT)")
endif
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
	VERSION=$(VERSION) OUTPUT=$(OUTPUT) make upload-asset

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

	$(eval OUTPUT := "tmp/pyroscope-$(VERSION)-$(shell echo $(FPM_ARCH) | tr '/' '-').tar.gz")
	tar czf $(OUTPUT) $(DIR)/usr/bin/*
	chmod 755 $(DIR)/usr/bin/pyroscope

	VERSION=$(VERSION) OUTPUT=$(OUTPUT) make upload-asset

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

.PHONY: upload-source
upload-source:
	$(eval OUTPUT := "tmp/pyroscope-$(VERSION)-source.tar.gz")
	git archive --format tar.gz --output $(OUTPUT) HEAD
	VERSION=$(VERSION) OUTPUT=$(OUTPUT) make upload-asset

.PHONY: print-versions
print-versions:
	@echo "current versions:"
	@echo $(shell git tag | grep '^v' | sort | tr -d 'v')
	@echo ""

# Run this one when releasing a new version:
.PHONY: new-version-release
new-version-release: print-versions
	# ifeq ($(VERSION), "")
	$(eval VERSION := $(shell read -p 'enter new version (without v):' ver; echo $$ver))
	# endif
	@echo "Buidling version $(VERSION)"

	VERSION=$(VERSION) make github-make-release || true
	VERSION=$(VERSION) make upload-source
	# VERSION=$(VERSION) make docker-build-all-arches
	VERSION=$(VERSION) make build-all-arches
	VERSION=$(VERSION) make generate-packages-manifest

.PHONY: git-archive
git-archive:
	git archive --format tar.gz --output tmp/pyroscope.tar.gz HEAD
	cp tmp/pyroscope.tar.gz /Users/dmitry/Library/Caches/Homebrew/downloads/abc93b5985f47d8af95b6ef78548f8f8c748e704a0ea70ac53857ca8db10b7e8--pyroscope-0.0.8.tar.gz
	sha256sum tmp/pyroscope.tar.gz

.PHONY: update-brew-package
update-brew-package:
	brew bump-formula-pr --url https://github.com/pyroscope-io/pyroscope/archive/0.0.3.tar.gz pyroscope-io/brew/pyroscope

.PHONY: generate-packages-manifest
generate-packages-manifest:
	scripts/packages/generate-packages-manifest.rb
	cp scripts/packages/packages.manifest.json ../pyroscope.io/packages.manifest.json

