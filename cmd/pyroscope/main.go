package main

import (
	"github.com/fatih/color"

	// revive:disable:blank-imports Depending on configuration these packages may or may not be used.
	//   That's why we do a blank import here and then packages themselves register with the rest of the code.
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/dotnetspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/gospy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/phpspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/pyspy"
	_ "github.com/pyroscope-io/pyroscope/pkg/agent/rbspy"

	// revive:enable:blank-imports

	"github.com/pyroscope-io/pyroscope/cmd/pyroscope/command"
)

func main() {
	if err := command.Execute(); err != nil {
		fatalf("%s %v\n\n", color.RedString("Error:"), err)
	}
}
