//go:build linux

package symtab

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/grafana/phlare/ebpf/symtab/elf"
	gosym2 "github.com/grafana/phlare/ebpf/symtab/gosym"
)

func TestGoSymSelfTest(t *testing.T) {
	var ptr = reflect.ValueOf(TestGoSymSelfTest).Pointer()
	mod := "/proc/self/exe"
	me, err := elf.NewMMapedElfFile(mod)
	require.NoError(t, err)
	defer me.Close()
	symtab, err := me.NewGoTable()
	require.NoError(t, err)
	sym := symtab.Resolve(uint64(ptr))
	expectedSym := "github.com/grafana/phlare/ebpf/symtab.TestGoSymSelfTest"
	require.NotNil(t, sym)
	require.Equal(t, expectedSym, sym)
}

func TestPclntab18(t *testing.T) {
	s := "f0 ff ff ff 00 00 01 08 9a 05 00 00 00 00 00 00 " +
		" bb 00 00 00 00 00 00 00 a0 23 40 00 00 00 00 00" +
		" 60 00 00 00 00 00 00 00 c0 bb 00 00 00 00 00 00" +
		" c0 c3 00 00 00 00 00 00 c0 df 00 00 00 00 00 00"
	bs, _ := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	textStart := gosym2.ParseRuntimeTextFromPclntab18(bs)
	expected := uint64(0x4023a0)
	require.Equal(t, expected, textStart)
}

func BenchmarkGoSym(b *testing.B) {
	mod := "/proc/self/exe"
	symbols, err := elf.GetGoSymbols(mod, false)
	require.NoError(b, err)
	me, err := elf.NewMMapedElfFile(mod)
	require.NoError(b, err)
	defer me.Close()
	gosym, err := me.NewGoTable()
	require.NoError(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, symbol := range symbols {
			gosym.Resolve(symbol.Start)
		}
	}
}
