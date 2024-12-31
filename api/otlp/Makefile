# Large chuncks of this code are copied from https://github.com/open-telemetry/opentelemetry-proto
# See LICENSE file for licensing information.


# Function to execute a command. Note the empty line before endef to make sure each command
# gets executed separately instead of concatenated with previous one.
# Accepts command to execute as first parameter.
define exec-command
$(1)

endef

# Definitions for ProtoBuf generation.

# The source directory for OTLP ProtoBufs.
OPENTELEMETRY_PROTO_SRC_DIR=.tmp/opentelemetry-proto

# The SHA matching the current version of the proto to use
OPENTELEMETRY_PROTO_VERSION=v1.5.0

# Find all .proto files.

OPENTELEMETRY_PROTO_FILES := $(subst $(OPENTELEMETRY_PROTO_SRC_DIR)/,,$(wildcard $(OPENTELEMETRY_PROTO_SRC_DIR)/opentelemetry/proto/*/v1/*.proto $(OPENTELEMETRY_PROTO_SRC_DIR)/opentelemetry/proto/collector/*/v1/*.proto $(OPENTELEMETRY_PROTO_SRC_DIR)/opentelemetry/proto/*/v1development/*.proto $(OPENTELEMETRY_PROTO_SRC_DIR)/opentelemetry/proto/collector/*/v1development/*.proto))

# Target directory to write generated files to.
# PROTO_TARGET_GEN_DIR=otlp
PROTO_SOURCE_GEN_DIR=api/otlp
PROTO_TARGET_GEN_DIR=api/otlp

# Go package name to use for generated files.
# PROTO_PACKAGE=github.com/grafana/pyroscope/pkg/$(PROTO_TARGET_GEN_DIR)
PROTO_SOURCE_PACKAGE=github.com/grafana/pyroscope/$(PROTO_SOURCE_GEN_DIR)
PROTO_PACKAGE=github.com/grafana/pyroscope/$(PROTO_TARGET_GEN_DIR)

# Intermediate directory used during generation.
PROTO_INTERMEDIATE_DIR=.tmp/patched-otlp-proto

DOCKER_PROTOBUF ?= otel/build-protobuf:0.23.0
PROTOC := docker run --rm -u ${shell id -u} -v${PWD}:${PWD} -w${PWD}/$(PROTO_INTERMEDIATE_DIR) ${DOCKER_PROTOBUF} --proto_path=${PWD}
PROTO_INCLUDES := -I/usr/include/github.com/gogo/protobuf -I./

# Cleanup temporary directory
genproto-cleanup:
	rm -Rf ${OPENTELEMETRY_PROTO_SRC_DIR}

# Generate OTLP Protobuf Go files. This will place generated files in PROTO_TARGET_GEN_DIR.
genproto: genproto-cleanup
# TODO(@petethepig): undo this
# mkdir -p ${OPENTELEMETRY_PROTO_SRC_DIR}
# curl -sSL https://api.github.com/repos/open-telemetry/opentelemetry-proto/tarball/${OPENTELEMETRY_PROTO_VERSION} | tar xz --strip 1 -C ${OPENTELEMETRY_PROTO_SRC_DIR}
# # Call a sub-make to ensure OPENTELEMETRY_PROTO_FILES is populated
	rsync -av ../../otel/opentelemetry-proto/ ${OPENTELEMETRY_PROTO_SRC_DIR}
	$(MAKE) genproto_sub
	$(MAKE) genproto-cleanup

genproto_sub:
	@echo Generating code for the following files:
	@$(foreach file,$(OPENTELEMETRY_PROTO_FILES),$(call exec-command,echo $(file)))

	@echo Delete intermediate directory.
	@rm -rf $(PROTO_INTERMEDIATE_DIR)

	@echo Copy .proto file to intermediate directory.
	mkdir -p $(PROTO_INTERMEDIATE_DIR)/opentelemetry
	cp -R $(OPENTELEMETRY_PROTO_SRC_DIR)/opentelemetry/* $(PROTO_INTERMEDIATE_DIR)/opentelemetry

	# # Patch proto files. See proto_patch.sed for patching rules.
	# @echo Modify them in the intermediate directory.
	$(foreach file,$(OPENTELEMETRY_PROTO_FILES),$(call exec-command,sed -f api/otlp/proto_patch.sed $(OPENTELEMETRY_PROTO_SRC_DIR)/$(file) > $(PROTO_INTERMEDIATE_DIR)/$(file)))

	# HACK: Workaround for istio 1.15 / envoy 1.23.1 mistakenly emitting deprecated field.
	# reserved 1000 -> repeated ScopeLogs deprecated_scope_logs = 1000;
	sed 's/reserved 1000;/repeated ScopeLogs deprecated_scope_logs = 1000;/g' $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/logs/v1/logs.proto 1<> $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/logs/v1/logs.proto
	# reserved 1000 -> repeated ScopeProfiles deprecated_scope_profiles = 1000;
	sed 's/reserved 1000;/repeated ScopeProfiles deprecated_scope_profiles = 1000;/g' $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/profiles/v1development/profiles.proto 1<> $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/profiles/v1development/profiles.proto
	# reserved 1000 -> repeated ScopeMetrics deprecated_scope_metrics = 1000;
	sed 's/reserved 1000;/repeated ScopeMetrics deprecated_scope_metrics = 1000;/g' $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/metrics/v1/metrics.proto 1<> $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/metrics/v1/metrics.proto
	# reserved 1000 -> repeated ScopeSpans deprecated_scope_spans = 1000;
	sed 's/reserved 1000;/repeated ScopeSpans deprecated_scope_spans = 1000;/g' $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/trace/v1/trace.proto 1<> $(PROTO_INTERMEDIATE_DIR)/opentelemetry/proto/trace/v1/trace.proto


	@echo Generate Go code from .proto files in intermediate directory.
	$(foreach file,$(OPENTELEMETRY_PROTO_FILES),$(call exec-command,$(PROTOC) $(PROTO_INCLUDES) --gogofaster_out=plugins=grpc:./ $(file)))

	@echo Move generated code to target directory.
	mkdir -p $(PROTO_TARGET_GEN_DIR)
	cp -R $(PROTO_INTERMEDIATE_DIR)/$(PROTO_SOURCE_PACKAGE)/* $(PROTO_TARGET_GEN_DIR)/


	rm -rf $(PROTO_INTERMEDIATE_DIR)/go.opentelemetry.io

	rm -rf api/otlp/logs
	rm -rf api/otlp/metrics
	rm -rf api/otlp/trace

	rm -rf api/otlp/collector/logs
	rm -rf api/otlp/collector/metrics
	rm -rf api/otlp/collector/trace

	@rm -rf $(OPENTELEMETRY_PROTO_SRC_DIR)/*
	@rm -rf $(OPENTELEMETRY_PROTO_SRC_DIR)/.* > /dev/null 2>&1 || true
