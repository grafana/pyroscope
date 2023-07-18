package storage

//revive:disable:max-public-structs TODO: we will refactor this later

import (
	"context"
	"time"

	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/segment"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

// MetricsExporter exports values of particular stack traces sample from profiling
// data as a Prometheus metrics.
type MetricsExporter interface {
	// Evaluate evaluates metrics export rules against the input key and creates
	// prometheus counters for new time series, if required. Returned observer can
	// be used to evaluate and observe particular samples.
	//
	// If there are no matching rules, the function returns false.
	Evaluate(*PutInput) (SampleObserver, bool)
}

type SampleObserver interface {
	// Observe adds v to the matched counters if k satisfies node selector.
	// k is a sample stack trace where frames are delimited by semicolon.
	// v is the sample value.
	Observe(k []byte, v int)
}

type PutInput struct {
	StartTime       time.Time
	EndTime         time.Time
	Key             *segment.Key
	Val             *tree.Tree
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
}

type Putter interface {
	Put(context.Context, *PutInput) error
}
