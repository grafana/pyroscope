package atexit

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var callbacks []func()

var once sync.Once

func initSignalHandler() {
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
	once.Do(initSignalHandler)
	callbacks = append(callbacks, cb)
}
