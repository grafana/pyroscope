package stackbuilder

import (
	"context"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
)

type SamplesAppender interface {
	Append(stackID, value uint64)
}

type Label struct{ Key, Value string }
type Labels []Label

type WriteBatch interface {
	StackBuilder() tree.StackBuilder
	SamplesAppender(startTime, endTime int64, labels Labels) SamplesAppender
	Flush()
}

type WriteBatchFactory interface {
	NewWriteBatch(appName string, md metadata.Metadata) (WriteBatch, error)
}

type WriteBatchParser interface {
	ParseWithWriteBatch(c context.Context, wbf WriteBatchFactory, md ingestion.Metadata) error
}
