#!/usr/bin/env bash

set -euo pipefail

# If no arguments provided, determine the version from go.mod and fetch the latest patch version
if [ $# -eq 0 ]; then
    # Use the get-latest-go-version.sh script to get the latest patch version
    SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
    VERSION=$("$SCRIPT_DIR/get-latest-go-version.sh")

    if [ -z "$VERSION" ]; then
        echo "Error: Could not determine latest patch version"
        exit 1
    fi
else
    # Use the provided version
    if [ $# -ne 1 ]; then
        echo "Usage: $0 [version]"
        echo "If version is not provided, the latest patch version matching the current go.mod minor version will be used."
        exit 1
    fi
    VERSION="$1"
fi

echo "Updating Go version to $VERSION"

# update ci/cd versions
git ls-files .github/workflows | xargs sed -i 's/go-version:\([ \["]\+\)\([0-9\.\=<>]\+\)/go-version:\1'$VERSION'/g' 

# update goreleaser check
sed -i 's/go version go[0-9\.]\+/go version go'$VERSION'/g' .goreleaser.yaml

# update all dockerfile versions, skips the elf tests from ebpf
DOCKER_FILES=$(git ls-files '**/Dockerfile*' | grep -v ebpf/symtab/elf/testdata/Dockerfile)
sed -i 's/golang:[0-9\.]\+/golang:'$VERSION'/g' $DOCKER_FILES
git add -u $DOCKER_FILES

# add changes
git add -u .github/workflows .goreleaser.yaml
git commit -m "Update golang version to $VERSION"
