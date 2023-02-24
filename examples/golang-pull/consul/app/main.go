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
	"time"

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
	checkURL := fmt.Sprintf("http://%s:%d/health", hostname, servicePort)
	checkTimeout := 30 * time.Second
	registration := &api.AgentServiceRegistration{
		ID:      hostname,
		Name:    serviceName,
		Port:    servicePort,
		Address: hostname,
		Meta: map[string]string{
			"env": "dev",
		},
		Check: &api.AgentServiceCheck{
			HTTP:     checkURL,
			Interval: "10s",
			Timeout:  checkTimeout.String(),
			// The service and all its checks are deregistered after this check
			// is in the critical state for more than the specified value.
			// See https://developer.hashicorp.com/consul/docs/discovery/checks#deregister_critical_service_after
			DeregisterCriticalServiceAfter: (checkTimeout * 3).String(),
		},
	}

	if err = client.Agent().ServiceRegister(registration); err != nil {
		log.Fatalln("failed to register service:", err)
	}
	defer func() {
		if err = client.Agent().ServiceDeregister(registration.ID); err != nil {
			log.Println("failed to deregister service:", err)
		}
	}()

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
