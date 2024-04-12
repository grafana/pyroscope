//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/ebpf/cpp/demangle"
	ebpfmetrics "github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"

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

var configFile = flag.String("config", "", "config file path")
var server = flag.String("server", "http://localhost:4040", "")
var discoverFreq = flag.Duration("discover.freq",
	5*time.Second,
	"")

var collectFreq = flag.Duration("collect.freq",
	15*time.Second,
	"")

var (
	config  *Config
	logger  log.Logger
	metrics *ebpfmetrics.Metrics
	session ebpfspy.Session
)

type splitLog struct {
	err  log.Logger
	rest log.Logger
}

func (s splitLog) Log(keyvals ...interface{}) error {
	if len(keyvals)%2 != 0 {
		return s.err.Log(keyvals...)
	}
	for i := 0; i < len(keyvals); i += 2 {
		if keyvals[i] == "level" {
			vv := keyvals[i+1]
			vvs, ok := vv.(fmt.Stringer)
			if ok && vvs.String() == "error" {
				return s.err.Log(keyvals...)
			}
		}
	}
	return s.rest.Log(keyvals...)
}

func main() {
	config = getConfig()
	metrics = ebpfmetrics.New(prometheus.DefaultRegisterer)

	logger = &splitLog{
		err:  log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)),
		rest: log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout)),
	}

	targetFinder, err := sd.NewTargetFinder(os.DirFS("/"), logger, convertTargetOptions())
	if err != nil {
		panic(fmt.Errorf("ebpf target finder create: %w", err))
	}
	options := convertSessionOptions()
	session, err = ebpfspy.NewSession(
		logger,
		targetFinder,
		options,
	)
	err = session.Start()
	if err != nil {
		panic(err)
	}

	profiles := make(chan *pushv1.PushRequest, 128)
	go ingest(profiles)

	discoverTicker := time.NewTicker(*discoverFreq)
	collectTicker := time.NewTicker(*collectFreq)

	for {
		select {
		case <-discoverTicker.C:
			session.UpdateTargets(convertTargetOptions())
		case <-collectTicker.C:
			collectProfiles(profiles)
		}
	}
}

func collectProfiles(profiles chan *pushv1.PushRequest) {
	builders := pprof.NewProfileBuilders(pprof.BuildersOptions{
		SampleRate:    int64(config.SampleRate),
		PerPIDProfile: true,
	})
	err := pprof.Collect(builders, session)

	if err != nil {
		panic(err)
	}
	level.Debug(logger).Log("msg", "ebpf collectProfiles done", "profiles", len(builders.Builders))

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
		req := &pushv1.PushRequest{Series: []*pushv1.RawProfileSeries{{
			Labels: protoLabels,
			Samples: []*pushv1.RawSample{{
				RawProfile: buf.Bytes(),
			}},
		}}}
		select {
		case profiles <- req:
		default:
			_ = level.Error(logger).Log("err", "dropping profile", "target", builder.Labels.String())
		}

	}

	if err != nil {
		panic(err)
	}
}

func ingest(profiles chan *pushv1.PushRequest) {
	httpClient, err := commonconfig.NewClientFromConfig(commonconfig.DefaultHTTPClientConfig, "http_playground")
	if err != nil {
		panic(err)
	}
	client := pushv1connect.NewPusherServiceClient(httpClient, *server)

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

func convertTargetOptions() sd.TargetsOptions {
	return sd.TargetsOptions{
		TargetsOnly:        config.TargetsOnly,
		Targets:            relabelProcessTargets(getProcessTargets(), config.RelabelConfig),
		DefaultTarget:      config.DefaultTarget,
		ContainerCacheSize: config.ContainerCacheSize,
	}
}

func convertSessionOptions() ebpfspy.SessionOptions {
	return ebpfspy.SessionOptions{
		CollectUser:               config.CollectUser,
		CollectKernel:             config.CollectKernel,
		SampleRate:                config.SampleRate,
		UnknownSymbolAddress:      config.UnknownSymbolAddress,
		UnknownSymbolModuleOffset: config.UnknownSymbolModuleOffset,
		PythonEnabled:             config.PythonEnabled,
		Metrics:                   metrics,
		CacheOptions:              config.CacheOptions,
		VerifierLogSize:           1024 * 1024 * 20,
		PythonBPFErrorLogEnabled:  config.PythonBPFLogErr,
		PythonBPFDebugLogEnabled:  config.PythonBPFLogDebug,
	}
}

func getConfig() *Config {
	flag.Parse()

	if *configFile == "" {
		panic("config file not specified")
	}
	var config = new(Config)
	*config = defaultConfig
	configBytes, err := os.ReadFile(*configFile)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(configBytes, config)
	if err != nil {
		panic(err)
	}
	return config
}

var defaultConfig = Config{
	CollectUser:               true,
	CollectKernel:             true,
	UnknownSymbolModuleOffset: true,
	UnknownSymbolAddress:      true,
	PythonEnabled:             true,
	SymbolOptions: symtab.SymbolOptions{
		GoTableFallback:    true,
		PythonFullFilePath: false,
		DemangleOptions:    demangle.DemangleFull,
	},
	CacheOptions: symtab.CacheOptions{

		PidCacheOptions: symtab.GCacheOptions{
			Size:       239,
			KeepRounds: 8,
		},
		BuildIDCacheOptions: symtab.GCacheOptions{
			Size:       239,
			KeepRounds: 8,
		},
		SameFileCacheOptions: symtab.GCacheOptions{
			Size:       239,
			KeepRounds: 8,
		},
	},
	SampleRate:         97,
	TargetsOnly:        true,
	DefaultTarget:      nil,
	ContainerCacheSize: 1024,
	RelabelConfig:      nil,
	PythonBPFLogErr:    true,
	PythonBPFLogDebug:  true,
}

type Config struct {
	CollectUser               bool
	CollectKernel             bool
	UnknownSymbolModuleOffset bool
	UnknownSymbolAddress      bool
	PythonEnabled             bool
	SymbolOptions             symtab.SymbolOptions
	CacheOptions              symtab.CacheOptions
	SampleRate                int
	TargetsOnly               bool
	DefaultTarget             map[string]string
	ContainerCacheSize        int
	RelabelConfig             []*RelabelConfig
	PythonBPFLogErr           bool
	PythonBPFLogDebug         bool
}

type RelabelConfig struct {
	SourceLabels []string

	Separator string

	Regex string

	TargetLabel string `yaml:"target_label,omitempty"`

	Replacement string `yaml:"replacement,omitempty"`

	Action string
}

func getProcessTargets() []sd.DiscoveryTarget {
	dir, err := os.ReadDir("/proc")
	if err != nil {
		panic(err)
	}
	var res []sd.DiscoveryTarget
	for _, entry := range dir {
		if !entry.IsDir() {
			continue
		}
		spid := entry.Name()
		pid, err := strconv.ParseUint(spid, 10, 32)
		if err != nil {
			continue
		}
		if pid == 0 {
			continue
		}
		cwd, err := os.Readlink(fmt.Sprintf("/proc/%s/cwd", spid))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(logger).Log("err", err, "msg", "reading cwd", "pid", spid)
			}
			continue
		}
		exe, err := os.Readlink(fmt.Sprintf("/proc/%s/exe", spid))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(logger).Log("err", err, "msg", "reading exe", "pid", spid)
			}
			continue
		}
		comm, err := os.ReadFile(fmt.Sprintf("/proc/%s/comm", spid))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(logger).Log("err", err, "msg", "reading comm", "pid", spid)
			}
		}
		cgroup, err := os.ReadFile(fmt.Sprintf("/proc/%s/cgroup", spid))
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				_ = level.Error(logger).Log("err", err, "msg", "reading cgroup", "pid", spid)
			}
		}
		target := sd.DiscoveryTarget{
			"__process_pid__":       spid,
			"__meta_process_cwd":    cwd,
			"__meta_process_exe":    exe,
			"__meta_process_comm":   string(comm),
			"__meta_process_cgroup": string(cgroup),
			"pid":                   spid,
			"exe":                   exe,
		}
		res = append(res, target)
	}
	return res
}

func relabelProcessTargets(targets []sd.DiscoveryTarget, cfg []*RelabelConfig) []sd.DiscoveryTarget {
	var promConfig []*relabel.Config
	for _, c := range cfg {
		var srcLabels model.LabelNames
		for _, label := range c.SourceLabels {
			srcLabels = append(srcLabels, model.LabelName(label))
		}
		promConfig = append(promConfig, &relabel.Config{
			SourceLabels: srcLabels,
			Separator:    c.Separator,
			Regex:        relabel.MustNewRegexp(c.Regex),
			TargetLabel:  c.TargetLabel,
			Replacement:  c.Replacement,
			Action:       relabel.Action(c.Action),
		})
	}
	var res []sd.DiscoveryTarget
	for _, target := range targets {
		lbls := labels.FromMap(target)
		lbls, keep := relabel.Process(lbls, promConfig...)
		if !keep {
			continue
		}
		tt := sd.DiscoveryTarget(lbls.Map())
		res = append(res, tt)
	}
	return res
}
