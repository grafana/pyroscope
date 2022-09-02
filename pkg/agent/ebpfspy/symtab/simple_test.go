package symtab

import (
	"fmt"

	"testing"
)

func TestResolving(t *testing.T) {
	sym := NewSimpleSymbolTable([]SimpleSymbolTableEntry{
		{0x1000, 0x1200, "0x1000"},
		{0x1200, 0x1300, "0x1200"},
		{0x1300, 0x3000, "0x1300"},
	})
	expect := func(expected string, at uint64) {
		s := sym.Resolve(at)
		if s != expected {
			t.Fatalf("Expected %s got %s", expected, s)
		}
	}
	bases := []uint64{0, 0x4000}
	cases := []struct {
		expected string
		addr     uint64
	}{
		{"", 0xef},
		{"0x1000", 0x1000},
		{"0x1000", 0x1100},
		{"0x1000", 0x11FF},
		{"0x1200", 0x1200},
		{"0x1200", 0x12FF},
		{"0x1300", 0x1300},
		{"0x1300", 0x2FFF},
		{"", 0x3000},
		{"", 0x4000},
	}
	for _, base := range bases {
		for _, c := range cases {
			t.Run(fmt.Sprintf("base %x c %v", base, c), func(t *testing.T) {
				sym.Rebase(base)
				expect(c.expected, base+c.addr)
			})
		}
	}
}
