package symtab

import (
	"crypto/md5"
	"encoding/hex"
	"github.com/grafana/phlare/ebpf/util"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTestdataMD5(t *testing.T) {
	elfs := []struct {
		md5Sum, file string
	}{
		{"5201d962e9f71ea220b9610d6352b57e", "elf"},
		{"e300c975548c3d7617a263243592ae46", "elf.debug"},
		{"668e90be1ac8a0e8e89ebd47284bf7fc", "elf.debuglink"},
		{"7ecf01cd4fe52e4a31d7840e8d93ac56", "elf.nobuildid"},
		{"4284c6ba06fedfe6e05627ddd5ccff18", "elf.nopie"},
		{"635fd79c77b9de925647fe566668ea6d", "elf.stripped"},
		{"b69d2a627f90ecac7868effa89a37c33", "libexample.so"},
	}
	for _, elf := range elfs {
		data, err := os.ReadFile(path.Join("elf", "testdata", "elfs", elf.file))
		if err != nil {
			t.Errorf("failed to check md5 %v %v", elf, err)
			continue
		}
		hash := md5.New()
		hash.Write(data)
		md5Sum := hex.EncodeToString(hash.Sum(nil))
		if md5Sum != elf.md5Sum {
			t.Errorf("failed to check md5 %v %v", elf, md5Sum)
		}
	}
}

type procTestdata struct {
	name   string
	elf    string
	offset uint64
	base   uint64
}

func testProc(t *testing.T, maps string, data []procTestdata) {
	wd, _ := os.Getwd()
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	m := NewProcTable(logger, ProcTableOptions{
		Pid: 239,
		ElfTableOptions: ElfTableOptions{
			ElfCache: elfCache,
		},
	})
	m.rootFS = path.Join(wd, "elf", "testdata")
	m.refresh([]byte(maps))
	for _, td := range data {
		sym := m.Resolve(td.base + td.offset)
		require.Equal(t, sym.Name, td.name)
		require.Contains(t, sym.Module, td.elf)
	}
}

func TestProc(t *testing.T) {
	maps := `56483a0ee000-56483a0ef000 r--p 00000000 09:00 9469561                    /elfs/elf
56483a0ef000-56483a0f0000 r-xp 00001000 09:00 9469561                    /elfs/elf
56483a0f0000-56483a0f1000 r--p 00002000 09:00 9469561                    /elfs/elf
56483a0f1000-56483a0f2000 r--p 00002000 09:00 9469561                    /elfs/elf
56483a0f2000-56483a0f3000 rw-p 00003000 09:00 9469561                    /elfs/elf
7fa9f6fda000-7fa9f6fdd000 rw-p 00000000 00:00 0 
7fa9f6fdd000-7fa9f7005000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fa9f7005000-7fa9f719a000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fa9f719a000-7fa9f71f2000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fa9f71f2000-7fa9f71f6000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fa9f71f6000-7fa9f71f8000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fa9f71f8000-7fa9f7205000 rw-p 00000000 00:00 0 
7fa9f720e000-7fa9f720f000 r--p 00000000 09:00 9543485                    /elfs/libexample.so
7fa9f720f000-7fa9f7210000 r-xp 00001000 09:00 9543485                    /elfs/libexample.so
7fa9f7210000-7fa9f7211000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fa9f7211000-7fa9f7212000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fa9f7212000-7fa9f7213000 rw-p 00003000 09:00 9543485                    /elfs/libexample.so
7fa9f7213000-7fa9f7215000 rw-p 00000000 00:00 0 
7fa9f7215000-7fa9f7217000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fa9f7217000-7fa9f7241000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fa9f7241000-7fa9f724c000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fa9f724d000-7fa9f724f000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fa9f724f000-7fa9f7251000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffe08f0d000-7ffe08f2e000 rw-p 00000000 00:00 0                          [stack]
7ffe08f52000-7ffe08f56000 r--p 00000000 00:00 0                          [vvar]
7ffe08f56000-7ffe08f58000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf", 0x1149, 0x56483a0ee000},
		{"main", "elf", 0x115e, 0x56483a0ee000},
		{"lib_iter", "libexample.so", 0x1139, 0x7fa9f720e000},
	}
	testProc(t, maps, syms)
}

func TestProcNoPie(t *testing.T) {
	maps := `00400000-00401000 r--p 00000000 09:00 9543481                            /elfs/elf.nopie
00401000-00402000 r-xp 00001000 09:00 9543481                            /elfs/elf.nopie
00402000-00403000 r--p 00002000 09:00 9543481                            /elfs/elf.nopie
00403000-00404000 r--p 00002000 09:00 9543481                            /elfs/elf.nopie
00404000-00405000 rw-p 00003000 09:00 9543481                            /elfs/elf.nopie
7f8f62c82000-7f8f62c85000 rw-p 00000000 00:00 0 
7f8f62c85000-7f8f62cad000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f8f62cad000-7f8f62e42000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f8f62e42000-7f8f62e9a000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f8f62e9a000-7f8f62e9e000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f8f62e9e000-7f8f62ea0000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f8f62ea0000-7f8f62ead000 rw-p 00000000 00:00 0 
7f8f62eb6000-7f8f62eb7000 r--p 00000000 09:00 9543485                    /elfs/libexample.so
7f8f62eb7000-7f8f62eb8000 r-xp 00001000 09:00 9543485                    /elfs/libexample.so
7f8f62eb8000-7f8f62eb9000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7f8f62eb9000-7f8f62eba000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7f8f62eba000-7f8f62ebb000 rw-p 00003000 09:00 9543485                    /elfs/libexample.so
7f8f62ebb000-7f8f62ebd000 rw-p 00000000 00:00 0 
7f8f62ebd000-7f8f62ebf000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f8f62ebf000-7f8f62ee9000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f8f62ee9000-7f8f62ef4000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f8f62ef5000-7f8f62ef7000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f8f62ef7000-7f8f62ef9000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffe100a2000-7ffe100c3000 rw-p 00000000 00:00 0                          [stack]
7ffe1013d000-7ffe10141000 r--p 00000000 00:00 0                          [vvar]
7ffe10141000-7ffe10143000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf.nopie", 0x401136, 0},
		{"main", "elf.nopie", 0x40114b, 0},
		{"lib_iter", "libexample.so", 0x1139, 0x7f8f62eb6000},
	}
	testProc(t, maps, syms)
}

func TestDebugFileBuildID(t *testing.T) {
	maps := `556bf5712000-556bf5713000 r--p 00000000 09:00 9469523                    /elfs/elf.stripped
556bf5713000-556bf5714000 r-xp 00001000 09:00 9469523                    /elfs/elf.stripped
556bf5714000-556bf5715000 r--p 00002000 09:00 9469523                    /elfs/elf.stripped
556bf5715000-556bf5716000 r--p 00002000 09:00 9469523                    /elfs/elf.stripped
556bf5716000-556bf5717000 rw-p 00003000 09:00 9469523                    /elfs/elf.stripped
7fb225802000-7fb225805000 rw-p 00000000 00:00 0 
7fb225805000-7fb22582d000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fb22582d000-7fb2259c2000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fb2259c2000-7fb225a1a000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fb225a1a000-7fb225a1e000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fb225a1e000-7fb225a20000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fb225a20000-7fb225a2d000 rw-p 00000000 00:00 0 
7fb225a36000-7fb225a37000 r--p 00000000 09:00 9543485                    /elfs/libexample.so
7fb225a37000-7fb225a38000 r-xp 00001000 09:00 9543485                    /elfs/libexample.so
7fb225a38000-7fb225a39000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fb225a39000-7fb225a3a000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fb225a3a000-7fb225a3b000 rw-p 00003000 09:00 9543485                    /elfs/libexample.so
7fb225a3b000-7fb225a3d000 rw-p 00000000 00:00 0 
7fb225a3d000-7fb225a3f000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fb225a3f000-7fb225a69000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fb225a69000-7fb225a74000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fb225a75000-7fb225a77000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fb225a77000-7fb225a79000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffd99230000-7ffd99251000 rw-p 00000000 00:00 0                          [stack]
7ffd99336000-7ffd9933a000 r--p 00000000 00:00 0                          [vvar]
7ffd9933a000-7ffd9933c000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf.stripped", 0x1149, 0x556bf5712000},
		{"main", "elf.stripped", 0x115e, 0x556bf5712000},
		{"lib_iter", "libexample.so", 0x1139, 0x7fb225a36000},
	}
	testProc(t, maps, syms)
}

func TestDebugFileDebugLink(t *testing.T) {
	//nolint:goconst
	maps := `559090826000-559090827000 r--p 00000000 09:00 9543482                    /elfs/elf.debuglink
559090827000-559090828000 r-xp 00001000 09:00 9543482                    /elfs/elf.debuglink
559090828000-559090829000 r--p 00002000 09:00 9543482                    /elfs/elf.debuglink
559090829000-55909082b000 rw-p 00002000 09:00 9543482                    /elfs/elf.debuglink
7fd1c6f27000-7fd1c6f2a000 rw-p 00000000 00:00 0 
7fd1c6f2a000-7fd1c6f52000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c6f52000-7fd1c70e7000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c70e7000-7fd1c713f000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c713f000-7fd1c7143000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7143000-7fd1c7145000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7145000-7fd1c7152000 rw-p 00000000 00:00 0 
7fd1c715b000-7fd1c715c000 r--p 00000000 09:00 9543485                    /elfs/libexample.so
7fd1c715c000-7fd1c715d000 r-xp 00001000 09:00 9543485                    /elfs/libexample.so
7fd1c715d000-7fd1c715e000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fd1c715e000-7fd1c715f000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fd1c715f000-7fd1c7160000 rw-p 00003000 09:00 9543485                    /elfs/libexample.so
7fd1c7160000-7fd1c7162000 rw-p 00000000 00:00 0 
7fd1c7162000-7fd1c7164000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c7164000-7fd1c718e000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c718e000-7fd1c7199000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719a000-7fd1c719c000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719c000-7fd1c719e000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fffd9b57000-7fffd9b78000 rw-p 00000000 00:00 0                          [stack]
7fffd9bd2000-7fffd9bd6000 r--p 00000000 00:00 0                          [vvar]
7fffd9bd6000-7fffd9bd8000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf.debuglink", 0x1149, 0x559090826000},
		{"main", "elf.debuglink", 0x115e, 0x559090826000},
		{"lib_iter", "libexample.so", 0x1139, 0x7fd1c715b000},
	}
	testProc(t, maps, syms)
}

func TestUnload(t *testing.T) {
	//nolint:goconst
	maps := `559090826000-559090827000 r--p 00000000 09:00 9543482                    /elfs/elf.debuglink
559090827000-559090828000 r-xp 00001000 09:00 9543482                    /elfs/elf.debuglink
559090828000-559090829000 r--p 00002000 09:00 9543482                    /elfs/elf.debuglink
559090829000-55909082b000 rw-p 00002000 09:00 9543482                    /elfs/elf.debuglink
7fd1c6f27000-7fd1c6f2a000 rw-p 00000000 00:00 0 
7fd1c6f2a000-7fd1c6f52000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c6f52000-7fd1c70e7000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c70e7000-7fd1c713f000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c713f000-7fd1c7143000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7143000-7fd1c7145000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7145000-7fd1c7152000 rw-p 00000000 00:00 0 
7fd1c715b000-7fd1c715c000 r--p 00000000 09:00 9543485                    /elfs/libexample.so
7fd1c715c000-7fd1c715d000 r-xp 00001000 09:00 9543485                    /elfs/libexample.so
7fd1c715d000-7fd1c715e000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fd1c715e000-7fd1c715f000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fd1c715f000-7fd1c7160000 rw-p 00003000 09:00 9543485                    /elfs/libexample.so
7fd1c7160000-7fd1c7162000 rw-p 00000000 00:00 0 
7fd1c7162000-7fd1c7164000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c7164000-7fd1c718e000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c718e000-7fd1c7199000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719a000-7fd1c719c000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719c000-7fd1c719e000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fffd9b57000-7fffd9b78000 rw-p 00000000 00:00 0                          [stack]
7fffd9bd2000-7fffd9bd6000 r--p 00000000 00:00 0                          [vvar]
7fffd9bd6000-7fffd9bd8000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	iterSym := procTestdata{"lib_iter", "libexample.so", 0x1139, 0x7fd1c715b000}
	var syms = []procTestdata{
		{"iter", "elf.debuglink", 0x1149, 0x559090826000},
		{"main", "elf.debuglink", 0x115e, 0x559090826000},
		iterSym,
	}

	wd, _ := os.Getwd()
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	m := NewProcTable(logger, ProcTableOptions{
		Pid: 239,
		ElfTableOptions: ElfTableOptions{
			ElfCache: elfCache,
		},
	})
	m.rootFS = path.Join(wd, "elf", "testdata")
	m.refresh([]byte(maps))
	for _, td := range syms {
		sym := m.Resolve(td.base + td.offset)
		if sym.Name != td.name || !strings.Contains(sym.Module, td.elf) {
			t.Errorf("failed to Resolve %v (%v)", td, sym)
		}
	}
	maps = `559090826000-559090827000 r--p 00000000 09:00 9543482                    /elfs/elf.debuglink
559090827000-559090828000 r-xp 00001000 09:00 9543482                    /elfs/elf.debuglink
559090828000-559090829000 r--p 00002000 09:00 9543482                    /elfs/elf.debuglink
559090829000-55909082b000 rw-p 00002000 09:00 9543482                    /elfs/elf.debuglink
7fd1c6f27000-7fd1c6f2a000 rw-p 00000000 00:00 0 
7fd1c6f2a000-7fd1c6f52000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c6f52000-7fd1c70e7000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c70e7000-7fd1c713f000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c713f000-7fd1c7143000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7143000-7fd1c7145000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7145000-7fd1c7152000 rw-p 00000000 00:00 0
7fd1c7162000-7fd1c7164000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c7164000-7fd1c718e000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c718e000-7fd1c7199000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719a000-7fd1c719c000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719c000-7fd1c719e000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fffd9b57000-7fffd9b78000 rw-p 00000000 00:00 0                          [stack]
7fffd9bd2000-7fffd9bd6000 r--p 00000000 00:00 0                          [vvar]
7fffd9bd6000-7fffd9bd8000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	require.Equal(t, 4, len(m.file2Table))
	m.refresh([]byte(maps))
	require.Equal(t, 3, len(m.file2Table))
	sym := m.Resolve(iterSym.base + iterSym.offset)
	require.Empty(t, sym.Name)
	require.Empty(t, sym.Module)
	require.Empty(t, sym.Start)
}

func TestInodeChange(t *testing.T) {
	//nolint:goconst
	maps := `559090826000-559090827000 r--p 00000000 09:00 9543482                    /elfs/elf.debuglink
559090827000-559090828000 r-xp 00001000 09:00 9543482                    /elfs/elf.debuglink
559090828000-559090829000 r--p 00002000 09:00 9543482                    /elfs/elf.debuglink
559090829000-55909082b000 rw-p 00002000 09:00 9543482                    /elfs/elf.debuglink
7fd1c6f27000-7fd1c6f2a000 rw-p 00000000 00:00 0 
7fd1c6f2a000-7fd1c6f52000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c6f52000-7fd1c70e7000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c70e7000-7fd1c713f000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c713f000-7fd1c7143000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7143000-7fd1c7145000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7145000-7fd1c7152000 rw-p 00000000 00:00 0 
7fd1c715b000-7fd1c715c000 r--p 00000000 09:00 9543485                    /elfs/libexample.so
7fd1c715c000-7fd1c715d000 r-xp 00001000 09:00 9543485                    /elfs/libexample.so
7fd1c715d000-7fd1c715e000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fd1c715e000-7fd1c715f000 r--p 00002000 09:00 9543485                    /elfs/libexample.so
7fd1c715f000-7fd1c7160000 rw-p 00003000 09:00 9543485                    /elfs/libexample.so
7fd1c7160000-7fd1c7162000 rw-p 00000000 00:00 0 
7fd1c7162000-7fd1c7164000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c7164000-7fd1c718e000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c718e000-7fd1c7199000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719a000-7fd1c719c000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719c000-7fd1c719e000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fffd9b57000-7fffd9b78000 rw-p 00000000 00:00 0                          [stack]
7fffd9bd2000-7fffd9bd6000 r--p 00000000 00:00 0                          [vvar]
7fffd9bd6000-7fffd9bd8000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	iterSym := procTestdata{"lib_iter", "libexample.so", 0x1139, 0x7fd1c715b000}
	var syms = []procTestdata{
		{"iter", "elf.debuglink", 0x1149, 0x559090826000},
		{"main", "elf.debuglink", 0x115e, 0x559090826000},
		iterSym,
	}

	wd, _ := os.Getwd()
	elfCache, _ := NewElfCache(testCacheOptions, testCacheOptions)
	logger := util.TestLogger(t)
	m := NewProcTable(logger, ProcTableOptions{
		Pid: 239,
		ElfTableOptions: ElfTableOptions{
			ElfCache: elfCache,
		},
	})
	m.rootFS = path.Join(wd, "elf", "testdata")
	m.refresh([]byte(maps))
	for _, td := range syms {
		sym := m.Resolve(td.base + td.offset)
		if sym.Name != td.name || !strings.Contains(sym.Module, td.elf) {
			t.Errorf("failed to Resolve %v (%v)", td, sym)
		}
	}
	maps = `559090826000-559090827000 r--p 00000000 09:00 9543482                    /elfs/elf.debuglink
559090827000-559090828000 r-xp 00001000 09:00 9543482                    /elfs/elf.debuglink
559090828000-559090829000 r--p 00002000 09:00 9543482                    /elfs/elf.debuglink
559090829000-55909082b000 rw-p 00002000 09:00 9543482                    /elfs/elf.debuglink
7fd1c6f27000-7fd1c6f2a000 rw-p 00000000 00:00 0 
7fd1c6f2a000-7fd1c6f52000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c6f52000-7fd1c70e7000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c70e7000-7fd1c713f000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c713f000-7fd1c7143000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7143000-7fd1c7145000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7fd1c7145000-7fd1c7152000 rw-p 00000000 00:00 0 
7fd1c715b000-7fd1c715c000 r--p 00000000 09:00 9543486                    /elfs/libexample.so
7fd1c715c000-7fd1c715d000 r-xp 00001000 09:00 9543486                    /elfs/libexample.so
7fd1c715d000-7fd1c715e000 r--p 00002000 09:00 9543486                    /elfs/libexample.so
7fd1c715e000-7fd1c715f000 r--p 00002000 09:00 9543486                    /elfs/libexample.so
7fd1c715f000-7fd1c7160000 rw-p 00003000 09:00 9543486                    /elfs/libexample.so
7fd1c7160000-7fd1c7162000 rw-p 00000000 00:00 0 
7fd1c7162000-7fd1c7164000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c7164000-7fd1c718e000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c718e000-7fd1c7199000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719a000-7fd1c719c000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fd1c719c000-7fd1c719e000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7fffd9b57000-7fffd9b78000 rw-p 00000000 00:00 0                          [stack]
7fffd9bd2000-7fffd9bd6000 r--p 00000000 00:00 0                          [vvar]
7fffd9bd6000-7fffd9bd8000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	require.Equal(t, 4, len(m.file2Table))
	m.refresh([]byte(maps))
	require.Equal(t, 4, len(m.file2Table))
	sym := m.Resolve(iterSym.base + iterSym.offset)
	require.NotEmpty(t, sym.Name)
	require.NotEmpty(t, sym.Module)
	require.NotEmpty(t, sym.Start)
}
