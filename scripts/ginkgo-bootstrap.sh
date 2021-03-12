#!/bin/sh
go list ./... | sed 's/github.com\/pyroscope-io\/pyroscope/./' | xargs -I {} bash -c 'cd {} && ginkgo bootstrap'
