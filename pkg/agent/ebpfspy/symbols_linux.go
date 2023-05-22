//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//
//	https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import "C"
import (
	"fmt"
	"github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/symtab"
	"github.com/pyroscope-io/pyroscope/pkg/util/genericlru"
	"os"
	"sync"
)

type symbolCacheEntry struct {
	symbolTable symtab.SymbolTable
	roundNumber int
}
type pidKey uint32

type symbolCache struct {
	pid2Cache *genericlru.GenericLRU[pidKey, symbolCacheEntry]
	mutex     sync.Mutex
	kallsyms  symbolCacheEntry
}

func newSymbolCache(cacheSize int) (*symbolCache, error) {
	pid2Cache, err := genericlru.NewGenericLRU[pidKey, symbolCacheEntry](cacheSize, func(pid pidKey, e *symbolCacheEntry) {

	})
	if err != nil {
		return nil, fmt.Errorf("create pid symbol cache %w", err)
	}
	kallsymsData, err := os.ReadFile("/proc/kallsyms")
	if err != nil {
		return nil, fmt.Errorf("read kallsyms %w")
	}
	kallsyms, err := symtab.NewKallsyms(kallsymsData)
	if err != nil {
		return nil, fmt.Errorf("create kallsyms %w ", err)
	}
	return &symbolCache{
		pid2Cache: pid2Cache,
		kallsyms:  symbolCacheEntry{symbolTable: kallsyms},
	}, nil
}

func (sc *symbolCache) bccResolve(pid uint32, addr uint64, roundNumber int) *symtab.Symbol {
	e := sc.getOrCreateCacheEntry(pidKey(pid))
	staleCheck := false
	if roundNumber != e.roundNumber {
		e.roundNumber = roundNumber
		staleCheck = true
	}
	if staleCheck {
		e.symbolTable.Refresh()
	}
	return e.symbolTable.Resolve(addr)
}

func (sc *symbolCache) getOrCreateCacheEntry(pid pidKey) *symbolCacheEntry {
	if pid == 0 {
		return &sc.kallsyms
	}
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	if cache, ok := sc.pid2Cache.Get(pid); ok {
		return cache
	}

	symbolTable := symtab.NewProcTable(symtab.ProcTableOptions{Pid: int(pid)})
	e := &symbolCacheEntry{symbolTable: symbolTable}
	sc.pid2Cache.Add(pid, e)
	return e
}

func (sc *symbolCache) clear() {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	for _, pid := range sc.pid2Cache.Keys() {
		sc.pid2Cache.Remove(pid)
	}
}
