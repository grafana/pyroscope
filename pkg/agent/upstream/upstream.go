package upstream

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type UploadJob struct {
	Name            string
	StartTime       time.Time
	EndTime         time.Time
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string
	Trie            *transporttrie.Trie
}

type Upstream interface {
	Stop()
	// TODO: too complex, fix it
	Upload(u *UploadJob)
}
