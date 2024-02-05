package symtab

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/pyroscope/ebpf/metrics"
)

type PidKey uint32

// SymbolCache is responsible for resolving PC address to Symbol
// maintaining a pid -> ProcTable cache
// resolving kernel symbols
type SymbolCache struct {
	pidCache *GCache[PidKey, *ProcTable]

	elfCache *ElfCache
	kallsyms *SymbolTab
	logger   log.Logger

	metrics *metrics.SymtabMetrics
}
type CacheOptions struct {
	PidCacheOptions      GCacheOptions
	BuildIDCacheOptions  GCacheOptions
	SameFileCacheOptions GCacheOptions
}

func NewSymbolCache(logger log.Logger, options CacheOptions, metrics *metrics.SymtabMetrics) (*SymbolCache, error) {
	if metrics == nil {
		panic("metrics is nil")
	}
	elfCache, err := NewElfCache(options.BuildIDCacheOptions, options.SameFileCacheOptions)
	if err != nil {
		return nil, fmt.Errorf("create elf cache %w", err)
	}

	cache, err := NewGCache[PidKey, *ProcTable](options.PidCacheOptions)
	if err != nil {
		return nil, fmt.Errorf("create pid cache %w", err)
	}
	return &SymbolCache{
		logger:   logger,
		pidCache: cache,
		kallsyms: nil,
		elfCache: elfCache,
		metrics:  metrics,
	}, nil
}

func (sc *SymbolCache) NextRound() {
	sc.pidCache.NextRound()
	sc.elfCache.NextRound()
}

func (sc *SymbolCache) Cleanup() {
	sc.elfCache.Cleanup()
	sc.pidCache.Cleanup()
}

func (sc *SymbolCache) GetProcTableCached(pid PidKey) *ProcTable {
	cached := sc.pidCache.Get(pid)
	if cached != nil {
		return cached
	}
	return nil
}

func (sc *SymbolCache) NewProcTable(pid PidKey, symbolOptions *SymbolOptions) *ProcTable {

	level.Debug(sc.logger).Log("msg", "NewProcTable", "pid", pid)
	fresh := NewProcTable(sc.logger, ProcTableOptions{
		Pid: int(pid),
		ElfTableOptions: ElfTableOptions{
			ElfCache:      sc.elfCache,
			Metrics:       sc.metrics,
			SymbolOptions: symbolOptions,
		},
	})

	sc.pidCache.Cache(pid, fresh)
	return fresh
}

func (sc *SymbolCache) GetKallsyms() SymbolTable {
	if sc.kallsyms != nil {
		return sc.kallsyms
	}
	return sc.initKallsyms()
}

func (sc *SymbolCache) initKallsyms() SymbolTable {
	var err error
	sc.kallsyms, err = NewKallsyms()
	if err != nil {
		level.Error(sc.logger).Log("msg", "kallsyms init fail", "err", err)
		sc.kallsyms = NewSymbolTab(nil)
	}
	if len(sc.kallsyms.symbols) == 0 {
		_ = level.Error(sc.logger).
			Log("msg", "kallsyms is empty. check your permissions kptr_restrict==0 && sysctl_perf_event_paranoid <= 1 or kptr_restrict==1 &&  CAP_SYSLOG")
	}

	return sc.kallsyms
}

func (sc *SymbolCache) UpdateOptions(options CacheOptions) {
	sc.pidCache.Update(options.PidCacheOptions)
	sc.elfCache.Update(options.BuildIDCacheOptions, options.SameFileCacheOptions)
}

func (sc *SymbolCache) PidCacheDebugInfo() GCacheDebugInfo[ProcTableDebugInfo] {
	return DebugInfo[PidKey, *ProcTable, ProcTableDebugInfo](
		sc.pidCache,
		func(k PidKey, v *ProcTable, round int) ProcTableDebugInfo {
			res := v.DebugInfo()
			res.LastUsedRound = round
			return res
		})
}

func (sc *SymbolCache) ElfCacheDebugInfo() ElfCacheDebugInfo {
	return sc.elfCache.DebugInfo()
}

func (sc *SymbolCache) RemoveDeadPID(pid PidKey) {
	sc.pidCache.Remove(pid)
}
