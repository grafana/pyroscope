#!/usr/bin/env bash

set -euo pipefail

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

# update ci/cd versions
git ls-files .github/workflows | xargs sed -i 's/go-version:\([ \["]\+\)\([0-9\.\=<>]\+\)/go-version:\1'$1'/g' 

# update goreleaser check
sed -i 's/go version go[0-9\.]\+/go version go'$1'/g' .goreleaser.yaml

# update all dockerfile versions, skips the elf tests from ebpf
DOCKER_FILES=$(git ls-files '**/Dockerfile*' | grep -v ebpf/symtab/elf/testdata/Dockerfile)
sed -i 's/golang:[0-9\.]\+/golang:'$1'/g' $DOCKER_FILES
git add -u $DOCKER_FILES

# add changes
git add -u .github/workflows .goreleaser.yaml
git commit -m "Update golang version to $1"
