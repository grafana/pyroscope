//go:build linux

package symtab

import (
	"fmt"
	"os"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

type PidKey uint32

// SymbolCache is responsible for resolving PC address to Symbol
// maintaining a pid -> ProcTable cache
// resolving kernel symbols
type SymbolCache struct {
	pidCache *GCache[PidKey, *ProcTable]

	elfCache *ElfCache
	kallsyms SymbolTable
	logger   log.Logger
	metrics  *Metrics
}
type CacheOptions struct {
	PidCacheOptions      GCacheOptions
	BuildIDCacheOptions  GCacheOptions
	SameFileCacheOptions GCacheOptions
	Metrics              *Metrics // may be nil for tests
}

func NewSymbolCache(logger log.Logger, options CacheOptions) (*SymbolCache, error) {
	elfCache, err := NewElfCache(options.BuildIDCacheOptions, options.SameFileCacheOptions)
	if err != nil {
		return nil, fmt.Errorf("create elf cache %w", err)
	}

	kallsymsData, err := os.ReadFile("/proc/kallsyms")
	if err != nil {
		return nil, fmt.Errorf("read kallsyms %w", err)
	}
	kallsyms, err := NewKallsyms(kallsymsData)
	if err != nil {
		return nil, fmt.Errorf("create kallsyms %w ", err)
	}
	if len(kallsyms.symbols) == 0 {
		level.Error(logger).Log("msg", "kallsyms is empty. check your permissions kptr_restrict==0 && sysctl_perf_event_paranoid <= 1 or kptr_restrict==1 &&  CAP_SYSLOG")
	}
	cache, err := NewGCache[PidKey, *ProcTable](options.PidCacheOptions)
	if err != nil {
		return nil, fmt.Errorf("create pid cache %w", err)
	}
	return &SymbolCache{
		logger:   logger,
		pidCache: cache,
		kallsyms: kallsyms,
		elfCache: elfCache,
		metrics:  options.Metrics,
	}, nil
}

func (sc *SymbolCache) NextRound() {
	sc.pidCache.NextRound()
	sc.elfCache.NextRound()
}

func (sc *SymbolCache) Resolve(pid uint32, addr uint64) Symbol {
	e := sc.getOrCreateCacheEntry(PidKey(pid))
	return e.Resolve(addr)
}

func (sc *SymbolCache) Cleanup() {
	sc.elfCache.Cleanup()
	sc.pidCache.Cleanup()
}

func (sc *SymbolCache) getOrCreateCacheEntry(pid PidKey) SymbolTable {
	if pid == 0 {
		return sc.kallsyms
	}
	cached := sc.pidCache.Get(pid)
	if cached != nil {
		return cached
	}

	level.Debug(sc.logger).Log("msg", "NewProcTable", "pid", pid)
	fresh := NewProcTable(sc.logger, ProcTableOptions{
		Pid: int(pid),
		ElfTableOptions: ElfTableOptions{
			ElfCache: sc.elfCache,
			Metrics:  sc.metrics,
		},
	})

	sc.pidCache.Cache(pid, fresh)
	return fresh
}

func (sc *SymbolCache) UpdateOptions(options CacheOptions) {
	sc.pidCache.Update(options.PidCacheOptions)
	sc.elfCache.Update(options.BuildIDCacheOptions, options.SameFileCacheOptions)
}

func (sc *SymbolCache) PidCacheDebugInfo() GCacheDebugInfo[ProcTableDebugInfo] {
	return DebugInfo[PidKey, *ProcTable, ProcTableDebugInfo](
		sc.pidCache,
		func(k PidKey, v *ProcTable) ProcTableDebugInfo {
			return v.DebugInfo()
		})
}

func (sc *SymbolCache) ElfCacheDebugInfo() ElfCacheDebugInfo {
	return sc.elfCache.DebugInfo()
}
