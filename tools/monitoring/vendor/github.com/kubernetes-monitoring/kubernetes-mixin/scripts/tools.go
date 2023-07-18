//go:build tools
// +build tools

// Package tools tracks dependencies for tools that used in the build process.
// See https://github.com/golang/go/issues/25922
package tools

import (
	_ "github.com/google/go-jsonnet/cmd/jsonnet"
	_ "github.com/google/go-jsonnet/cmd/jsonnet-lint"
	_ "github.com/google/go-jsonnet/cmd/jsonnetfmt"
	_ "github.com/grafana/dashboard-linter"
	_ "github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb"
	_ "github.com/prometheus/prometheus/cmd/promtool"
)
