package symtab

import (
	"fmt"
	"testing"
)

var testdata = `ffffffff81000000 T _text
ffffffff81000040 T _stext
ffffffff81000060 T startup_64
ffffffff81000170 T x86_64_start_kernel
ffffffff810001e0 T x86_64_start_reservations
ffffffff81000250 T start_kernel
ffffffff81000ad0 T setup_arch
ffffffff81001200 T setup_machine_fdt	[fake_module]
ffffffff81001450 T setup_machine_tags
ffffffff81001630 T reserve_early
ffffffff81001640 D data_symbol
ffffffff81001660 T free_memory_resource
ffffffff810016a0 T alloc_memory_resource
ffffffff810016f0 T memblock_reserve
ffffffff81001720 T memblock_free
ffffffff81001750 T memblock_find
ffffffff81001780 T __memblock_alloc_base
ffffffff810017d0 T memblock_alloc
ffffffff81001820 T early_memtest
ffffffff810018a0 T early_memtest_report`

func TestKallsyms(t *testing.T) {
	kallsyms, err := NewKallsyms([]byte(testdata))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%+v\n", kallsyms)
	testcases := []struct {
		addr uint64
		name string
		mod  string
	}{
		{0xffffffff81001820, "early_memtest", "kernel"},
		{0xffffffff810018a0, "early_memtest_report", "kernel"},
		{0xffffffff81001640, "reserve_early", "kernel"},
		{0xffffffff81001200, "setup_machine_fdt", "fake_module"},
	}
	for _, testcase := range testcases {
		resolved := kallsyms.Resolve(testcase.addr)
		if testcase.name == "" {
			if resolved != nil {
				t.Fatalf("expected nil, got %v", resolved)
			}
			return
		}
		if resolved == nil {
			t.Fatalf("failed to resolve %v %v %v", testcase.addr, testcase.name, resolved)
		}
		if resolved.Name != testcase.name {
			t.Fatalf("failed to resolve %v %v %v", testcase.addr, testcase.name, resolved)
		}
		if resolved.Module != testcase.mod {
			t.Fatalf("failed to resolve %v %v %v", testcase.addr, testcase.name, resolved)
		}
	}
}
