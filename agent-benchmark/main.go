package main

import (
	"fmt"
	"github.com/jaegertracing/jaeger/examples/hotrod/cmd"
	"github.com/pyroscope-io/pyroscope/pkg/agent/profiler"
	_ "net/http/pprof"
	"os"
	"time"

	"github.com/dhoomakethu/stress/utils"
)

func main() {
	fmt.Println("starting")

	pyroAddress := os.Getenv("PYROSCOPE_ADDRESS")

	fmt.Println("pyro address is", pyroAddress)
	if pyroAddress != "" {
		fmt.Println("starting profiler")
		_, err := profiler.Start(profiler.Config{
			ApplicationName: "hotrod.golang.app",

			// replace this with the address of pyroscope server
			ServerAddress: pyroAddress,

			// by default all profilers are enabled,
			// but you can select the ones you want to use:
			//		ProfileTypes: []profiler.ProfileType{
			//			profiler.ProfileCPU,
			//			profiler.ProfileAllocObjects,
			//			profiler.ProfileAllocSpace,
			//			profiler.ProfileInuseObjects,
			//			profiler.ProfileInuseSpace,
			//		},
		})
		if err != nil {
			fmt.Println("error in profiler")
			panic(err)
		}
	}

	go func() {
		var cpuload float64
		var duration float64
		var cpu int

		cpuload = 0.75
		sampleInterval := 100 * time.Millisecond
		duration = 3600 // in seconds

		controller := utils.NewCpuLoadController(sampleInterval, cpuload)
		monitor := utils.NewCpuLoadMonitor(float64(cpu), sampleInterval)

		actuator := utils.NewCpuLoadGenerator(controller, monitor, time.Duration(duration))
		utils.StartCpuLoadController(controller)
		utils.StartCpuMonitor(monitor)

		utils.RunCpuLoader(actuator)
		utils.StopCpuLoadController(controller)
		utils.StopCpuMonitor(monitor)
	}()

	//_ = cmd.Execute

	// TODO remove hotrod
	// right now just running to reuse the metrics endpoint
	cmd.Execute()
}
