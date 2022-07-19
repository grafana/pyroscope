package ingestion

import "context"

type NoopIngester struct{}

func NewNoopIngester() *NoopIngester {
	return &NoopIngester{}
}
func (*NoopIngester) Ingest(_ context.Context, _ *IngestInput) error {
	return nil
}
