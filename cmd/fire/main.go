package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/grafana/fire/pkg/cfg"
	"github.com/grafana/fire/pkg/fire"
	_ "github.com/grafana/fire/pkg/util/build"
)

func main() {

	// collect block and mutex contention profiles
	// TODO: Configuration variable?
	runtime.SetMutexProfileFraction(10)
	runtime.SetBlockProfileRate(10)

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
