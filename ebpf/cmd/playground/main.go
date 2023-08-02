package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
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
)

const sampleRate = 11 // times per second
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

		builders := pprof.NewProfileBuilders(time.Second.Nanoseconds() / sampleRate)
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
			go ingest(rawProfile, serviceName)
		}

		if err != nil {
			panic(err)
		}
	}
}

func ingest(profile []byte, name string) {

	buf := bytes.NewBuffer(nil)
	w := multipart.NewWriter(buf)
	stcW, err := w.CreateFormFile("sample_type_config", "sample_type_config")
	if err != nil {
		panic(err)
	}
	_, err = stcW.Write([]byte(`{
  "cpu": {
    "units": "samples",
    "sampled": true
  }
}`))
	if err != nil {
		panic(err)
	}

	profileW, err := w.CreateFormFile("profile", "profile")
	if err != nil {
		panic(err)
	}
	_, err = profileW.Write(profile)
	if err != nil {
		panic(err)
	}
	err = w.Close()
	if err != nil {
		panic(err)
	}

	url := "http://localhost:4100/ingest?name=" + name + ""
	req, err := http.NewRequest("POST", url, buf)
	if err != nil {
		panic(err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

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
		SampleRate:    sampleRate,
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
