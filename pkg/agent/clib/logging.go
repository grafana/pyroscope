package main

import (
	"C"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/agent"
)

//export TestLogger
func TestLogger() int {
	logger.Errorf("logger test error %d", 123)
	logger.Infof("logger test info %d", 123)
	logger.Debugf("logger test debug %d", 123)
	return 0
}

//export SetLoggerLevel
func SetLoggerLevel(l int) int {
	level = l
	return 0
}

var logger agent.Logger
var level int

func init() {
	logger = &clibLogger{}
}

type clibLogger struct{}

func (*clibLogger) Errorf(a string, b ...interface{}) {
	if level < 0 {
		return
	}
	if a[len(a)-1] != '\n' {
		a += "\n"
	}
	fmt.Printf("[ERROR] "+a, b...)
}

func (*clibLogger) Infof(a string, b ...interface{}) {
	if level < 1 {
		return
	}
	if a[len(a)-1] != '\n' {
		a += "\n"
	}
	fmt.Printf("[INFO] "+a, b...)
}

func (*clibLogger) Debugf(a string, b ...interface{}) {
	if level < 2 {
		return
	}
	if a[len(a)-1] != '\n' {
		a += "\n"
	}
	fmt.Printf("[DEBUG] "+a, b...)
}
