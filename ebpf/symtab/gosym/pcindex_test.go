package gosym

import (
	"fmt"
	"math/rand"
	"testing"

	"golang.org/x/exp/slices"
)

func BenchmarkBinSearch(b *testing.B) {
	const nsym = 64 * 1024
	rnd := rand.NewSource(239)
	syms := make([]uint64, nsym)
	for i := 0; i < nsym; i++ {
		syms[i] = uint64(rnd.Int63()) & 0x7fffffff
	}
	slices.Sort(syms)

	pci := NewPCIndex(nsym)
	for i, sym := range syms {
		pci.Set(i, sym)
	}
	b.ResetTimer()
	idx := 0
	for i := 0; i < b.N; i++ {
		for j := 0; j < nsym; j++ {
			idx += pci.FindIndex(syms[j])
		}
	}
	b.StopTimer()
	fmt.Println(idx)
}
