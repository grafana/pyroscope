package main

import (
	"context"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"

	"github.com/pyroscope-io/client/pyroscope"
)

//go:noinline
func work(n int) {
	// revive:disable:empty-block this is fine because this is a example app, not real production code
	for i := 0; i < n; i++ {
	}
	// revive:enable:empty-block
}

var m *sync.Mutex

func init() {
	m = &sync.Mutex{}
	runtime.SetMutexProfileFraction(5)
	runtime.SetBlockProfileRate(5)
}

func fastFunction(c context.Context, wg *sync.WaitGroup) {
	m.Lock()
	defer m.Unlock()
	pyroscope.TagWrapper(c, pyroscope.Labels("function", "fast"), func(c context.Context) {
		work(20000000)
	})
	wg.Done()
}

func slowFunction(c context.Context, wg *sync.WaitGroup) {
	m.Lock()
	defer m.Unlock()
	// standard pprof.Do wrappers work as well
	pprof.Do(c, pprof.Labels("function", "slow"), func(c context.Context) {
		work(80000000)
	})
	wg.Done()
}

func main() {
	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()
	serverAddress := os.Getenv("PYROSCOPE_SERVER_ADDRESS")
	if serverAddress == "" {
		serverAddress = "http://localhost:4040"
	}
	pyroscope.TagWrapper(context.Background(), pyroscope.Labels("foo", "bar"), func(c context.Context) {
		for {
			wg := sync.WaitGroup{}
			wg.Add(6)
			go fastFunction(c, &wg)
			go fastFunction(c, &wg)
			go fastFunction(c, &wg)
			go slowFunction(c, &wg)
			go slowFunction(c, &wg)
			go slowFunction(c, &wg)
			wg.Wait()
		}
	})
}
