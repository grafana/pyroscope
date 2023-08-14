package symtab

import (
	"github.com/grafana/pyroscope/ebpf/symtab/elf"
)

type ElfCache struct {
	BuildIDCache  *GCache[elf.BuildID, SymbolNameResolver]
	SameFileCache *GCache[Stat, SymbolNameResolver]
}

func NewElfCache(buildIDCacheOptions GCacheOptions, sameFileCacheOptions GCacheOptions) (*ElfCache, error) {
	buildIdCache, err := NewGCache[elf.BuildID, SymbolNameResolver](buildIDCacheOptions)
	if err != nil {
		return nil, err
	}

	statCache, err := NewGCache[Stat, SymbolNameResolver](sameFileCacheOptions)
	if err != nil {
		return nil, err
	}
	return &ElfCache{
		BuildIDCache:  buildIdCache,
		SameFileCache: statCache}, nil
}

func (e *ElfCache) GetSymbolsByBuildID(buildID elf.BuildID) SymbolNameResolver {
	res := e.BuildIDCache.Get(buildID)
	if res == nil {
		return nil
	}
	if res.IsDead() {
		e.BuildIDCache.Remove(buildID)
		return nil
	}
	return res
}

func (e *ElfCache) CacheByBuildID(buildID elf.BuildID, v SymbolNameResolver) {
	if v == nil {
		return
	}
	e.BuildIDCache.Cache(buildID, v)
}

func (e *ElfCache) GetSymbolsByStat(s Stat) SymbolNameResolver {
	res := e.SameFileCache.Get(s)
	if res == nil {
		return nil
	}
	if res.IsDead() {
		e.SameFileCache.Remove(s)
		return nil
	}
	return res
}

func (e *ElfCache) CacheByStat(s Stat, v SymbolNameResolver) {
	if v == nil {
		return
	}
	e.SameFileCache.Cache(s, v)
}

func (e *ElfCache) Update(buildIDCacheOptions GCacheOptions, sameFileCacheOptions GCacheOptions) {
	e.BuildIDCache.Update(buildIDCacheOptions)
	e.SameFileCache.Update(sameFileCacheOptions)
}

func (e *ElfCache) NextRound() {
	e.BuildIDCache.NextRound()
	e.SameFileCache.NextRound()
}

func (e *ElfCache) Cleanup() {
	e.BuildIDCache.Cleanup()
	e.SameFileCache.Cleanup()
}

type ElfCacheDebugInfo struct {
	BuildIDCache  GCacheDebugInfo[elf.SymTabDebugInfo] `river:"build_id_cache,attr,optional"`
	SameFileCache GCacheDebugInfo[elf.SymTabDebugInfo] `river:"same_file_cache,attr,optional"`
}

func (e *ElfCache) DebugInfo() ElfCacheDebugInfo {
	return ElfCacheDebugInfo{
		BuildIDCache: DebugInfo[elf.BuildID, SymbolNameResolver, elf.SymTabDebugInfo](
			e.BuildIDCache,
			func(b elf.BuildID, v SymbolNameResolver, round int) elf.SymTabDebugInfo {
				res := v.DebugInfo()
				res.LastUsedRound = round
				return res
			}),
		SameFileCache: DebugInfo[Stat, SymbolNameResolver, elf.SymTabDebugInfo](
			e.SameFileCache,
			func(s Stat, v SymbolNameResolver, round int) elf.SymTabDebugInfo {
				res := v.DebugInfo()
				res.LastUsedRound = round
				return res
			}),
	}
}
