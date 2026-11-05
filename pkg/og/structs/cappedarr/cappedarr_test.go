package cappedarr

import (
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMinValue(t *testing.T) {
	t.Run("simple case returns correct value", func(t *testing.T) {
		values := []uint64{1, 2, 3, 4, 5, 6}
		for i := 0; i < 1000; i++ {
			ca := New(4)
			rand.Seed(time.Now().UnixNano())
			rand.Shuffle(len(values), func(i, j int) {
				values[i], values[j] = values[j], values[i]
			})

			for _, v := range values {
				ca.Push(v)
			}

			require.Equal(t, uint64(3), ca.MinValue())
		}
	})
}
