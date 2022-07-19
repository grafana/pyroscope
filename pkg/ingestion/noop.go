package ingestion

import "context"

type NoopIngester struct{}

func NewNoopIngester() *NoopIngester {
	return &NoopIngester{}
}
func (*NoopIngester) Ingest(ctx context.Context, in *IngestInput) error {
	return nil
}
