package model

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
)

func TestProfilePresenceMerger_ConcurrentMerge(t *testing.T) {
	const goroutines = 16
	const perGoroutine = 50

	merger := NewProfilePresenceMerger()
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			entries := make([]*queryv1.ProfilePresenceEntry, perGoroutine)
			for i := range entries {
				entries[i] = &queryv1.ProfilePresenceEntry{ProfileId: fmt.Sprintf("%d-%d", g, i)}
			}
			merger.MergeProfilePresence(entries)
		}(g)
	}
	wg.Wait()

	require.Len(t, merger.Profiles(), goroutines*perGoroutine)
}
