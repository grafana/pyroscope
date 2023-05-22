package symtab

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"
)

func TestTestdataMD5(t *testing.T) {
	elfs := []struct {
		md5, file string
	}{
		{"bd5290f82511ce60aad5d2a75e668130", "elf"},
		{"6dc6f1b3693aecf1a49b5cd2371c6d8d", "elf.debug"},
		{"66889a4baed3181540cca8cb2b450951", "elf.debuglink"},
		{"4015c4730be255e170225b0f28845f7b", "elf.nopie"},
		{"ddbff547e12f3a530f2f7cb38de710b5", "elf.stripped"},
	}
	for _, elf := range elfs {
		//todo check md5

		data, err := os.ReadFile(path.Join("testdata", "elfs", elf.file))
		if err != nil {
			t.Errorf("failed to check md5 %v %v", elf, err)
			continue
		}
		hash := md5.New()
		hash.Write(data)
		md5 := hex.EncodeToString(hash.Sum(nil))
		if md5 != elf.md5 {
			t.Errorf("failed to check md5 %v %v", elf, md5)
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
	m := NewProcTable(ProcTableOptions{Pid: 239})
	m.rootFS = path.Join(wd, "testdata")
	m.refresh([]byte(maps))
	for _, td := range data {
		sym := m.Resolve(td.base + td.offset)
		if sym == nil || sym.Name != td.name || !strings.ContainsAny(sym.Module, td.elf) {
			t.Errorf("failed to resolve %v (%v)", td, sym)
		}
	}

}
func TestProc(t *testing.T) {
	maps := `558d93249000-558d9324a000 r--p 00000000 00:2b 14178666                   /elfs/elf
558d9324a000-558d9324b000 r-xp 00001000 00:2b 14178666                   /elfs/elf
558d9324b000-558d9324c000 r--p 00002000 00:2b 14178666                   /elfs/elf
558d9324c000-558d9324d000 r--p 00002000 00:2b 14178666                   /elfs/elf
558d9324d000-558d9324e000 rw-p 00003000 00:2b 14178666                   /elfs/elf
7f5e5a7c1000-7f5e5a7c4000 rw-p 00000000 00:00 0
7f5e5a7c4000-7f5e5a7ec000 r--p 00000000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f5e5a7ec000-7f5e5a981000 r-xp 00028000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f5e5a981000-7f5e5a9d9000 r--p 001bd000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f5e5a9d9000-7f5e5a9dd000 r--p 00214000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f5e5a9dd000-7f5e5a9df000 rw-p 00218000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f5e5a9df000-7f5e5a9ec000 rw-p 00000000 00:00 0
7f5e5a9ef000-7f5e5a9f1000 rw-p 00000000 00:00 0
7f5e5a9f1000-7f5e5a9f3000 r--p 00000000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5e5a9f3000-7f5e5aa1d000 r-xp 00002000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5e5aa1d000-7f5e5aa28000 r--p 0002c000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5e5aa29000-7f5e5aa2b000 r--p 00037000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5e5aa2b000-7f5e5aa2d000 rw-p 00039000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffdd7ae5000-7ffdd7b06000 rw-p 00000000 00:00 0                          [stack]
7ffdd7b5b000-7ffdd7b5f000 r--p 00000000 00:00 0                          [vvar]
7ffdd7b5f000-7ffdd7b61000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf", 4457, 0x558d93249000},
		{"main", "elf", 4505, 0x558d93249000},
		{"open64", "libc.so.6", 1132176, 0x7f5e5a7c4000},
		{"close", "libc.so.6", 1134848, 0x7f5e5a7c4000},
	}
	testProc(t, maps, syms)
}

func TestProcNoPie(t *testing.T) {
	maps := `00400000-00401000 r--p 00000000 00:2b 14178675                           /elfs/elf.nopie
00401000-00402000 r-xp 00001000 00:2b 14178675                           /elfs/elf.nopie
00402000-00403000 r--p 00002000 00:2b 14178675                           /elfs/elf.nopie
00403000-00404000 r--p 00002000 00:2b 14178675                           /elfs/elf.nopie
00404000-00405000 rw-p 00003000 00:2b 14178675                           /elfs/elf.nopie
7faf71e67000-7faf71e6a000 rw-p 00000000 00:00 0
7faf71e6a000-7faf71e92000 r--p 00000000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7faf71e92000-7faf72027000 r-xp 00028000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7faf72027000-7faf7207f000 r--p 001bd000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7faf7207f000-7faf72083000 r--p 00214000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7faf72083000-7faf72085000 rw-p 00218000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7faf72085000-7faf72092000 rw-p 00000000 00:00 0
7faf72095000-7faf72097000 rw-p 00000000 00:00 0
7faf72097000-7faf72099000 r--p 00000000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7faf72099000-7faf720c3000 r-xp 00002000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7faf720c3000-7faf720ce000 r--p 0002c000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7faf720cf000-7faf720d1000 r--p 00037000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7faf720d1000-7faf720d3000 rw-p 00039000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffcdd495000-7ffcdd4b6000 rw-p 00000000 00:00 0                          [stack]
7ffcdd596000-7ffcdd59a000 r--p 00000000 00:00 0                          [vvar]
7ffcdd59a000-7ffcdd59c000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf.nopie", 0x401156, 0},
		{"main", "elf.nopie", 0x401186, 0},
		{"open64", "libc.so.6", 1132176, 0x7faf71e6a000},
		{"close", "libc.so.6", 1134848, 0x7faf71e6a000},
	}
	testProc(t, maps, syms)
}

func TestDebugFileBuildID(t *testing.T) {
	maps := `55aa0e908000-55aa0e909000 r--p 00000000 00:2b 14178678                   /elfs/elf.stripped
55aa0e909000-55aa0e90a000 r-xp 00001000 00:2b 14178678                   /elfs/elf.stripped
55aa0e90a000-55aa0e90b000 r--p 00002000 00:2b 14178678                   /elfs/elf.stripped
55aa0e90b000-55aa0e90c000 r--p 00002000 00:2b 14178678                   /elfs/elf.stripped
55aa0e90c000-55aa0e90d000 rw-p 00003000 00:2b 14178678                   /elfs/elf.stripped
7f7aaabf6000-7f7aaabf9000 rw-p 00000000 00:00 0
7f7aaabf9000-7f7aaac21000 r--p 00000000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaac21000-7f7aaadb6000 r-xp 00028000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaadb6000-7f7aaae0e000 r--p 001bd000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaae0e000-7f7aaae12000 r--p 00214000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaae12000-7f7aaae14000 rw-p 00218000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaae14000-7f7aaae21000 rw-p 00000000 00:00 0
7f7aaae24000-7f7aaae26000 rw-p 00000000 00:00 0
7f7aaae26000-7f7aaae28000 r--p 00000000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae28000-7f7aaae52000 r-xp 00002000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae52000-7f7aaae5d000 r--p 0002c000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae5e000-7f7aaae60000 r--p 00037000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae60000-7f7aaae62000 rw-p 00039000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffedc173000-7ffedc194000 rw-p 00000000 00:00 0                          [stack]
7ffedc1c0000-7ffedc1c4000 r--p 00000000 00:00 0                          [vvar]
7ffedc1c4000-7ffedc1c6000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf.stripped", 4457, 0x55aa0e908000},
		{"main", "elf.stripped", 4505, 0x55aa0e908000},
		{"open64", "libc.so.6", 1132176, 0x7f7aaabf9000},
		{"close", "libc.so.6", 1134848, 0x7f7aaabf9000},
	}
	testProc(t, maps, syms)
}

func TestDebugFileDebugLink(t *testing.T) {
	maps := `55aa0e908000-55aa0e909000 r--p 00000000 00:2b 14178678                   /elfs/elf.debuglink
55aa0e909000-55aa0e90a000 r-xp 00001000 00:2b 14178678                   /elfs/elf.debuglink
55aa0e90a000-55aa0e90b000 r--p 00002000 00:2b 14178678                   /elfs/elf.debuglink
55aa0e90b000-55aa0e90c000 r--p 00002000 00:2b 14178678                   /elfs/elf.debuglink
55aa0e90c000-55aa0e90d000 rw-p 00003000 00:2b 14178678                   /elfs/elf.debuglink
7f7aaabf6000-7f7aaabf9000 rw-p 00000000 00:00 0
7f7aaabf9000-7f7aaac21000 r--p 00000000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaac21000-7f7aaadb6000 r-xp 00028000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaadb6000-7f7aaae0e000 r--p 001bd000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaae0e000-7f7aaae12000 r--p 00214000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaae12000-7f7aaae14000 rw-p 00218000 00:2b 5231163                    /usr/lib/x86_64-linux-gnu/libc.so.6
7f7aaae14000-7f7aaae21000 rw-p 00000000 00:00 0
7f7aaae24000-7f7aaae26000 rw-p 00000000 00:00 0
7f7aaae26000-7f7aaae28000 r--p 00000000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae28000-7f7aaae52000 r-xp 00002000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae52000-7f7aaae5d000 r--p 0002c000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae5e000-7f7aaae60000 r--p 00037000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f7aaae60000-7f7aaae62000 rw-p 00039000 00:2b 5231145                    /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffedc173000-7ffedc194000 rw-p 00000000 00:00 0                          [stack]
7ffedc1c0000-7ffedc1c4000 r--p 00000000 00:00 0                          [vvar]
7ffedc1c4000-7ffedc1c6000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`
	var syms = []procTestdata{
		{"iter", "elf.stripped", 4457, 0x55aa0e908000},
		{"main", "elf.stripped", 4505, 0x55aa0e908000},
		{"open64", "libc.so.6", 1132176, 0x7f7aaabf9000},
		{"close", "libc.so.6", 1134848, 0x7f7aaabf9000},
	}
	testProc(t, maps, syms)
}

func TestMallocResolve(t *testing.T) {
	gosym := NewProcTable(ProcTableOptions{
		Pid:              os.Getpid(),
		IgnoreDebugFiles: true,
	})
	gosym.Refresh()
	malloc := testHelperGetMalloc()
	res := gosym.Resolve(uint64(malloc))
	if res == nil {
		t.Fatalf("expected malloc sym, got %v", res)
	}
	if !strings.Contains(res.Name, "malloc") {
		t.Errorf("expected malloc got %s", res.Name)
	}
	if !strings.Contains(res.Module, "libc.so") {
		t.Errorf("expected libc, got %v", res.Module)
	}
	fmt.Printf("malloc at %x\n", malloc)
}

func BenchmarkProc(b *testing.B) {
	gosym, _ := newGoSymbols("/proc/self/exe")
	bccsym := NewProcTable(ProcTableOptions{Pid: os.Getpid()})
	bccsym.Refresh()
	if len(gosym.symbols) < 1000 {
		b.FailNow()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, symbol := range gosym.symbols {
			bccsym.Resolve(symbol.Start)
		}
	}
}
