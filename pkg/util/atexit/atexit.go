package atexit

import (
	"os"
	"os/signal"
	"syscall"
)

var callbacks []func()

func init() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		for _, cb := range callbacks {
			cb()
		}
	}()
}

func Register(cb func()) {
	callbacks = append(callbacks, cb)
}
