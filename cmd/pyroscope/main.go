package main

import (
	"github.com/fatih/color"

	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func main() {
	if err := cli.Start(new(config.Config)); err != nil {
		fatalf("%s %v\n\n", color.RedString("Error:"), err)
	}
}
