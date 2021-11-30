//go:build !js
// +build !js

package main

import (
	"github.com/pyroscope-io/pyroscope/pkg/cli"
)

func init() {
	cli.InitLogging()
}
