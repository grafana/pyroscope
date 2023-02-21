package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
	"runtime/pprof"
	"sync"

	"github.com/hashicorp/consul/api"
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

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte("ok"))
}

func main() {
	const (
		consulAddress = "consul:8500"
		serviceName   = "test-service"
		servicePort   = 8000
	)

	go func() {
		http.HandleFunc("/health", healthHandler)
		log.Println(http.ListenAndServe(fmt.Sprintf(":%d", servicePort), nil))
	}()

	// Register the service in consul with the local agent.
	config := api.DefaultConfig()
	config.Address = consulAddress
	client, err := api.NewClient(config)
	if err != nil {
		log.Fatalf("unable to initialize consul client: %v", err)
	}
	hostname, _ := os.Hostname()
	registration := &api.AgentServiceRegistration{
		Name:    serviceName,
		Port:    servicePort,
		Address: hostname,
		Meta: map[string]string{
			"env": "dev",
		},
		Check: &api.AgentServiceCheck{
			HTTP:     fmt.Sprintf("http://%s:%d/health", hostname, servicePort),
			Interval: "10s",
			Timeout:  "30s",
		},
	}

	if err = client.Agent().ServiceRegister(registration); err != nil {
		log.Fatalf("failed to register consul client: %v", err)
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
