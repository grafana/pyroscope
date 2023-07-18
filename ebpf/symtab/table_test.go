package symtab

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSymTab(t *testing.T) {
	sym := NewSymTab([]Sym{
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
