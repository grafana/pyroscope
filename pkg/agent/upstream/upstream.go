package upstream

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type UploadJob struct {
	Name            string
	StartTime       time.Time
	EndTime         time.Time
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
	Trie            *transporttrie.Trie
}

type Upstream interface {
	Start()
	Stop()
	Upload(u *UploadJob)
}
