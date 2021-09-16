package main

import (
	"fmt"
	"os"

	"github.com/jaegertracing/jaeger/examples/hotrod/cmd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
)

func main() {
	fmt.Println("starting")

	pyroAddress := os.Getenv("PYROSCOPE_ADDRESS")

	fmt.Println("pyro address is", pyroAddress)
	if pyroAddress != "" {
		profiler.Start(profiler.Config{
			ApplicationName: "hotrod.golang.app",

			// replace this with the address of pyroscope server
			ServerAddress: pyroAddress,

			// by default all profilers are enabled,
			// but you can select the ones you want to use:
			ProfileTypes: []profiler.ProfileType{
				profiler.ProfileCPU,
				profiler.ProfileAllocObjects,
				profiler.ProfileAllocSpace,
				profiler.ProfileInuseObjects,
				profiler.ProfileInuseSpace,
			},
		})
	}
	cmd.Execute()
}
