package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	ebpfspy "github.com/grafana/phlare/ebpf"
	"github.com/grafana/phlare/ebpf/pprof"
	"github.com/grafana/phlare/ebpf/sd"
	"github.com/grafana/phlare/ebpf/symtab"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
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
	options := convertSessionOptions()
	session, err := ebpfspy.NewSession(
		l,
		targetFinder,
		options,
	)
	_ = session
	err = session.Start()
	if err != nil {
		panic(err)
	}
	for {
		time.Sleep(5 * time.Second)

		builders := pprof.NewProfileBuilders(options.SampleRate)
		err := session.CollectProfiles(func(target *sd.Target, stack []string, value uint64, pid uint32) {
			labelsHash, labels := target.Labels()
			builder := builders.BuilderForTarget(labelsHash, labels)
			builder.AddSample(stack, value)
			fmt.Printf("%s %d\n", strings.Join(stack, ";"), value)
		})

		if err != nil {
			panic(err)
		}
		level.Debug(l).Log("msg", "ebpf collectProfiles done", "profiles", len(builders.Builders))

		for _, builder := range builders.Builders {
			serviceName := builder.Labels.Get("service_name")

			buf := bytes.NewBuffer(nil)
			_, err := builder.Write(buf)
			if err != nil {
				panic(err)
			}
			rawProfile := buf.Bytes()
			go ingest(rawProfile, serviceName, builder.Labels)
		}

		if err != nil {
			panic(err)
		}
	}
}

func ingest(profile []byte, name string, labels labels.Labels) {
	//todo labels
	//todo sample type config
	url := "http://localhost:4100/ingest?name=" + name + "&format=pprof"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(profile))
	if err != nil {
		panic(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println(err)
	} else {
		fmt.Printf("ingested %s %s\n", name, res.Status)
	}
}

func convertSessionOptions() ebpfspy.SessionOptions {
	ms := symtab.NewMetrics(prometheus.DefaultRegisterer)
	return ebpfspy.SessionOptions{
		CollectUser:   true,
		CollectKernel: true,
		SampleRate:    11,
		PythonPIDs:    []int{222633},
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
