//go:build linux

package symtab

import (
	elf0 "debug/elf"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/symtab/elf"
	"github.com/grafana/pyroscope/ebpf/util"
	"github.com/stretchr/testify/require"
)

func TestMallocResolve(t *testing.T) {
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	gosym := NewProcTable(logger, ProcTableOptions{
		Pid: os.Getpid(),
		ElfTableOptions: ElfTableOptions{
			ElfCache: elfCache,
			Metrics:  metrics.NewSymtabMetrics(nil),
		},
	})
	gosym.Refresh()
	malloc := testHelperGetMalloc()
	res := gosym.Resolve(uint64(malloc))
	require.Contains(t, res.Name, "malloc")
	if !strings.Contains(res.Module, "/libc.so") && !strings.Contains(res.Module, "/libc-") {
		t.Errorf("expected libc, got %v", res.Module)
	}
}

func BenchmarkProc(b *testing.B) {
	gosym, _ := elf.GetGoSymbols("/proc/self/exe", false)
	logger := log.NewSyncLogger(log.NewLogfmtLogger(os.Stderr))
	proc := NewProcTable(logger, ProcTableOptions{Pid: os.Getpid()})
	proc.Refresh()
	if len(gosym) < 1000 {
		b.FailNow()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, symbol := range gosym {
			proc.Resolve(symbol.Start)
		}
	}
}

func TestSelfElfSymbolsLazy(t *testing.T) {
	f, err := os.Readlink("/proc/self/exe")
	require.NoError(t, err)

	e, err := elf0.Open(f)
	require.NoError(t, err)
	defer e.Close()

	expectedSymbols := elf.GetELFSymbolsFromSymtab(e)

	if len(expectedSymbols) == 0 {
		t.Skip("no symbols found")
		return
	}

	me, err := elf.NewMMapedElfFile(f)
	require.NoError(t, err)
	defer me.Close()

	symbolTable, err := me.NewSymbolTable(new(elf.SymbolsOptions))
	require.NoError(t, err)

	for _, symbol := range expectedSymbols {
		name := symbolTable.Resolve(symbol.Start)
		if symbol.Name == "runtime.text" && name == "internal/cpu.Initialize" {
			continue
		}
		var found []elf.TestSym
		var nameMatches int
		// there may be multiple symbols with the same start address
		// in no particular order so just require to have at least one
		for j := range expectedSymbols {
			if expectedSymbols[j].Start == symbol.Start {
				found = append(found, expectedSymbols[j])
				if expectedSymbols[j].Name == name {
					nameMatches++
				}
			}
		}
		require.GreaterOrEqualf(t, nameMatches, 1, "symbol %v at %x (found %v)", symbol.Name, symbol.Start, found)
	}
}
