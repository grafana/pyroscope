package symtab

import (
	"testing"

	"github.com/grafana/pyroscope/ebpf/symtab/elf"
	"github.com/grafana/pyroscope/ebpf/util"

	"github.com/stretchr/testify/require"
)

func TestElf(t *testing.T) {
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	tab := NewElfTable(logger, &ProcMap{StartAddr: 0x1000, Offset: 0x1000}, ".", "elf/testdata/elfs/elf",
		ElfTableOptions{
			ElfCache: elfCache,
		})

	syms := []struct {
		name string
		pc   uint64
	}{
		{"", 0x0},
		{"iter", 0x1149},
		{"main", 0x115e},
	}
	for _, sym := range syms {
		res := tab.Resolve(sym.pc)
		require.Equal(t, res, sym.name)
	}
}

func TestGoTableFallbackFiltering(t *testing.T) {
	ts := []struct {
		f string
	}{
		{"elf/testdata/elfs/go12"},
		{"elf/testdata/elfs/go16"},
		{"elf/testdata/elfs/go18"},
		{"elf/testdata/elfs/go20"},
		{"elf/testdata/elfs/go12-static"},
		{"elf/testdata/elfs/go16-static"},
		{"elf/testdata/elfs/go18-static"},
		{"elf/testdata/elfs/go20-static"},
	}
	for _, e := range ts {
		elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
		logger := util.TestLogger(t)
		tab := NewElfTable(logger, &ProcMap{StartAddr: 0x1000, Offset: 0x1000}, ".", e.f,
			ElfTableOptions{
				ElfCache:      elfCache,
				SymbolOptions: &SymbolOptions{GoTableFallback: true},
			})
		tab.load()
		require.NoError(t, tab.err)
		gt, ok := tab.table.(*elf.GoTableWithFallback)
		require.True(t, ok)
		_ = gt
		for i := 0; i < gt.GoTable.Index.Entry.Length(); i++ {
			pc := gt.GoTable.Index.Entry.Get(i)
			s1 := gt.GoTable.Resolve(pc)
			s2 := gt.SymTable.Resolve(pc)
			require.NotEqual(t, "", s1)
			require.Equal(t, "", s2)
		}
		elfCache.Cleanup()
	}

}

func TestGoTableFallbackDisabled(t *testing.T) {
	ts := []struct {
		f string
	}{
		{"elf/testdata/elfs/go12"},
		{"elf/testdata/elfs/go16"},
		{"elf/testdata/elfs/go18"},
		{"elf/testdata/elfs/go20"},
		{"elf/testdata/elfs/go12-static"},
		{"elf/testdata/elfs/go16-static"},
		{"elf/testdata/elfs/go18-static"},
		{"elf/testdata/elfs/go20-static"},
	}
	for _, e := range ts {
		elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
		logger := util.TestLogger(t)
		tab := NewElfTable(logger, &ProcMap{StartAddr: 0x1000, Offset: 0x1000}, ".", e.f,
			ElfTableOptions{
				ElfCache:      elfCache,
				SymbolOptions: &SymbolOptions{GoTableFallback: false},
			})
		tab.load()
		require.NoError(t, tab.err)
		_, ok := tab.table.(*elf.GoTable)
		require.True(t, ok)
		elfCache.Cleanup()
	}

}
