package elf

import (
	"debug/elf"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"
)

func TestElfSymbolComparison(t *testing.T) {
	testOneElfFile := func(t *testing.T, f string) {
		e, err := elf.Open(f)
		require.NoError(t, err)
		defer e.Close()

		genuineSymbols := GetELFSymbolsFromSymtab(e)

		me, err := NewMMapedElfFile(f)
		require.NoError(t, err)
		defer me.Close()

		tab, _ := me.NewSymbolTable()
		if tab == nil {
			tab = &SymbolTable{}
		}
		var mySymbols []TestSym

		require.Equal(t, len(genuineSymbols), len(tab.Index.Names))
		for i, symbol := range genuineSymbols {
			require.Equal(t, symbol.Start, tab.Index.Values.Value(i))
			name, _ := tab.symbolName(i)
			mySymbols = append(mySymbols, TestSym{
				Name:  name,
				Start: symbol.Start,
			})
		}

		cmp := func(a, b TestSym) bool {
			if a.Start == b.Start {
				return strings.Compare(a.Name, b.Name) < 0
			}
			return a.Start < b.Start
		}
		slices.SortFunc(mySymbols, cmp)
		slices.SortFunc(genuineSymbols, cmp)
		require.Equal(t, genuineSymbols, mySymbols)
	}

	fs := []string{
		"./testdata/elfs/elf",
		"./testdata/elfs/elf.debug",
		"./testdata/elfs/elf.nopie",
		"./testdata/elfs/libexample.so",
		"./testdata/elfs/go12",
		"./testdata/elfs/go16",
		"./testdata/elfs/go18",
		"./testdata/elfs/go20",
		"./testdata/elfs/go12-static",
		"./testdata/elfs/go16-static",
		"./testdata/elfs/go18-static",
		"./testdata/elfs/go20-static",
	}
	for _, f := range fs {
		t.Run(f, func(t *testing.T) {
			testOneElfFile(t, f)
		})
	}
}
