package profile

import (
	"bytes"
	"context"
	"fmt"

	"github.com/grafana/pyroscope/pkg/og/convert"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/og/structs/transporttrie"
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
		Val:             tree.New(),
	}

	cb := createParseCallback(input, exporter)
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

func createParseCallback(pi *storage.PutInput, e storage.MetricsExporter) func([]byte, int) {
	o, ok := e.Evaluate(pi)
	if !ok {
		return pi.Val.InsertInt
	}
	return func(k []byte, v int) {
		o.Observe(k, v)
		pi.Val.InsertInt(k, v)
	}
}
