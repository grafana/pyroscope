package ume

import (
	"fmt"
	"os"
	"regexp"
	"time"
)

func WaitForDebugger() {
	re := regexp.MustCompile("TracerPid:\\s+(\\d+)")
	for {
		fmt.Println("waiting for debugger to attach")
		time.Sleep(time.Second)
		status, err := os.ReadFile("/proc/self/status")
		if err != nil {
			fmt.Println(err)
		}
		submatches := re.FindAllStringSubmatch(string(status), -1)
		if len(submatches) != 1 {
			fmt.Println(" %w")
			continue
		}
		pid := submatches[0][1]
		if pid == "0" {
			continue
		}
		fmt.Printf("debugger attach %s\n", pid)
		break
	}
}
