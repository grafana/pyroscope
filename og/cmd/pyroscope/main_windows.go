package main

import (
	"fmt"
	"os"

	"github.com/kardianos/service"
	"golang.org/x/sys/windows/svc/eventlog"
)

func fatalf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if service.Interactive() {
		_, _ = fmt.Fprint(os.Stderr, msg)
		os.Exit(1)
	}
	log, err := eventlog.Open("Pyroscope")
	if err == nil {
		err = log.Error(1, msg)
	}
	if err != nil {
		panic(msg)
	}
	os.Exit(1)
}
