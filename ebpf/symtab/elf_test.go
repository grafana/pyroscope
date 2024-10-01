package symtab

import (
	elf2 "debug/elf"
	"testing"

	"github.com/grafana/pyroscope/ebpf/metrics"
	"github.com/grafana/pyroscope/ebpf/symtab/elf"
	"github.com/grafana/pyroscope/ebpf/util"
	"github.com/stretchr/testify/assert"

	"github.com/stretchr/testify/require"
)

func TestElf(t *testing.T) {
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	tab := NewElfTable(logger, &ProcMap{StartAddr: 0x1000, Offset: 0x1000}, ".", "elf/testdata/elfs/elf",
		ElfTableOptions{
			ElfCache: elfCache,
			Metrics:  metrics.NewSymtabMetrics(nil),
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
				Metrics:       metrics.NewSymtabMetrics(nil),
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
				Metrics:       metrics.NewSymtabMetrics(nil),
			})
		tab.load()
		require.NoError(t, tab.err)
		_, ok := tab.table.(*elf.GoTable)
		require.True(t, ok)
		elfCache.Cleanup()
	}

}

func TestFindBaseExec(t *testing.T) {
	et := ElfTable{}
	ef := elf.MMapedElfFile{}
	ef.FileHeader.Type = elf2.ET_EXEC
	assert.True(t, et.findBase(&ef))
	assert.Equal(t, uint64(0), et.base)
}

func TestFindBaseAlignedNoSeparateCode(t *testing.T) {
	//559f9d29f000-559f9d2ab000 r-xp 00000000 fc:01 53049243                   /FibProfilingTest
	//559f9d2ab000-559f9d2ac000 r--p 0000b000 fc:01 53049243                   /FibProfilingTest
	//559f9d2ac000-559f9d2ad000 rw-p 0000c000 fc:01 53049243                   /FibProfilingTest

	//Program Headers:
	//Type           Offset             VirtAddr           PhysAddr
	//               FileSiz            MemSiz              Flags  Align
	//LOAD           0x0000000000000000 0x0000000000000000 0x0000000000000000
	//               0x000000000000b1f4 0x000000000000b1f4  R E    0x1000
	// LOAD          0x000000000000bd90 0x000000000000cd90 0x000000000000cd90
	//               0x0000000000000370 0x00000000000003f8  RW     0x1000

	et := ElfTable{}
	et.procMap = &ProcMap{StartAddr: 0x559f9d29f000, EndAddr: 0x559f9d2ab000, Perms: &ProcMapPermissions{Execute: true, Read: true}, Offset: 0}

	ef := elf.MMapedElfFile{}
	ef.FileHeader.Type = elf2.ET_DYN
	ef.Progs = []elf2.ProgHeader{
		{Type: elf2.PT_LOAD, Flags: elf2.PF_X | elf2.PF_R, Off: 0, Vaddr: 0, Filesz: 0xb1f4, Memsz: 0xb1f4},
	}
	assert.True(t, et.findBase(&ef))
	assert.Equal(t, uint64(0x559f9d29f000), et.base)
}

func TestFindBaseUnalignedSeparateCode(t *testing.T) {
	//555e3d192000-555e3d1ac000 r--p 00000000 00:3e 1824988                    /smoketest
	//555e3d1ac000-555e3d212000 r-xp 00019000 00:3e 1824988                    /smoketest
	//555e3d212000-555e3d218000 r--p 0007e000 00:3e 1824988                    /smoketest
	//555e3d218000-555e3d219000 rw-p 00083000 00:3e 1824988                    /smoketest

	//Type           Offset             VirtAddr           PhysAddr
	//               FileSiz            MemSiz              Flags  Align
	//LOAD           0x0000000000000000 0x0000000000000000 0x0000000000000000
	//               0x0000000000019194 0x0000000000019194  R      0x1000
	//LOAD           0x00000000000191a0 0x000000000001a1a0 0x000000000001a1a0
	//               0x0000000000064ee0 0x0000000000064ee0  R E    0x1000
	//LOAD           0x000000000007e080 0x0000000000080080 0x0000000000080080
	//               0x0000000000005928 0x0000000000005928  RW     0x1000
	//LOAD           0x00000000000839b0 0x00000000000869b0 0x00000000000869b0
	//               0x0000000000000300 0x000000000020a764  RW     0x1000

	et := ElfTable{}
	et.procMap = &ProcMap{StartAddr: 0x555e3d1ac000, EndAddr: 0x555e3d212000, Perms: &ProcMapPermissions{Execute: true, Read: true}, Offset: 0x00019000}

	ef := elf.MMapedElfFile{}
	ef.FileHeader.Type = elf2.ET_DYN
	ef.Progs = []elf2.ProgHeader{
		{Type: elf2.PT_LOAD, Flags: elf2.PF_X | elf2.PF_R, Off: 0x191a0, Vaddr: 0x1a1a0, Filesz: 0x64ee0, Memsz: 0x64ee0},
	}
	assert.True(t, et.findBase(&ef))
	assert.Equal(t, uint64(0x555e3d192000), et.base)
}

func TestMiniDebugInfo(t *testing.T) {
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	tab := NewElfTable(logger, &ProcMap{StartAddr: 0x1000, Offset: 0x1000}, ".", "elf/testdata/elfs/elf.minidebuginfo",
		ElfTableOptions{
			ElfCache: elfCache,
			Metrics:  metrics.NewSymtabMetrics(nil),
		})

	syms := []struct {
		name string
		pc   uint64
	}{
		{"", 0x0},
		{"android_res_cancel", 0x1330}, // in .dynsym
		{"__on_dlclose", 0x1000},       // in .gnu_debugdata.symtab
	}
	for _, sym := range syms {
		res := tab.Resolve(sym.pc)
		require.Equal(t, res, sym.name)
	}
}
