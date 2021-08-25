// +build tools

// Package tools is used to describe various tools we use.
// Think of it as "dev-dependencies" in ruby or node projects
// See: https://marcofranssen.nl/manage-go-tools-via-go-modules/
// See Makefile for an example of how it's used
package tools

import (
	_ "github.com/cosmtrek/air"
	_ "github.com/davecgh/go-spew/spew"
	_ "github.com/google/go-jsonnet/cmd/jsonnet"
	_ "github.com/google/pprof"
	_ "github.com/jsonnet-bundler/jsonnet-bundler/cmd/jb"
	_ "github.com/kisielk/godepgraph"
	_ "github.com/kyoh86/richgo"
	_ "github.com/mattn/goreman"
	_ "github.com/mgechev/revive"
	_ "github.com/onsi/ginkgo/ginkgo"
	_ "golang.org/x/tools/cmd/godoc"
	_ "honnef.co/go/tools/cmd/staticcheck"
)
