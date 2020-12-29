package upstream

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type Upstream interface {
	Stop()
	Upload(name string, startTime time.Time, endTime time.Time, t *transporttrie.Trie)
}
