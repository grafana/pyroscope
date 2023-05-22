package symtab

import (
	"testing"
)

func TestSymTab(t *testing.T) {
	sym := NewSymTab([]Symbol{
		{0x1000, "0x1000", ""},
		{0x1200, "0x1200", ""},
		{0x1300, "0x1300", ""},
	})
	expect := func(t *testing.T, expected string, at uint64) {
		resolved := sym.Resolve(at)
		if expected == "" {
			if resolved != nil {
				t.Fatalf("expected nil, got %v", resolved)
			}
			return
		}
		if resolved == nil {
			t.Fatalf("failed to resolve %v %v %v", expected, at, resolved)
		}
		s := resolved.Name
		if s != expected {
			t.Fatalf("Expected %s got %s", expected, s)
		}
	}
	bases := []uint64{0, 0x4000}
	testcases := []struct {
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
		{"0x1300", 0x3000},
		{"0x1300", 0x4000},
	}
	for _, base := range bases {
		for _, c := range testcases {
			sym.Rebase(base)
			expect(t, c.expected, base+c.addr)
		}
	}
}
