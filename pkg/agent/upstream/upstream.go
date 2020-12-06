package upstream

import (
	"time"

	"github.com/petethepig/pyroscope/pkg/structs/transporttrie"
)

type Upstream interface {
	Start()
	Stop()
	Upload(name string, startTime time.Time, endTime time.Time, t *transporttrie.Trie)
}
