// Copyright 2022-2024 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package parca

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"regexp"
	goruntime "runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/parca-dev/parca-agent/pkg/buildid"
	"github.com/parca-dev/parca-agent/pkg/byteorder"
	"github.com/parca-dev/parca-agent/pkg/cache"
	"github.com/parca-dev/parca-agent/pkg/profile"
	"github.com/parca-dev/parca-agent/pkg/profiler"
	bpfmetrics "github.com/parca-dev/parca-agent/pkg/profiler/cpu/bpf/metrics"
	bpfprograms "github.com/parca-dev/parca-agent/pkg/profiler/cpu/bpf/programs"
	"github.com/parca-dev/parca-agent/pkg/runtime"
	"github.com/parca-dev/parca-agent/pkg/stack/unwind"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/procfs"
	"github.com/puzpuzpuz/xsync/v3"
)

type DumpRequest struct {
	Response chan DumpResponse
}

type DumpResponse struct {
	PID2Data               map[int]profile.ProcessRawData
	InterpreterSymbolTable profile.InterpreterSymbolTable
}

const (
	configKey = "unwinder_config"
)

// UnwinderConfig must be synced to the C definition.
type UnwinderConfig struct {
	FilterProcesses             bool
	VerboseLogging              bool
	MixedStackWalking           bool
	PythonEnable                bool
	RubyEnabled                 bool
	Padding1                    bool
	Padding2                    bool
	Padding3                    bool
	RateLimitUnwindInfo         uint32
	RateLimitProcessMappings    uint32
	RateLimitRefreshProcessInfo uint32
}

type Config struct {
	ProfilingDuration          time.Duration
	ProfilingSamplingFrequency uint64

	PerfEventBufferPollInterval       time.Duration
	PerfEventBufferProcessingInterval time.Duration
	PerfEventBufferWorkerCount        int

	MemlockRlimit uint64

	DebugProcessNames []string

	DWARFUnwindingDisabled         bool
	DWARFUnwindingMixedModeEnabled bool
	BPFVerboseLoggingEnabled       bool
	BPFEventsBufferSize            uint32

	PythonUnwindingEnabled bool
	RubyUnwindingEnabled   bool

	RateLimitUnwindInfo         uint32
	RateLimitProcessMappings    uint32
	RateLimitRefreshProcessInfo uint32
}

func (c Config) DebugModeEnabled() bool {
	return len(c.DebugProcessNames) > 0
}

type CPU struct {
	config *Config

	logger  log.Logger
	reg     prometheus.Registerer
	metrics *metrics

	processInfoManager profiler.ProcessInfoManager

	// Notify that the BPF program was loaded.
	bpfProgramLoaded chan bool
	bpfMaps          *Maps

	framePointerCache unwind.FramePointerCache
	interpSymTab      profile.InterpreterSymbolTable

	byteOrder binary.ByteOrder

	mtx                            *sync.RWMutex
	lastError                      error
	processLastErrors              map[int]error
	processErrorTracker            *errorTracker
	lastSuccessfulProfileStartedAt time.Time
	lastProfileStartedAt           time.Time

	requests chan DumpRequest
}

func NewCPUProfiler(
	logger log.Logger,
	reg prometheus.Registerer,
	processInfoManager profiler.ProcessInfoManager,
	compilerInfoManager *runtime.CompilerInfoManager,
	config *Config,
	bpfProgramLoaded chan bool,
) *CPU {
	return &CPU{
		config: config,

		logger:  logger,
		reg:     reg,
		metrics: newMetrics(reg),

		processInfoManager: processInfoManager,

		// CPU profiler specific caches.
		framePointerCache: unwind.NewHasFramePointersCache(logger, reg, compilerInfoManager),

		byteOrder: byteorder.GetHostByteOrder(),

		mtx: &sync.RWMutex{},
		// increase cache length if needed to track more errors
		processErrorTracker: newErrorTracker(logger, reg, "no_text_section_error_tracker"),

		bpfProgramLoaded: bpfProgramLoaded,

		requests: make(chan DumpRequest),
	}
}

func (p *CPU) Name() string {
	return "parca_agent_cpu"
}

func (p *CPU) LastProfileStartedAt() time.Time {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.lastProfileStartedAt
}

func (p *CPU) LastError() error {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.lastError
}

func (p *CPU) ProcessLastErrors() map[int]error {
	p.mtx.RLock()
	defer p.mtx.RUnlock()
	return p.processLastErrors
}

// loadBPFModules loads the BPF programs and maps.
// Also adjusts the unwind shards to the highest possible value.
// And configures shared maps between BPF programs.
func loadBPFModules(logger log.Logger, reg prometheus.Registerer, memlockRlimit uint64, config Config) (*Maps, error) {
	defer btf.FlushKernelSpec() // save some memory
	var lerr error

	maxLoadAttempts := 10
	unwindShards := uint32(MaxUnwindShards)

	nativeSpec, err := LoadParcaNative()
	if err != nil {
		return nil, err
	}

	modules := &Modules{
		ParcaNativeSpec: nativeSpec,
	}

	if config.RubyUnwindingEnabled {
		// rbperf
		rubySpec, err := LoadParcaRuby()
		if err != nil {
			return nil, err
		}
		modules.RubySpec = rubySpec

		level.Info(logger).Log("msg", "loaded rbperf BPF module")
	}

	if config.PythonUnwindingEnabled {
		// pyperf
		pythonSpec, err := LoadParcaPython()
		if err != nil {
			return nil, err
		}
		modules.PythonSpec = pythonSpec
		level.Info(logger).Log("msg", "loaded pyperf BPF module")
	}

	bpfmapMetrics := NewMapsMetrics(reg)
	bpfmapsProcessCache := NewProcessCache(reg)
	syncedIntepreters := cache.NewLRUCache[int, runtime.Interpreter](
		prometheus.WrapRegistererWith(prometheus.Labels{"cache": "synced_interpreters"}, reg),
		MaxCachedProcesses/10,
	)

	// Adaptive unwind shard count sizing.
	for i := 0; i < maxLoadAttempts; i++ {

		// Maps must be initialized before loading the BPF code.
		bpfMaps, err := NewMaps(
			logger,
			binary.LittleEndian,
			getArch(),
			modules,
			bpfmapMetrics,
			bpfmapsProcessCache,
			syncedIntepreters,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize eBPF maps: %w", err)
		}

		if config.DWARFUnwindingDisabled {
			// Even if DWARF-based unwinding is disabled, either due to the user passing the flag to disable it or running on arm64, still
			// create a handful of shards to ensure that when it is enabled we can at least create some shards. Basically we want to ensure
			// that we catch any potential issues as early as possible.
			unwindShards = uint32(5)
		}

		level.Debug(logger).Log("msg", "attempting to create unwind shards", "count", unwindShards)
		if err := bpfMaps.AdjustMapSizes(config.DebugModeEnabled(), unwindShards, config.BPFEventsBufferSize); err != nil {
			return nil, fmt.Errorf("failed to adjust map sizes: %w", err)
		}
		level.Debug(logger).Log("msg", "created unwind shards", "count", unwindShards)

		level.Debug(logger).Log("msg", "initializing BPF global variables")
		if err := nativeSpec.RewriteConstants(map[string]interface{}{configKey: UnwinderConfig{
			FilterProcesses:             config.DebugModeEnabled(),
			VerboseLogging:              config.BPFVerboseLoggingEnabled,
			MixedStackWalking:           config.DWARFUnwindingMixedModeEnabled,
			PythonEnable:                config.PythonUnwindingEnabled,
			RubyEnabled:                 config.RubyUnwindingEnabled,
			Padding1:                    false,
			Padding2:                    false,
			Padding3:                    false,
			RateLimitUnwindInfo:         config.RateLimitUnwindInfo,
			RateLimitProcessMappings:    config.RateLimitProcessMappings,
			RateLimitRefreshProcessInfo: config.RateLimitRefreshProcessInfo,
		}}); err != nil {
			return nil, fmt.Errorf("init global variable: %w", err)
		}

		if config.RubyUnwindingEnabled {
			if err := modules.RubySpec.RewriteConstants(map[string]interface{}{
				"verbose": config.BPFVerboseLoggingEnabled,
			}); err != nil {
				return nil, fmt.Errorf("rbperf: init global variable: %w", err)
			}
		}
		//
		if config.PythonUnwindingEnabled {
			if err := modules.PythonSpec.RewriteConstants(map[string]interface{}{"verbose": config.BPFVerboseLoggingEnabled}); err != nil {
				return nil, fmt.Errorf("pyperf: init global variable: %w", err)
			}
		}

		level.Debug(logger).Log("msg", "loading BPF object for native unwinder")
		nativeObjs := new(ParcaNativeObjects)
		lerr = nativeSpec.LoadAndAssign(nativeObjs, new(ebpf.CollectionOptions))
		if lerr == nil {
			modules.ParcaNativeObjects = nativeObjs
			// Must be called before loading the interpreter stack walkers.
			err := bpfMaps.ReuseMaps()
			if err != nil {
				return nil, fmt.Errorf("failed to reuse maps: %w", err)
			}

			if config.RubyUnwindingEnabled {
				level.Debug(logger).Log("msg", "loading BPF object for ruby unwinder")
				rubyObject := new(ParcaRubyObjects)
				opts := new(ebpf.CollectionOptions)
				opts.MapReplacements = modules.MapReplacements
				err = modules.RubySpec.LoadAndAssign(rubyObject, opts)
				if err != nil {
					return nil, fmt.Errorf("failed to load rbperf: %w", err)
				}
				modules.RubyObjects = rubyObject
			}

			if config.PythonUnwindingEnabled {
				level.Debug(logger).Log("msg", "loading BPF object for python unwinder")
				pythonObjects := new(ParcaPythonObjects)
				opts := new(ebpf.CollectionOptions)
				opts.MapReplacements = modules.MapReplacements
				err = modules.PythonSpec.LoadAndAssign(pythonObjects, opts)
				if err != nil {
					return nil, fmt.Errorf("failed to load pyperf: %w", err)
				}
				modules.PythonObjects = pythonObjects
			}

			level.Debug(logger).Log("msg", "updating programs map")
			err = bpfMaps.UpdateTailCallsMap()
			if err != nil {
				return nil, fmt.Errorf("failed to update programs map: %w", err)
			}
			//
			level.Debug(logger).Log("msg", "updating interpreter data")
			err = bpfMaps.SetInterpreterData()
			if err != nil {
				return nil, fmt.Errorf("failed to set interpreter data: %w", err)
			}

			return bpfMaps, nil
		}

		// There's not enough free memory for these many unwind shards, let's retry with half
		// as many.
		if errors.Is(lerr, syscall.ENOMEM) {
			if err := bpfMaps.Close(); err != nil { // Only required when we want to retry.
				return nil, fmt.Errorf("failed to cleanup previously created bpfmaps: %w", err)
			}
			unwindShards /= 2
		} else {
			break
		}
	}

	level.Error(logger).Log("msg", "could not create unwind info shards", "lastError", lerr)
	return nil, lerr
}

// listenEvents listens for events from the BPF program and handles them.
// It also listens for lost events and logs them.
func (p *CPU) listenEvents(ctx context.Context, eventsChan <-chan []byte, lostChan <-chan uint64, requestUnwindInfoChan chan<- int) {
	prefetch := make(chan int, p.config.PerfEventBufferWorkerCount*4)
	refresh := make(chan int, p.config.PerfEventBufferWorkerCount*2)
	defer func() {
		close(prefetch)
		close(refresh)
	}()

	var (
		fetchInProgress   = xsync.NewMapOf[int, struct{}]()
		refreshInProgress = xsync.NewMapOf[int, struct{}]()
	)
	for i := 0; i < p.config.PerfEventBufferWorkerCount; i++ {
		go func() {
			for {
				select {
				case pid, open := <-prefetch:
					if !open {
						return
					}
					_ = p.prefetchProcessInfo(ctx, pid)
					fetchInProgress.Delete(pid)
				case pid, open := <-refresh:
					if !open {
						return
					}
					if err := p.fetchProcessInfoWithFreshMappings(ctx, pid); err != nil {
						return
					}

					executable := fmt.Sprintf("/proc/%d/exe", pid)
					shouldUseFPByDefault, err := p.framePointerCache.HasFramePointers(executable) // nolint:contextcheck
					if err != nil {
						// It might not exist as reading procfs is racy. If the executable has no symbols
						// that we use as a heuristic to detect whether it has frame pointers or not,
						// we assume it does not and that we should generate the unwind information.
						level.Debug(p.logger).Log("msg", "frame pointer detection failed", "executable", executable, "err", err)
						if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, elf.ErrNoSymbols) {
							return
						}
					}

					// Process information has been refreshed, now refresh the mappings and their unwind info.
					p.bpfMaps.RefreshProcessInfo(pid, shouldUseFPByDefault)
					refreshInProgress.Delete(pid)
				}
			}
		}()
	}

	for {
		select {
		case receivedBytes, open := <-eventsChan:
			if !open {
				return
			}
			if len(receivedBytes) == 0 {
				p.metrics.eventsReceived.WithLabelValues(labelEventEmpty).Inc()
				continue
			}

			payload := binary.LittleEndian.Uint64(receivedBytes)
			// Get the 4 more significant bytes and convert to int as they are different types.
			// On x86_64:
			//	- unsafe.Sizeof(int(0)) = 8
			//	- unsafe.Sizeof(uint32(0)) = 4
			pid := int(int32(payload))
			switch {
			case payload&RequestUnwindInformation == RequestUnwindInformation:
				if p.config.DWARFUnwindingDisabled {
					continue
				}
				p.metrics.eventsReceived.WithLabelValues(labelEventUnwindInfo).Inc()
				// See onDemandUnwindInfoBatcher for consumer.
				requestUnwindInfoChan <- pid
			case payload&RequestProcessMappings == RequestProcessMappings:
				p.metrics.eventsReceived.WithLabelValues(labelEventProcessMappings).Inc()
				if _, exists := fetchInProgress.LoadOrStore(pid, struct{}{}); exists {
					continue
				}
				prefetch <- pid
			case payload&RequestRefreshProcInfo == RequestRefreshProcInfo:
				p.metrics.eventsReceived.WithLabelValues(labelEventRefreshProcInfo).Inc()
				// Refresh mappings and their unwind info if they've changed.
				if _, exists := refreshInProgress.LoadOrStore(pid, struct{}{}); exists {
					continue
				}
				refresh <- pid
			}
		case lost, open := <-lostChan:
			if !open {
				return
			}
			p.metrics.eventsLost.Inc()
			level.Warn(p.logger).Log("msg", "lost events", "count", lost)
		default:
			time.Sleep(p.config.PerfEventBufferProcessingInterval)
		}
	}
}

func (p *CPU) prefetchProcessInfo(ctx context.Context, pid int) error {
	procInfo, err := p.processInfoManager.Fetch(ctx, pid)
	if err != nil {
		level.Debug(p.logger).Log("msg", "failed to prefetch process info", "pid", pid, "err", err)
		return fmt.Errorf("failed to prefetch process info: %w", err)
	}

	if procInfo.Interpreter != nil {
		// AddInterpreter is idempotent.
		err := p.bpfMaps.AddInterpreter(pid, *procInfo.Interpreter)
		if err != nil {
			level.Debug(p.logger).Log("msg", "failed to call AddInterpreter", "pid", pid, "err", err)
			return fmt.Errorf("failed to call AddInterpreter: %w", err)
		}
	}
	return nil
}

// fetchProcessInfoWithFreshMappings fetches process information and makes sure its mappings are up-to-date.
func (p *CPU) fetchProcessInfoWithFreshMappings(ctx context.Context, pid int) error {
	procInfo, err := p.processInfoManager.FetchWithFreshMappings(ctx, pid)
	if err != nil {
		level.Debug(p.logger).Log("msg", "failed to fetch process info", "pid", pid, "err", err)
		return fmt.Errorf("failed to fetch process info: %w", err)
	}

	if procInfo.Interpreter != nil {
		// AddInterpreter is idempotent.
		err := p.bpfMaps.AddInterpreter(pid, *procInfo.Interpreter)
		if err != nil {
			level.Debug(p.logger).Log("msg", "failed to call AddInterpreter", "pid", pid, "err", err)
			return fmt.Errorf("failed to call AddInterpreter: %w", err)
		}
	}
	return nil
}

// onDemandUnwindInfoBatcher batches PIDs sent from the BPF program when
// frame pointers and unwind information are not present.
//
// Waiting for as long as `duration` is important because `PersistUnwindTable`
// must be called to write the in-flight shard to the BPF map. This has been
// a hot path in the CPU profiles we take in Demo when we persisted the unwind
// tables after adding every pid.
func (p *CPU) onDemandUnwindInfoBatcher(ctx context.Context, requestUnwindInfoChannel <-chan int) {
	processEventBatcher(ctx, requestUnwindInfoChannel, 150*time.Millisecond, func(pids []int) {
		for _, pid := range pids {
			p.addUnwindTableForProcess(ctx, pid)
		}

		// Must be called after all the calls to `addUnwindTableForProcess`, as it's possible
		// that the current in-flight shard hasn't been written to the BPF map, yet.
		err := p.bpfMaps.PersistUnwindTable()
		if err != nil {
			if errors.Is(err, ErrNeedMoreProfilingRounds) {
				p.metrics.unwindTablePersistErrors.WithLabelValues(labelNeedMoreProfilingRounds).Inc()
				level.Debug(p.logger).Log("msg", "PersistUnwindTable called to soon", "err", err)
			} else {
				p.metrics.unwindTablePersistErrors.WithLabelValues(labelOther).Inc()
				level.Error(p.logger).Log("msg", "PersistUnwindTable failed", "err", err)
			}
		}
	})
}

func (p *CPU) addUnwindTableForProcess(ctx context.Context, pid int) {
	executable := fmt.Sprintf("/proc/%d/exe", pid)
	shouldUseFPByDefault, err := p.framePointerCache.HasFramePointers(executable) // nolint:contextcheck
	if err != nil {
		// It might not exist as reading procfs is racy. If the executable has no symbols
		// that we use as a heuristic to detect whether it has frame pointers or not,
		// we assume it does not and that we should generate the unwind information.
		level.Debug(p.logger).Log("msg", "frame pointer detection failed", "executable", executable, "err", err)
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, elf.ErrNoSymbols) {
			return
		}
	}

	level.Debug(p.logger).Log("msg", "prefetching process info", "pid", pid)
	if err := p.prefetchProcessInfo(ctx, pid); err != nil {
		return
	}

	level.Debug(p.logger).Log("msg", "adding unwind tables", "pid", pid)
	if err = p.bpfMaps.AddUnwindTableForProcess(pid, nil, true, shouldUseFPByDefault); err == nil {
		// Happy path.
		return
	}

	// Error handling,
	switch {
	case errors.Is(err, ErrNeedMoreProfilingRounds):
		p.metrics.unwindTableAddErrors.WithLabelValues(labelNeedMoreProfilingRounds).Inc()
		level.Debug(p.logger).Log("msg", "PersistUnwindTable called to soon", "err", err)
	case errors.Is(err, os.ErrNotExist):
		p.metrics.unwindTableAddErrors.WithLabelValues(labelProcfsRace).Inc()
		level.Debug(p.logger).Log("msg", "failed to add unwind table due to a procfs race", "pid", pid, "err", err)
	case errors.Is(err, ErrTooManyExecutableMappings):
		p.metrics.unwindTableAddErrors.WithLabelValues(labelTooManyMappings).Inc()
		level.Warn(p.logger).Log("msg", "failed to add unwind table due to having too many executable mappings", "pid", pid, "err", err)
	case errors.Is(err, buildid.ErrTextSectionNotFound):
		p.processErrorTracker.Track(pid, err)
	default:
		p.metrics.unwindTableAddErrors.WithLabelValues(labelOther).Inc()
		level.Error(p.logger).Log("msg", "failed to add unwind table", "pid", pid, "err", err)
	}
}

// processEventBatcher batches PIDs sent from the BPF program.
//
// Waits for as long as `duration` and calls the `callback` function with a slice of PIDs.
func processEventBatcher(ctx context.Context, eventsChannel <-chan int, duration time.Duration, callback func([]int)) {
	batch := make([]int, 0)
	timerOn := false
	timer := &time.Timer{}
	for {
		select {
		case <-ctx.Done():
			return
		case pid := <-eventsChannel:
			// We want to set a deadline whenever an event is received, if there is
			// no other deadline in progress. During this time period we'll batch
			// all the events received. Once time's up, we will pass the batch to
			// the callback.
			if !timerOn {
				timerOn = true
				timer = time.NewTimer(duration)
			}
			batch = append(batch, pid)
		case <-timer.C:
			callback(batch)
			batch = batch[:0]
			timerOn = false
			timer.Stop()
		}
	}
}

// TODO(kakkoyun): Combine with process information discovery.
func (p *CPU) watchProcesses(ctx context.Context, pfs procfs.FS, matchers []*regexp.Regexp) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		pids := []int{}

		allThreads := func() procfs.Procs {
			allProcs, err := pfs.AllProcs()
			if err != nil {
				level.Error(p.logger).Log("msg", "failed to list processes", "err", err)
				return nil
			}

			allThreads := make(procfs.Procs, len(allProcs))
			for _, proc := range allProcs {
				threads, err := pfs.AllThreads(proc.PID)
				if err != nil {
					level.Debug(p.logger).Log("msg", "failed to list threads", "err", err)
					continue
				}
				allThreads = append(allThreads, threads...)
			}
			return allThreads
		}

		// Filter processes if needed.
		if p.config.DebugModeEnabled() {
			level.Debug(p.logger).Log("msg", "debug process matchers found, starting process watcher")

			for _, thread := range allThreads() {
				if thread.PID == 0 {
					continue
				}
				comm, err := thread.Comm()
				if err != nil {
					level.Debug(p.logger).Log("msg", "failed to read process name", "pid", thread.PID, "err", err)
					continue
				}

				if comm == "" {
					continue
				}

				for _, m := range matchers {
					if m.MatchString(comm) {
						level.Info(p.logger).Log("msg", "match found; debugging process", "pid", thread.PID, "comm", comm)
						pids = append(pids, thread.PID)
					}
				}
			}

			if len(pids) > 0 {
				level.Debug(p.logger).Log("msg", "updating debug pids map", "pids", fmt.Sprintf("%v", pids))
				// Only meant to be used for debugging, it is not safe to use in production.
				if err := p.bpfMaps.SetDebugPIDs(pids); err != nil {
					level.Error(p.logger).Log("msg", "failed to update debug pids map", "err", err)
				}
			} else {
				level.Debug(p.logger).Log("msg", "no processes matched the provided regex")
				if err := p.bpfMaps.SetDebugPIDs(nil); err != nil {
					level.Error(p.logger).Log("msg", "failed to update debug pids map", "err", err)
				}
			}
		} else {
			for _, thread := range allThreads() {
				pids = append(pids, thread.PID)
			}
		}
	}
}

func bpfCheck() error {
	var result error

	//if support, err := libbpf.BPFProgramTypeIsSupported(libbpf.BPFProgTypePerfEvent); !support {
	//	result = errors.Join(result, fmt.Errorf("perf event program type not supported: %w", err))
	//}
	//
	//if support, err := libbpf.BPFMapTypeIsSupported(libbpf.MapTypeStackTrace); !support {
	//	result = errors.Join(result, fmt.Errorf("stack trace map type not supported: %w", err))
	//}
	//
	//if support, err := libbpf.BPFMapTypeIsSupported(libbpf.MapTypeHash); !support {
	//	result = errors.Join(result, fmt.Errorf("hash map type not supported: %w", err))
	//}

	return result
}

func (p *CPU) Run(ctx context.Context) error {
	level.Debug(p.logger).Log("msg", "starting cpu profiler")

	err := bpfCheck()
	if err != nil {
		return fmt.Errorf("bpf check: %w", err)
	}

	var matchers []*regexp.Regexp
	if p.config.DebugModeEnabled() {
		level.Info(p.logger).Log("msg", "process names specified, debugging processes", "matchers", strings.Join(p.config.DebugProcessNames, ", "))
		for _, exp := range p.config.DebugProcessNames {
			regex, err := regexp.Compile(exp)
			if err != nil {
				return fmt.Errorf("failed to compile regex: %w", err)
			}
			matchers = append(matchers, regex)
		}
	}

	level.Debug(p.logger).Log("msg", "loading BPF modules")
	bpfMaps, err := loadBPFModules(p.logger, p.reg, p.config.MemlockRlimit, *p.config)
	if err != nil {
		return fmt.Errorf("load bpf program: %w", err)
	}
	defer bpfMaps.Close()
	level.Debug(p.logger).Log("msg", "BPF modules loaded")

	p.bpfMaps = bpfMaps
	p.bpfProgramLoaded <- true

	// Get bpf metrics
	agentProc, err := procfs.Self() // pid of parca-agent
	if err != nil {
		level.Debug(p.logger).Log("msg", "error getting parca-agent pid", "err", err)
	}

	p.reg.MustRegister(bpfmetrics.NewCollector(p.logger, "percpu_stats", agentProc.PID))

	// Record start time for first profile.
	p.mtx.Lock()
	p.lastProfileStartedAt = time.Now()
	p.mtx.Unlock()

	prog := p.bpfMaps.modules.ParcaNativeObjects.ParcaNativePrograms.NativeUnwind
	programs := p.bpfMaps.modules.ParcaNativeObjects.ParcaNativeMaps.Programs

	if err := programs.Update(uint32(bpfprograms.NativeProgramFD), prog, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failure updating: %w", err)
	}

	if err := p.bpfMaps.Create(); err != nil {
		return fmt.Errorf("failed to create maps: %w", err)
	}

	pfs, err := procfs.NewDefaultFS()
	if err != nil {
		return fmt.Errorf("failed to create procfs: %w", err)
	}

	if len(matchers) > 0 {
		// Update the debug pids map.
		go p.watchProcesses(ctx, pfs, matchers)
	}

	// Process BPF events.
	var (
		eventsChan  = make(chan []byte, 30)
		lostChannel = make(chan uint64, 10)
	)
	eventsReader, err := NewEventsReader(p.logger, p.bpfMaps.modules.ParcaNativeObjects.ParcaNativeMaps.Events, eventsChan, true)
	if err != nil {
		return fmt.Errorf("failed to init perf buffer: %w", err)
	}
	eventsReader.Start()

	requestUnwindInfoChannel := make(chan int, 30)
	go p.listenEvents(ctx, eventsChan, lostChannel, requestUnwindInfoChannel)
	go p.onDemandUnwindInfoBatcher(ctx, requestUnwindInfoChannel)

	level.Debug(p.logger).Log("msg", "start profiling loop")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req := <-p.requests:
			p.handleRequest(ctx, req)
		}

	}
}

func (p *CPU) handleRequest(ctx context.Context, req DumpRequest) {
	res := DumpResponse{}
	defer func() {
		req.Response <- res
	}()
	obtainStart := time.Now()
	rawData, err := p.obtainRawData(ctx)
	if err != nil {
		p.metrics.obtainAttempts.WithLabelValues(labelError).Inc()
		level.Warn(p.logger).Log("msg", "failed to obtain profiles from eBPF maps", "err", err)
		return
	}
	p.metrics.obtainAttempts.WithLabelValues(labelSuccess).Inc()
	p.metrics.obtainDuration.Observe(time.Since(obtainStart).Seconds())

	groupedRawData := make(map[int]profile.ProcessRawData)
	for _, perThreadRawData := range rawData {
		pid := int(perThreadRawData.PID)
		data, ok := groupedRawData[pid]
		if !ok {
			groupedRawData[pid] = profile.ProcessRawData{
				PID:        perThreadRawData.PID,
				RawSamples: perThreadRawData.RawSamples,
			}
			continue
		}

		groupedRawData[pid] = profile.ProcessRawData{
			PID:        perThreadRawData.PID,
			RawSamples: append(data.RawSamples, perThreadRawData.RawSamples...),
		}
	}
	//res.PID2Data
	processLastErrors := map[int]error{}

	for pid, perProcessRawData := range groupedRawData {
		processLastErrors[pid] = nil
		res.InterpreterSymbolTable, err = p.interpreterSymbolTable(perProcessRawData.RawSamples)
		if err != nil {
			level.Debug(p.logger).Log("msg", "failed to get interpreter symbol table", "pid", pid, "err", err)
		}
	}
	res.PID2Data = groupedRawData

	p.report(err, processLastErrors)
}

func (p *CPU) report(lastError error, processLastErrors map[int]error) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if lastError == nil {
		p.lastSuccessfulProfileStartedAt = p.lastProfileStartedAt
		p.lastProfileStartedAt = time.Now()
	}
	p.lastError = lastError
	p.processLastErrors = processLastErrors
}

type (
	// stackCountKey mirrors the struct in BPF program.
	// NOTICE: The memory layout and alignment of the struct currently matches the struct in BPF program.
	// However, keep in mind that Go compiler injects padding to align the struct fields to be a multiple of 8 bytes.
	// The Go spec says the address of a struct’s fields must be naturally aligned.
	// https://dave.cheney.net/2015/10/09/padding-is-hard
	// TODO(https://github.com/parca-dev/parca-agent/issues/207)
	stackCountKey struct {
		PID                int32
		TID                int32
		UserStackID        uint64
		KernelStackID      uint64
		InterpreterStackID uint64
	}
)

type profileKey struct {
	pid int32
	tid int32
}

// interpreterSymbolTable returns an up-to-date symbol table for the interpreter.
func (p *CPU) interpreterSymbolTable(samples []profile.RawSample) (profile.InterpreterSymbolTable, error) {
	if !p.config.RubyUnwindingEnabled && !p.config.PythonUnwindingEnabled {
		return nil, nil
	}

	if p.interpSymTab == nil {
		if err := p.updateInterpreterSymbolTable(); err != nil {
			// Return the old version of the symbol table if we failed to update it.
			return p.interpSymTab, err
		}
		return p.interpSymTab, nil
	}

	for _, sample := range samples {
		if sample.InterpreterStack == nil {
			continue
		}

		for _, id := range sample.InterpreterStack {
			if _, ok := p.interpSymTab[uint32(id)]; !ok {
				if err := p.updateInterpreterSymbolTable(); err != nil {
					// Return the old version of the symbol table if we failed to update it.
					return p.interpSymTab, err
				}
				// We only need to update the symbol table once.
				return p.interpSymTab, nil
			}
		}
	}
	// The symbol table is up-to-date.
	return p.interpSymTab, nil
}

func (p *CPU) updateInterpreterSymbolTable() error {
	interpSymTab, err := p.bpfMaps.InterpreterSymbolTable()
	if err != nil {
		return fmt.Errorf("get interpreter symbol table: %w", err)
	}
	p.interpSymTab = interpSymTab
	return nil
}

// obtainProfiles collects profiles from the BPF maps.
func (p *CPU) obtainRawData(ctx context.Context) (profile.RawData, error) {
	rawData := map[profileKey]map[bpfprograms.CombinedStack]uint64{}

	it := p.bpfMaps.modules.ParcaNativeObjects.StackCounts.Iterate()
	keyBytes := make([]byte, p.bpfMaps.modules.ParcaNativeObjects.StackCounts.KeySize())
	//todo value is not used, todo use batch
	valueBytes := make([]byte, p.bpfMaps.modules.ParcaNativeObjects.StackCounts.ValueSize())
	for it.Next(keyBytes, valueBytes) {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		var key stackCountKey
		// NOTICE: This works because the key struct in Go and the key struct in C has exactly the same memory layout.
		// See the comment in stackCountKey for more details.
		if err := binary.Read(bytes.NewBuffer(keyBytes), p.byteOrder, &key); err != nil {
			p.metrics.stackDrop.WithLabelValues(labelStackDropReasonKey).Inc()
			return nil, fmt.Errorf("read stack count key: %w", err)
		}

		// Profile aggregation key.
		pKey := profileKey{pid: key.PID, tid: key.TID}

		// Twice the stack depth because we have a user and a potential Kernel stack.
		// Read order matters, since we read from the key buffer.
		stack := bpfprograms.CombinedStack{}
		interpreterStack := stack[bpfprograms.StackDepth*2:]

		var userErr error

		// User stacks which could have been unwound with the frame pointer or CFI unwinders.
		userStack := stack[:bpfprograms.StackDepth]
		userErr = p.bpfMaps.ReadStack(key.UserStackID, userStack)
		if userErr != nil {
			p.metrics.stackDrop.WithLabelValues(labelStackDropReasonUser).Inc()
			if errors.Is(userErr, ErrUnrecoverable) {
				p.metrics.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelError).Inc()
				return nil, userErr
			}
			if errors.Is(userErr, ErrUnwindFailed) {
				p.metrics.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelFailed).Inc()
			}
			if errors.Is(userErr, ErrMissing) {
				p.metrics.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelMissing).Inc()
			}
		} else {
			p.metrics.readMapAttempts.WithLabelValues(labelUser, labelNativeUnwind, labelSuccess).Inc()
		}

		if key.InterpreterStackID != 0 {
			if interpErr := p.bpfMaps.ReadStack(key.InterpreterStackID, interpreterStack); interpErr != nil {
				p.metrics.readMapAttempts.WithLabelValues(labelInterpreter, labelInterpreterUnwind, labelError).Inc()
				level.Debug(p.logger).Log("msg", "failed to read interpreter stacks", "err", interpErr)
			} else {
				p.metrics.readMapAttempts.WithLabelValues(labelInterpreter, labelInterpreterUnwind, labelSuccess).Inc()
			}
		}

		kStack := stack[bpfprograms.StackDepth : bpfprograms.StackDepth*2]
		kernelErr := p.bpfMaps.ReadStack(key.KernelStackID, kStack)
		if kernelErr != nil {
			p.metrics.stackDrop.WithLabelValues(labelStackDropReasonKernel).Inc()
			if errors.Is(kernelErr, ErrUnrecoverable) {
				p.metrics.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelError).Inc()
				return nil, kernelErr
			}
			if errors.Is(kernelErr, ErrUnwindFailed) {
				p.metrics.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelFailed).Inc()
			}
			if errors.Is(kernelErr, ErrMissing) {
				p.metrics.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelMissing).Inc()
			}
		} else {
			p.metrics.readMapAttempts.WithLabelValues(labelKernel, labelKernelUnwind, labelSuccess).Inc()
		}

		if userErr != nil && kernelErr != nil {
			// Both user stack (either via frame pointers or dwarf) and kernel stack
			// have failed. Nothing to do.
			continue
		}

		value, err := p.bpfMaps.ReadStackCount(keyBytes)
		if err != nil {
			p.metrics.stackDrop.WithLabelValues(labelStackDropReasonCount).Inc()
			return nil, fmt.Errorf("read value: %w", err)
		}
		if value == 0 {
			p.metrics.stackDrop.WithLabelValues(labelStackDropReasonZeroCount).Inc()
			// This should never happen, but it's here just in case.
			// If we have a zero value, we don't want to add it to the profile.
			continue
		}

		perThreadData, ok := rawData[pKey]
		if !ok {
			// We haven't seen this id yet.
			perThreadData = map[bpfprograms.CombinedStack]uint64{}
			rawData[pKey] = perThreadData
		}

		perThreadData[stack] += value
	}
	if it.Err() != nil {
		p.metrics.stackDrop.WithLabelValues(labelStackDropReasonIterator).Inc()
		return nil, fmt.Errorf("failed iterator: %w", it.Err())
	}

	if err := p.bpfMaps.FinalizeProfileLoop(); err != nil {
		level.Warn(p.logger).Log("msg", "failed to clean BPF maps that store stacktraces", "err", err)
	}

	return preprocessRawData(rawData), nil
}

func (p *CPU) Dump() DumpResponse {
	r := DumpRequest{
		Response: make(chan DumpResponse),
	}
	p.requests <- r
	return <-r.Response
}

// preprocessRawData takes the raw data from the BPF maps and converts it into
// a profile.RawData, which already splits the stacks into user, kernel and interpreter
// stacks. Since the input data is a map of maps, we can assume that they're
// already unique and there are no duplicates, which is why at this point we
// can just transform them into plain slices and structs.
func preprocessRawData(rawData map[profileKey]map[bpfprograms.CombinedStack]uint64) profile.RawData {
	res := make(profile.RawData, 0, len(rawData))
	for pKey, perThreadRawData := range rawData {
		p := profile.ProcessRawData{
			PID:        profile.PID(pKey.pid),
			RawSamples: make([]profile.RawSample, 0, len(perThreadRawData)),
		}

		for stack, count := range perThreadRawData {
			kernelStackDepth := 0
			userStackDepth := 0
			interpreterStackDepth := 0

			// We count the number of frames in the stack to be able to preallocate.
			// If an address in the stack is 0 then the stack ended.
			for _, addr := range stack[:bpfprograms.StackDepth] {
				if addr == 0 {
					break
				}
				userStackDepth++
			}
			for _, addr := range stack[bpfprograms.StackDepth : bpfprograms.StackDepth*2] {
				if addr == 0 {
					break
				}
				kernelStackDepth++
			}

			for _, addr := range stack[bpfprograms.StackDepth*2:] {
				if addr == 0 {
					break
				}
				interpreterStackDepth++
			}

			userStack := make([]uint64, userStackDepth)
			kernelStack := make([]uint64, kernelStackDepth)
			interpreterStack := make([]uint64, interpreterStackDepth)

			copy(userStack, stack[:userStackDepth])
			copy(kernelStack, stack[bpfprograms.StackDepth:bpfprograms.StackDepth+kernelStackDepth])
			copy(interpreterStack, stack[bpfprograms.StackDepth*2:bpfprograms.StackDepth*2+interpreterStackDepth])

			p.RawSamples = append(p.RawSamples, profile.RawSample{
				TID:              profile.PID(pKey.tid),
				UserStack:        userStack,
				KernelStack:      kernelStack,
				InterpreterStack: interpreterStack,
				Value:            count,
			})
		}

		res = append(res, p)
	}

	return res
}

func getArch() elf.Machine {
	switch goruntime.GOARCH {
	case "arm64":
		return elf.EM_AARCH64
	case "amd64":
		return elf.EM_X86_64
	default:
		return elf.EM_NONE
	}
}

type errorTracker struct {
	logger          log.Logger
	errorEncounters prometheus.Counter

	name string
	c    *cache.Cache[string, int]
}

func newErrorTracker(logger log.Logger, reg prometheus.Registerer, name string) *errorTracker {
	return &errorTracker{
		name:   name,
		logger: logger,
		errorEncounters: prometheus.NewCounter(prometheus.CounterOpts{
			Name:        "parca_agent_profiler_tracked_errors_total",
			Help:        "Counts errors encountered in the profiler",
			ConstLabels: map[string]string{"type": name},
		}),
		c: cache.NewLRUCache[string, int](
			prometheus.WrapRegistererWith(prometheus.Labels{"cache": name}, reg),
			512,
		),
	}
}

func (et *errorTracker) Track(pid int, err error) {
	et.errorEncounters.Inc()
	v, ok := et.c.Peek(err.Error())
	if ok {
		et.c.Add(err.Error(), v+1)
	} else {
		et.c.Add(err.Error(), 1)
	}
	v, _ = et.c.Get(err.Error())
	if v%50 == 0 || v == 1 {
		level.Error(et.logger).Log("msg", "failed to add unwind table due to unavailable .text section", "pid", pid, "err", err, "encounters", v)
	} else {
		level.Debug(et.logger).Log("msg", "failed to add unwind table due to unavailable .text section", "pid", pid, "err", err, "encounters", v)
	}
}
