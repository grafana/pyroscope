//go:build ebpfspy

package symtab

import (
	"os"
	"strings"
	"testing"
)

func TestGoSymBccFallback(t *testing.T) {
	bcc := func() SymbolTable {
		return NewBCCSymbolTable(os.Getpid())
	}
	gosym, _ := NewGoSymbolTable("/proc/self/exe", os.Getpid(), &bcc)
	malloc := testHelperGetMalloc()
	res := gosym.Resolve(uint64(malloc), false)
	if !strings.Contains(res.Name, "malloc") {
		t.FailNow()
	}
	if !strings.Contains(res.Module, "libc.so") {
		t.FailNow()
	}
}
func BenchmarkBCC(b *testing.B) {
	gosym, _ := NewGoSymbolTable("/proc/self/exe", os.Getpid(), nil)
	bccsym := NewBCCSymbolTable(os.Getpid())
	if len(gosym.tab.symbols) < 1000 {
		b.FailNow()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, symbol := range gosym.tab.symbols {
			bccsym.Resolve(symbol.Entry, false)
		}
	}
}
