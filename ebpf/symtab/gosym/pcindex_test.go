package gosym

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/exp/slices"
)

func TestPCIndex_FindIndex(t *testing.T) {
	s := "aaaaccfff"
	pci := NewPCIndex(len(s))
	for i := 0; i < len(s); i++ {
		pci.Set(i, uint64(s[i]))
	}
	assert.Equal(t, -1, pci.FindIndex(uint64(0x20)))
	assert.Equal(t, 0, pci.FindIndex(uint64('a')))
	assert.Equal(t, 0, pci.FindIndex(uint64('b')))
	assert.Equal(t, 4, pci.FindIndex(uint64('c')))
	assert.Equal(t, 4, pci.FindIndex(uint64('d')))
	assert.Equal(t, 4, pci.FindIndex(uint64('e')))
	assert.Equal(t, 6, pci.FindIndex(uint64('f')))
	assert.Equal(t, 6, pci.FindIndex(uint64('z')))
}

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
	//fmt.Println(idx)
}
