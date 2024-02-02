package parca

import (
	"context"
	"fmt"
	"os"
	runtimepprof "runtime/pprof"
	"time"

	"github.com/cilium/ebpf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/parca-dev/parca-agent/pkg/debuginfo"
	"github.com/parca-dev/parca-agent/pkg/metadata/labels"
	"github.com/parca-dev/parca-agent/pkg/objectfile"
	"github.com/parca-dev/parca-agent/pkg/process"
	"github.com/parca-dev/parca-agent/pkg/runtime"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

type Parca struct {
	logger log.Logger
	g      run.Group
	cpu    *CPU
}

func (p *Parca) Entrypoint() *ebpf.Program {
	return p.cpu.bpfMaps.modules.ParcaNativeObjects.ParcaNativePrograms.Entrypoint
}

func NewParca(logger log.Logger, profilingDuration time.Duration) (*Parca, error) {
	p := new(Parca)
	reg := prometheus.NewRegistry()

	var (
		tp trace.TracerProvider = noop.NewTracerProvider()
	)

	pfs, err := procfs.NewDefaultFS()
	if err != nil {
		return nil, fmt.Errorf("failed to open procfs: %w", err)
	}
	ofp := objectfile.NewPool(logger, reg,
		"lru",
		100,
		profilingDuration)

	var dbginfo process.DebuginfoManager = debuginfo.NoopDebuginfoManager{}

	labelsManager := labels.NewManager(
		log.With(logger, "component", "labels_manager"),
		tp.Tracer("labels_manager"),
		reg,
		nil,
		nil,
		false,
		profilingDuration, // Cache durations are calculated from profiling duration.
	)

	processInfoManager := process.NewInfoManager(
		log.With(logger, "component", "process_info"),
		tp.Tracer("process_info"),
		reg,
		pfs,
		ofp,
		process.NewMapManager(
			reg,
			pfs,
			ofp,
		),
		dbginfo,
		labelsManager,
		profilingDuration,
		5*time.Minute,
	)
	{
		logger := log.With(logger, "group", "process_info_manager")
		ctx, cancel := context.WithCancel(context.Background())
		p.g.Add(func() error {
			level.Debug(logger).Log("msg", "starting")
			defer level.Debug(logger).Log("msg", "stopped")

			return processInfoManager.Run(ctx)
		}, func(error) {
			cancel()
			processInfoManager.Close()
		})
	}

	compilerInfoManager := runtime.NewCompilerInfoManager(reg, ofp)

	bpfProgramLoaded := make(chan bool, 1)
	p.cpu = NewCPUProfiler(
		log.With(logger, "component", "cpu_profiler"),
		reg,
		processInfoManager,
		compilerInfoManager,

		&Config{
			ProfilingDuration:                 profilingDuration,
			ProfilingSamplingFrequency:        0,
			PerfEventBufferPollInterval:       250 * time.Millisecond,
			PerfEventBufferProcessingInterval: 100 * time.Millisecond,
			PerfEventBufferWorkerCount:        4,
			MemlockRlimit:                     0,
			DebugProcessNames:                 nil,
			DWARFUnwindingDisabled:            false,
			DWARFUnwindingMixedModeEnabled:    false,
			BPFVerboseLoggingEnabled:          false,
			BPFEventsBufferSize:               uint32(8 * os.Getpagesize()),
			PythonUnwindingEnabled:            false,
			RubyUnwindingEnabled:              true,
			RateLimitUnwindInfo:               50,
			RateLimitProcessMappings:          50,
			RateLimitRefreshProcessInfo:       50,
		},
		bpfProgramLoaded,
	)

	// Run profilers.
	{
		ctx, cancel := context.WithCancel(context.Background())

		cpu := p.cpu
		logger := log.With(logger, "group", "profiler/"+cpu.Name())
		p.g.Add(func() error {
			level.Debug(logger).Log("msg", "starting", "name", cpu.Name())
			defer level.Debug(logger).Log("msg", "stopped", "profiler", cpu.Name())

			var err error
			runtimepprof.Do(ctx, runtimepprof.Labels("component", cpu.Name()), func(ctx context.Context) {
				err = cpu.Run(ctx)
			})

			return err
		}, func(error) {
			level.Debug(logger).Log("msg", "cleaning up")
			defer level.Debug(logger).Log("msg", "cleanup finished")

			cancel()
		})
	}

	go func() {
		err := p.g.Run()
		if err != nil {
			level.Error(logger).Log("msg", "run group failed", "err", err)
			return
		}
		level.Info(logger).Log("msg", "run group finished")
	}()
	<-bpfProgramLoaded

	return p, nil
}

func (p *Parca) Dump() DumpResponse {
	return p.cpu.Dump()
}
