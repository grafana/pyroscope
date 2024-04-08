package symtab

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

type MockSymTab struct {
	Symbols []MockSym
	base    uint64
}

type MockSym struct {
	Start uint64
	Name  string
}

func NewSymTab(symbols []MockSym) *MockSymTab {
	return &MockSymTab{Symbols: symbols}
}

func (t *MockSymTab) Rebase(base uint64) {
	t.base = base
}

func (t *MockSymTab) Resolve(addr uint64) *MockSym {
	if len(t.Symbols) == 0 {
		return nil
	}
	addr -= t.base
	if addr < t.Symbols[0].Start {
		return nil
	}
	i := sort.Search(len(t.Symbols), func(i int) bool {
		return addr < t.Symbols[i].Start
	})
	i--
	return &t.Symbols[i]
}

func (t *MockSymTab) Length() int {
	return len(t.Symbols)
}

func TestSymTab(t *testing.T) {
	sym := NewSymTab([]MockSym{
		{0x1000, "0x1000"},
		{0x1200, "0x1200"},
		{0x1300, "0x1300"},
	})
	expect := func(t *testing.T, expected string, at uint64) {
		resolved := sym.Resolve(at)
		if expected == "" {
			require.Nil(t, resolved)
			return
		}
		require.NotNil(t, resolved)
		require.Equal(t, expected, resolved.Name)
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
