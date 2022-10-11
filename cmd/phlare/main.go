package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"

	"github.com/grafana/phlare/pkg/cfg"
	"github.com/grafana/phlare/pkg/phlare"
	_ "github.com/grafana/phlare/pkg/util/build"
)

func main() {
	var config phlare.Config
	if err := cfg.DynamicUnmarshal(&config, os.Args[1:], flag.CommandLine); err != nil {
		fmt.Fprintf(os.Stderr, "failed parsing config: %v\n", err)
		os.Exit(1)
	}

	f, err := phlare.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed creating phlare: %v\n", err)
		os.Exit(1)
	}

	err = f.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed running phlare: %v\n", err)
		os.Exit(1)
	}
}
