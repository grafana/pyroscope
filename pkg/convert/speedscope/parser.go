package speedscope

import (
	"context"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

// RawProfile implements ingestion.RawProfile for Speedscope format
type RawProfile struct {
	RawData []byte
}

// Parse parses a profile
func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
	input := storage.PutInput{
		StartTime:       md.StartTime,
		EndTime:         md.EndTime,
		Key:             md.Key,
		SpyName:         md.SpyName,
		SampleRate:      md.SampleRate,
		Units:           md.Units,
		AggregationType: md.AggregationType,
	}
	return p.convert(putter, &input)
}

func (p *RawProfile) convert(putter storage.Putter, input *storage.PutInput) error {
	panic("TODO")
}

// Bytes returns the raw bytes of the profile
func (p *RawProfile) Bytes() ([]byte, error) {
	return p.RawData, nil
}

// ContentType returns the HTTP ContentType of the profile
func (p *RawProfile) ContentType() string {
	return "application/json"
}
