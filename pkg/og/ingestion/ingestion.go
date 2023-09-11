package ingestion

import (
  "context"
  "errors"
  "time"

  distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"

  "github.com/grafana/pyroscope/pkg/og/storage"
  "github.com/grafana/pyroscope/pkg/og/storage/metadata"
  "github.com/grafana/pyroscope/pkg/og/storage/segment"
)

type Ingester interface {
  Ingest(context.Context, *IngestInput) error
}

type IngestInput struct {
  Format   Format
  Profile  RawProfile
  Metadata Metadata
}

type Format string

const (
  FormatPprof      Format = "pprof"
  FormatJFR        Format = "jfr"
  FormatTrie       Format = "trie"
  FormatTree       Format = "tree"
  FormatLines      Format = "lines"
  FormatGroups     Format = "groups"
  FormatSpeedscope Format = "speedscope"
)

type RawProfile interface {
  Parse(context.Context, storage.Putter, storage.MetricsExporter, Metadata) error
}

type ParseableToPprof interface {
  ParseToPprof(context.Context, Metadata) (*distributormodel.PushRequest, error)
}

type Metadata struct {
  StartTime       time.Time
  EndTime         time.Time
  Key             *segment.Key
  SpyName         string
  SampleRate      uint32
  Units           metadata.Units
  AggregationType metadata.AggregationType
}

type Error struct{ Err error }

func (e Error) Error() string { return e.Err.Error() }

func (e Error) Unwrap() error { return e.Err }

func IsIngestionError(err error) bool {
  if err == nil {
    return false
  }
  var v Error
  return errors.As(err, &v)
}
