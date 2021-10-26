package upstream

import (
	"time"
)

type UploadFormat string
type Payload interface {
	Bytes() []byte
}

const (
	Pprof UploadFormat = "pprof"
	Trie               = "trie"
)

type UploadJob struct {
	Name            string
	StartTime       time.Time
	EndTime         time.Time
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string
	Format          UploadFormat
	Payload         Payload
}

type Upstream interface {
	Stop()
	Upload(u *UploadJob)
}
