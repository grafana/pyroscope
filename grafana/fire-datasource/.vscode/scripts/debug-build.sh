#!/bin/bash

# so mage is in the path
export PATH=$(go env GOPATH)/bin:$PATH

mage -v build:debug
mage -v reloadPlugin