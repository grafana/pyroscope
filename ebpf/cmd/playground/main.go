package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/bufbuild/connect-go"
	"github.com/go-kit/log"

	"github.com/go-kit/log/level"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/gen/proto/go/push/v1/pushv1connect"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	ebpfspy "github.com/grafana/pyroscope/ebpf"
	"github.com/grafana/pyroscope/ebpf/pprof"
	"github.com/grafana/pyroscope/ebpf/sd"
	"github.com/grafana/pyroscope/ebpf/symtab"
	"github.com/prometheus/client_golang/prometheus"
	commonconfig "github.com/prometheus/common/config"
)

const sampleRate = 99 // times per second

// todo remove this cmd or move to a separate go module or out of repository, because it adds a bunch of dependencies
func main() {
	l := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))

	targetFinder, err := sd.NewTargetFinder(os.DirFS("/"), l, sd.TargetsOptions{
		TargetsOnly:        false,
		DefaultTarget:      map[string]string{"service_name": "playground7"},
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

	profiles := make(chan *pushv1.PushRequest, 128)
	go ingest(profiles)
	for {
		time.Sleep(5 * time.Second)

		builders := pprof.NewProfileBuilders(sampleRate)
		err := session.CollectProfiles(func(target *sd.Target, stack []string, value uint64, pid uint32) {
			labelsHash, labels := target.Labels()
			builder := builders.BuilderForTarget(labelsHash, labels)
			builder.AddSample(stack, value)
			//fmt.Printf("%s %d\n", strings.Join(stack, ";"), value)
		})

		if err != nil {
			panic(err)
		}
		level.Debug(l).Log("msg", "ebpf collectProfiles done", "profiles", len(builders.Builders))

		for _, builder := range builders.Builders {

			protoLabels := make([]*typesv1.LabelPair, 0, builder.Labels.Len())
			for _, label := range builder.Labels {
				protoLabels = append(protoLabels, &typesv1.LabelPair{
					Name: label.Name, Value: label.Value,
				})
			}

			buf := bytes.NewBuffer(nil)
			_, err := builder.Write(buf)
			if err != nil {
				panic(err)
			}
			rawProfile := buf.Bytes()
			req := &pushv1.PushRequest{
				Series: []*pushv1.RawProfileSeries{
					{
						Labels: protoLabels,
						Samples: []*pushv1.RawSample{
							{
								RawProfile: rawProfile,
							},
						},
					},
				},
			}
			select {
			case profiles <- req:
			default:
				fmt.Println("dropping profile")
			}

		}

		if err != nil {
			panic(err)
		}
	}
}

func ingest(profiles chan *pushv1.PushRequest) {
	httpClient, err := commonconfig.NewClientFromConfig(commonconfig.DefaultHTTPClientConfig, "http_playground")
	if err != nil {
		panic(err)
	}
	client := pushv1connect.NewPusherServiceClient(httpClient, "http://localhost:4100")

	for {
		it := <-profiles
		res, err := client.Push(context.TODO(), connect.NewRequest(it))
		if err != nil {
			fmt.Println(err)
		}
		if res != nil {
			fmt.Println(res)
		}
	}

}

func convertSessionOptions() ebpfspy.SessionOptions {
	ms := symtab.NewMetrics(prometheus.DefaultRegisterer)
	return ebpfspy.SessionOptions{
		CollectUser:               true,
		CollectKernel:             true,
		SampleRate:                sampleRate,
		UnknownSymbolAddress:      true,
		UnknownSymbolModuleOffset: true,
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
			SymbolOptions: symtab.SymbolOptions{
				GolangSymbolTableFallback: true,
			},
		},
	}
}
