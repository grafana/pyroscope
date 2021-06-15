package atexit

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var callbacks []func()

var once sync.Once
var wg sync.WaitGroup

func initSignalHandler() {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-signalChan
		for _, cb := range callbacks {
			cb()
			wg.Done()
		}
	}()
}

func Register(cb func()) {
	once.Do(initSignalHandler)
	wg.Add(1)
	callbacks = append(callbacks, cb)
}

func Wait() {
	wg.Wait()
}
