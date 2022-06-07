package profile

import (
	"bytes"
	"context"
	"fmt"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type RawProfile struct {
	Format  ingestion.Format
	RawData []byte
}

func (p *RawProfile) Bytes() ([]byte, error) { return p.RawData, nil }

func (*RawProfile) ContentType() string { return "binary/octet-stream" }

func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, exporter storage.MetricsExporter, md ingestion.Metadata) error {
	input := &storage.PutInput{
		StartTime:       md.StartTime,
		EndTime:         md.EndTime,
		Key:             md.Key,
		SpyName:         md.SpyName,
		SampleRate:      md.SampleRate,
		Units:           md.Units,
		AggregationType: md.AggregationType,
	}

	input.Val = tree.New()
	cb := input.Val.InsertInt
	if o, ok := exporter.Evaluate(input); ok {
		cb = func(k []byte, v int) {
			o.Observe(k, v)
			cb(k, v)
		}
	}

	r := bytes.NewReader(p.RawData)
	var err error
	switch p.Format {
	case ingestion.FormatTrie:
		err = transporttrie.IterateRaw(r, make([]byte, 0, 256), cb)
	case ingestion.FormatTree:
		err = convert.ParseTreeNoDict(r, cb)
	case ingestion.FormatLines:
		err = convert.ParseIndividualLines(r, cb)
	case ingestion.FormatGroups:
		err = convert.ParseGroups(r, cb)
	default:
		return fmt.Errorf("unknown format %q", p.Format)
	}

	if err != nil {
		return err
	}

	if err = putter.Put(ctx, input); err != nil {
		return ingestion.Error{Err: err}
	}

	return nil
}
