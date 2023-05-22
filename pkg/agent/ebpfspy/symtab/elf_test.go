package symtab

import (
	"testing"
)

func TestElf(t *testing.T) {
	tab, err := NewElfTable(".", "testdata/elfs/elf", false)

	if err != nil {
		t.Fatal(err)
	}
	syms := []struct {
		name string
		pc   uint64
	}{
		{"iter", 4457},
		{"main", 4505},
	}
	for _, sym := range syms {
		res := tab.table.Resolve(sym.pc)
		if res == nil || res.Name != sym.name {
			t.Errorf("failed to resolv %v got %v", sym, res)
		}
	}
}
