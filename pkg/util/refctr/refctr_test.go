package refctr

import (
	"io"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_Counter(t *testing.T) {
	const (
		workers = 100
		cycles  = 1000
	)

	var r Counter
	var wg sync.WaitGroup
	var inits int64

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < cycles; j++ {
				err := r.Inc(func() error {
					if j%4 == 0 {
						return io.EOF
					}
					inits++
					return nil
				})
				if err != nil {
					continue
				}
				// Let others touch r while
				// it is initialized.
				if j%workers == 0 {
					runtime.Gosched()
				}
				r.Dec(func() {
					inits--
				})
			}
		}()
	}

	wg.Wait()
	require.Zero(t, inits)
}
