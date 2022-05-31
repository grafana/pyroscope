// Package parser deals with parsing various incoming formats
package parser

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/convert/jfr"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
)

type PutInput struct {
	Format           Format
	Profile          io.Reader
	PreviousProfile  io.Reader
	SampleTypeConfig map[string]*tree.SampleTypeConfig

	// these parameters are the same as the ones in storage.PutInput
	StartTime       time.Time
	EndTime         time.Time
	Key             *segment.Key
	SpyName         string
	SampleRate      uint32
	Units           metadata.Units
	AggregationType metadata.AggregationType
}

type Format string

const (
	Pprof  Format = "pprof"
	JFR    Format = "jfr"
	Trie   Format = "trie"
	Tree   Format = "tree"
	Lines  Format = "lines"
	Groups Format = "groups"
)

type Parser struct {
	log      *logrus.Logger
	putter   storage.Putter
	exporter storage.MetricsExporter
}

func New(log *logrus.Logger, s storage.Putter, exporter storage.MetricsExporter) *Parser {
	return &Parser{
		log:      log,
		putter:   s,
		exporter: exporter,
	}
}

func (p *Parser) createParseCallback(pi *storage.PutInput) func([]byte, int) {
	pi.Val = tree.New()
	cb := pi.Val.InsertInt
	o, ok := p.exporter.Evaluate(pi)
	if !ok {
		return cb
	}
	return func(k []byte, v int) {
		o.Observe(k, v)
		cb(k, v)
	}
}

// Put takes parser.PutInput, turns it into storage.PutIntput and passes it to Putter.
// TODO(kolesnikovae): Should we split it into more specific methods (e.g PutPprof, PutTrie, etc)?
func (p *Parser) Put(ctx context.Context, in *PutInput) error {
	pi := &storage.PutInput{
		StartTime:       in.StartTime,
		EndTime:         in.EndTime,
		Key:             in.Key,
		SpyName:         in.SpyName,
		SampleRate:      in.SampleRate,
		Units:           in.Units,
		AggregationType: in.AggregationType,
	}

	cb := p.createParseCallback(pi)
	var err error
	switch in.Format {
	default:
		return fmt.Errorf("unknown format %q", in.Format)

	// with some formats we write directly to storage, hence the early return
	case JFR:
		return jfr.ParseJFR(ctx, p.putter, in.Profile, pi)
	case Pprof:
		return writePprof(ctx, p.putter, in)

	case Trie:
		err = transporttrie.IterateRaw(in.Profile, make([]byte, 0, 256), cb)
	case Tree:
		err = convert.ParseTreeNoDict(in.Profile, cb)
	case Lines:
		err = convert.ParseIndividualLines(in.Profile, cb)
	case Groups:
		err = convert.ParseGroups(in.Profile, cb)
	}

	if err != nil {
		return err
	}

	if err = p.putter.Put(ctx, pi); err != nil {
		return storage.IngestionError{Err: err}
	}

	return nil
}

func writePprof(ctx context.Context, s storage.Putter, pi *PutInput) error {
	if len(pi.SampleTypeConfig) == 0 {
		pi.SampleTypeConfig = tree.DefaultSampleTypeMapping
	}

	w := pprof.NewProfileWriter(s, pprof.ProfileWriterConfig{
		SampleTypes: pi.SampleTypeConfig,
		Labels:      pi.Key.Labels(),
		SpyName:     pi.SpyName,
	})

	if pi.PreviousProfile != nil {
		if err := pprof.DecodePool(pi.PreviousProfile, func(p *tree.Profile) error {
			return w.WriteProfile(ctx, pi.StartTime, pi.EndTime, p)
		}); err != nil {
			return err
		}
	}

	return pprof.DecodePool(pi.Profile, func(p *tree.Profile) error {
		return w.WriteProfile(ctx, pi.StartTime, pi.EndTime, p)
	})
}
