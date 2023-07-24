package ume

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"time"
)

func StartGDBServer() {
	//gdbserver --attach :4343 65639
	dateCmd := exec.Command("date", "--attach", ":4343", strconv.Itoa(os.Getpid()))
	err := dateCmd.Start()
	if err != nil {
		panic(err)
	}
}
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
