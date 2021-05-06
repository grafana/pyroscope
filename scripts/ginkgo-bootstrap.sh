#!/bin/sh
go list ./... | sed 's/github.com\/pyroscope-io\/pyroscope/./' | \
  grep -v 'pkg/agent/.\+spy' | \
  grep -v 'pkg/testing' | \
  grep -v 'pkg/agent/cli' | \
  grep -v 'pkg/dbmanager' | \
  grep -v 'scripts' | \
  grep -v 'tools' | \
  grep -v 'examples' | \
  xargs -I {} bash -c 'cd {} && ginkgo bootstrap'
