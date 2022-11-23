//go:build linux
// +build linux

package symtab

import (
	"encoding/hex"
	"reflect"
	"strings"
	"testing"
)

func TestGoSymSelfTest(t *testing.T) {
	var ptr = reflect.ValueOf(TestGoSymSelfTest).Pointer()
	mod := "/proc/self/exe"
	symtab, err := NewGoSymbolTable(mod, nil)
	if err != nil {
		t.Fatalf("failed to create symtab %v", err)
	}
	sym := symtab.Resolve(uint64(ptr), false)
	expectedSym := "github.com/pyroscope-io/pyroscope/pkg/agent/ebpfspy/symtab.TestGoSymSelfTest"
	if sym.Name != expectedSym {
		t.Fatalf("Expected %s got %s", expectedSym, sym.Name)
	}
	if sym.Module != mod {
		t.Fatalf("Expected %s got %s", mod, sym.Module)
	}
	if sym.Offset != uint64(ptr) {
		t.Fatalf("Expected %d got %d", ptr, sym.Offset)
	}
}

func TestPclntab18(t *testing.T) {
	s := "f0 ff ff ff 00 00 01 08 9a 05 00 00 00 00 00 00 " +
		" bb 00 00 00 00 00 00 00 a0 23 40 00 00 00 00 00" +
		" 60 00 00 00 00 00 00 00 c0 bb 00 00 00 00 00 00" +
		" c0 c3 00 00 00 00 00 00 c0 df 00 00 00 00 00 00"
	bs, _ := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	textStart := parseRuntimeTextFromPclntab18(bs)
	expected := uint64(0x4023a0)
	if textStart != expected {
		t.Fatalf("expected %d got %d", expected, textStart)
	}
}

func BenchmarkGoSym(b *testing.B) {
	gosym, _ := NewGoSymbolTable("/proc/self/exe", nil)
	if len(gosym.tab.symbols) < 1000 {
		b.FailNow()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, symbol := range gosym.tab.symbols {
			gosym.Resolve(symbol.Entry, false)
		}
	}
}
