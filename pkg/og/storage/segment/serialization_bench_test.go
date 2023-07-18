package segment

import (
	"bytes"
	"fmt"
	"math/big"
	"math/rand"
	"testing"
	"time"

	ptesting "github.com/grafana/pyroscope/pkg/og/testing"
)

func serialize(s *Segment) []byte {
	var buf bytes.Buffer
	s.Serialize(&buf)
	return buf.Bytes()
}

func BenchmarkSerialize(b *testing.B) {
	for k := 10; k <= 1000000; k *= 10 {
		s := New()
		for i := 0; i < k; i++ {
			s.Put(ptesting.SimpleTime(i*10), ptesting.SimpleTime(i*10+9), uint64(rand.Intn(100)), func(de int, t time.Time, r *big.Rat, a []Addon) {})
		}
		b.ResetTimer()
		b.Run(fmt.Sprintf("serialize %d", k), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = serialize(s)
			}
		})
	}
}
