#!/usr/bin/env bash

# Script to compile Pyroscope jsonnet mixin to JSON/YAML files
# This script compiles dashboards, recording rules, and alert rules
# from the pyroscope-mixin into ready-to-use files.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIXIN_DIR="${PROJECT_ROOT}/operations/pyroscope/jsonnet/pyroscope-mixin/pyroscope-mixin"
OUTPUT_DIR="${PROJECT_ROOT}/operations/pyroscope/mixin-compiled"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if required tools are available
check_dependencies() {
    local missing_deps=()

    if ! command -v jb &> /dev/null; then
        missing_deps+=("jb (jsonnet-bundler)")
    fi

    if ! command -v jsonnet &> /dev/null; then
        missing_deps+=("jsonnet")
    fi

    if [ ${#missing_deps[@]} -ne 0 ]; then
        log_error "Missing required dependencies: ${missing_deps[*]}"
        log_info "Please install them or run: make compile-mixin (which will install them automatically)"
        exit 1
    fi
}

# Install jsonnet dependencies
install_dependencies() {
    log_info "Installing jsonnet dependencies..."
    cd "${MIXIN_DIR}"

    if [ -f "jsonnetfile.json" ]; then
        jb install
        log_info "Dependencies installed successfully"
    else
        log_warn "No jsonnetfile.json found, skipping dependency installation"
    fi
}

# Create output directories
setup_output_dirs() {
    log_info "Setting up output directories..."
    mkdir -p "${OUTPUT_DIR}/dashboards"
    mkdir -p "${OUTPUT_DIR}/rules"
}

# Compile dashboards
compile_dashboards() {
    log_info "Compiling dashboards..."

    cd "${MIXIN_DIR}"

    # Create a temporary jsonnet file to extract dashboards
    cat > "${MIXIN_DIR}/compile-dashboards.jsonnet" <<'EOF'
local mixin = import 'mixin.libsonnet';
{
  [name]: mixin.grafanaDashboards[name]
  for name in std.objectFields(mixin.grafanaDashboards)
}
EOF

    # Compile all dashboards
    jsonnet -J vendor compile-dashboards.jsonnet -m "${OUTPUT_DIR}/dashboards"

    # Clean up
    rm -f "${MIXIN_DIR}/compile-dashboards.jsonnet"

    # Count and report
    local dashboard_count=$(find "${OUTPUT_DIR}/dashboards" -name "*.json" | wc -l | tr -d ' ')
    log_info "Compiled ${dashboard_count} dashboard(s)"
}

# Compile recording rules
compile_recording_rules() {
    log_info "Compiling recording rules..."

    cd "${MIXIN_DIR}"

    # Create a temporary jsonnet file to extract recording rules
    cat > "${MIXIN_DIR}/compile-recording-rules.jsonnet" <<'EOF'
local mixin = import 'mixin.libsonnet';
mixin.prometheusRules
EOF

    # Compile recording rules to YAML (Prometheus format)
    jsonnet -J vendor compile-recording-rules.jsonnet -o "${OUTPUT_DIR}/rules/recording-rules.json"

    # Convert JSON to YAML using Python (more reliable than jsonnet -y)
    python3 -c "import json, yaml, sys; print(yaml.dump(json.load(open('${OUTPUT_DIR}/rules/recording-rules.json')), default_flow_style=False))" > "${OUTPUT_DIR}/rules/recording-rules.yaml" 2>/dev/null || {
        # Fallback: keep JSON if yaml module not available
        log_warn "Python yaml module not found, keeping JSON format"
        mv "${OUTPUT_DIR}/rules/recording-rules.json" "${OUTPUT_DIR}/rules/recording-rules.yaml"
    }

    # Clean up JSON file if YAML was created successfully
    [ -f "${OUTPUT_DIR}/rules/recording-rules.yaml" ] && rm -f "${OUTPUT_DIR}/rules/recording-rules.json"

    # Clean up
    rm -f "${MIXIN_DIR}/compile-recording-rules.jsonnet"

    log_info "Compiled recording rules to recording-rules.yaml"
}

# Compile alert rules (if they exist)
compile_alert_rules() {
    log_info "Checking for alert rules..."

    cd "${MIXIN_DIR}"

    # Create a temporary jsonnet file to check for alerts
    cat > "${MIXIN_DIR}/compile-alert-rules.jsonnet" <<'EOF'
local mixin = import 'mixin.libsonnet';
if std.objectHas(mixin, 'prometheusAlerts') then
  mixin.prometheusAlerts
else
  {}
EOF

    # Check if alerts exist
    local alert_output=$(jsonnet -J vendor compile-alert-rules.jsonnet 2>/dev/null || echo "{}")

    if [ "${alert_output}" != "{}" ] && [ "${alert_output}" != "{ }" ]; then
        jsonnet -J vendor compile-alert-rules.jsonnet -o "${OUTPUT_DIR}/rules/alert-rules.json"

        # Convert JSON to YAML using Python
        python3 -c "import json, yaml, sys; print(yaml.dump(json.load(open('${OUTPUT_DIR}/rules/alert-rules.json')), default_flow_style=False))" > "${OUTPUT_DIR}/rules/alert-rules.yaml" 2>/dev/null || {
            log_warn "Python yaml module not found, keeping JSON format"
            mv "${OUTPUT_DIR}/rules/alert-rules.json" "${OUTPUT_DIR}/rules/alert-rules.yaml"
        }

        # Clean up JSON file
        [ -f "${OUTPUT_DIR}/rules/alert-rules.yaml" ] && rm -f "${OUTPUT_DIR}/rules/alert-rules.json"

        log_info "Compiled alert rules to alert-rules.yaml"
    else
        log_warn "No alert rules found in mixin"
        # Create an empty placeholder file with documentation
        cat > "${OUTPUT_DIR}/rules/alert-rules.yaml" <<'YAML_EOF'
# Pyroscope Alert Rules
#
# Currently, no alert rules are defined in the pyroscope-mixin.
# This file is a placeholder for future alert rules.
#
# To add your own alert rules, you can create them here following
# the Prometheus alerting rules format:
# https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/
groups: []
YAML_EOF
        log_info "Created placeholder alert-rules.yaml"
    fi

    # Clean up
    rm -f "${MIXIN_DIR}/compile-alert-rules.jsonnet"
}

# Generate metadata file
generate_metadata() {
    log_info "Generating metadata..."

    local git_commit=$(git -C "${PROJECT_ROOT}" rev-parse --short HEAD 2>/dev/null || echo "unknown")
    local git_branch=$(git -C "${PROJECT_ROOT}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
    local compile_date=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

    cat > "${OUTPUT_DIR}/metadata.json" <<EOF
{
  "compiled_at": "${compile_date}",
  "git_commit": "${git_commit}",
  "git_branch": "${git_branch}",
  "source": "operations/pyroscope/jsonnet/pyroscope-mixin/pyroscope-mixin",
  "compiler_version": {
    "jsonnet": "$(jsonnet --version 2>&1 | head -n 1 || echo 'unknown')",
    "jsonnet-bundler": "$(jb --version 2>&1 || echo 'unknown')"
  }
}
EOF

    log_info "Generated metadata.json"
}

# Main execution
main() {
    log_info "Starting Pyroscope mixin compilation..."
    log_info "Mixin source: ${MIXIN_DIR}"
    log_info "Output directory: ${OUTPUT_DIR}"

    # Check if running from Makefile (dependencies already installed)
    if [ "${FROM_MAKEFILE:-}" != "true" ]; then
        check_dependencies
    fi

    install_dependencies
    setup_output_dirs
    compile_dashboards
    compile_recording_rules
    compile_alert_rules
    generate_metadata

    log_info ""
    log_info "✓ Compilation complete!"
    log_info "Output location: ${OUTPUT_DIR}"
    log_info ""
    log_info "Files generated:"
    log_info "  - Dashboards: ${OUTPUT_DIR}/dashboards/"
    log_info "  - Recording Rules: ${OUTPUT_DIR}/rules/recording-rules.yaml"
    log_info "  - Alert Rules: ${OUTPUT_DIR}/rules/alert-rules.yaml"
    log_info "  - Metadata: ${OUTPUT_DIR}/metadata.json"
}

main "$@"
