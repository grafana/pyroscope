package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/grafana/fire/pkg/cfg"
	"github.com/grafana/fire/pkg/fire"
)

func main() {
	var config fire.Config
	if err := cfg.DynamicUnmarshal(&config, os.Args[1:], flag.CommandLine); err != nil {
		fmt.Fprintf(os.Stderr, "failed parsing config: %v\n", err)
		os.Exit(1)
	}

	f, err := fire.New(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed creating fire: %v\n", err)
		os.Exit(1)
	}

	err = f.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed running fire: %v\n", err)
		os.Exit(1)
	}
}
