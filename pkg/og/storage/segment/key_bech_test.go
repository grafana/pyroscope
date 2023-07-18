package segment

import (
	"math/rand"
	"testing"
)

func BenchmarkKey_Parse(b *testing.B) {
	const (
		labelsSize = 10
		minLen     = 6
		maxLen     = 16
	)

	// Duplicates are okay.
	labels := make(map[string]string, labelsSize+1)
	for i := 0; i < labelsSize; i++ {
		labels[randString(randInt(minLen, maxLen))] = randString(randInt(minLen, maxLen))
	}

	labels["__name__"] = "benchmark.key.parse"
	keyStr := NewKey(labels).Normalized()

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := ParseKey(keyStr); err != nil {
			b.Fatal(err)
		}
	}
}

// TODO(kolesnikovae): This is not near perfect way of generating strings.
//  It makes sense to create a package for util functions like this.

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}
