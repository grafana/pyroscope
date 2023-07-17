package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	ebpfspy "github.com/grafana/phlare/ebpf"
	"github.com/grafana/phlare/ebpf/sd"
	"github.com/grafana/phlare/ebpf/symtab"
	"github.com/prometheus/client_golang/prometheus"
)

func main() {
	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))

	targetFinder, err := sd.NewTargetFinder(os.DirFS("/"), l, sd.TargetsOptions{
		TargetsOnly:        false,
		DefaultTarget:      map[string]string{"service_name": "playground"},
		ContainerCacheSize: 239,
	})
	if err != nil {
		panic(fmt.Errorf("ebpf target finder create: %w", err))
	}
	session, err := ebpfspy.NewSession(
		l,
		targetFinder,
		convertSessionOptions(),
	)
	_ = session
	err = session.Start()
	if err != nil {
		panic(err)
	}
	for {
		time.Sleep(5 * time.Second)
		err := session.CollectProfiles(func(target *sd.Target, stack []string, value uint64, pid uint32) {
			fmt.Printf("%s %d\n", strings.Join(stack, ";"), value)
		})
		if err != nil {
			panic(err)
		}
	}
}

func convertSessionOptions() ebpfspy.SessionOptions {
	ms := symtab.NewMetrics(prometheus.DefaultRegisterer)
	return ebpfspy.SessionOptions{
		CollectUser:   true,
		CollectKernel: true,
		SampleRate:    11,
		CacheOptions: symtab.CacheOptions{
			PidCacheOptions: symtab.GCacheOptions{
				Size:       239,
				KeepRounds: 3,
			},
			BuildIDCacheOptions: symtab.GCacheOptions{
				Size:       239,
				KeepRounds: 3,
			},
			SameFileCacheOptions: symtab.GCacheOptions{
				Size:       239,
				KeepRounds: 3,
			},
			Metrics: ms,
		},
	}
}
