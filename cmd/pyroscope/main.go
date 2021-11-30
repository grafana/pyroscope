//go:build !js
// +build !js

package main

import (
	"github.com/fatih/color"

	"github.com/pyroscope-io/pyroscope/cmd/pyroscope/command"
)

func main() {
	if err := command.Execute(); err != nil {
		fatalf("%s %v\n\n", color.RedString("Error:"), err)
	}
}
