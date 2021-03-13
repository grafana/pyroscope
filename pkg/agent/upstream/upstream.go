package upstream

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type Upstream interface {
	Stop()
	// TODO: too complex, fix it
	Upload(name string, startTime, endTime time.Time, spyName string, sampleRate int, t *transporttrie.Trie)
}
