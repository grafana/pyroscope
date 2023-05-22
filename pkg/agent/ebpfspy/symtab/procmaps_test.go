package symtab

import (
	"reflect"
	"testing"
)

func TestProcMaps(t *testing.T) {
	data := []byte(`5644e74cc000-5644e74ce000 r--p 00000000 09:00 523083                     /usr/bin/cat
5644e74ce000-5644e74d2000 r-xp 00002000 09:00 523083                     /usr/bin/cat
5644e74d2000-5644e74d4000 r--p 00006000 09:00 523083                     /usr/bin/cat
5644e74d4000-5644e74d5000 r--p 00007000 09:00 523083                     /usr/bin/cat
5644e74d5000-5644e74d6000 rw-p 00008000 09:00 523083                     /usr/bin/cat
5644e9081000-5644e90a2000 rw-p 00000000 00:00 0                          [heap]
7f582297e000-7f58229a0000 rw-p 00000000 00:00 0
7f58229a0000-7f5822c89000 r--p 00000000 09:00 532371                     /usr/lib/locale/locale-archive
7f5822c89000-7f5822c8c000 rw-p 00000000 00:00 0
7f5822c8c000-7f5822cb4000 r--p 00000000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f5822cb4000-7f5822e49000 r-xp 00028000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f5822e49000-7f5822ea1000 r--p 001bd000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f5822ea1000-7f5822ea5000 r--p 00214000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f5822ea5000-7f5822ea7000 rw-p 00218000 09:00 533580                     /usr/lib/x86_64-linux-gnu/libc.so.6
7f5822ea7000-7f5822eb4000 rw-p 00000000 00:00 0
7f5822ebc000-7f5822ebe000 rw-p 00000000 00:00 0
7f5822ebe000-7f5822ec0000 r--p 00000000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5822ec0000-7f5822eea000 r-xp 00002000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5822eea000-7f5822ef5000 r--p 0002c000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5822ef6000-7f5822ef8000 r--p 00037000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7f5822ef8000-7f5822efa000 rw-p 00039000 09:00 533429                     /usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
7ffe15767000-7ffe15788000 rw-p 00000000 00:00 0                          [stack]
7ffe157f0000-7ffe157f4000 r--p 00000000 00:00 0                          [vvar]
7ffe157f4000-7ffe157f6000 r-xp 00000000 00:00 0                          [vdso]
ffffffffff600000-ffffffffff601000 --xp 00000000 00:00 0                  [vsyscall]
`)
	maps, err := parseProcMaps(data)
	if err != nil {
		t.Fatal(err)
	}

	expected := []procMapEntry{
		{0x5644e74ce000, 0x5644e74d2000, 0x00002000, 523083, "/usr/bin/cat"},
		{0x7f5822cb4000, 0x7f5822e49000, 0x00028000, 533580, "/usr/lib/x86_64-linux-gnu/libc.so.6"},
		{0x7f5822ec0000, 0x7f5822eea000, 0x00002000, 533429, "/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2"},
		{0x7ffe157f4000, 0x7ffe157f6000, 0, 0, "[vdso]"},
		{0xffffffffff600000, 0xffffffffff601000, 0, 0, "[vsyscall]"},
	}
	if len(maps) != len(expected) {
		t.Fatalf("expected %v got %v", expected, maps)
	}
	for i := range expected {
		if !reflect.DeepEqual(expected[i], maps[i]) {
			t.Fatalf("expected %v got %v", expected[i], maps[i])
		}
	}
}
