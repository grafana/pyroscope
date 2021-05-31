package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/pyroscope-io/pyroscope/pkg/cli"
	"github.com/pyroscope-io/pyroscope/pkg/config"
)

func main() {
	cfg := &config.Config{}
	err := cli.Start(cfg)
	if err != nil {
		os.Stderr.Write([]byte(color.RedString("Error: ") + err.Error() + "\n\n"))
	}
}
