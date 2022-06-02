// Package parser deals with parsing various incoming formats
package parser

import (
	"bytes"
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
	Format Format

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

func (pi *PutInput) Clone() *PutInput {
	return &PutInput{
		Format:           pi.Format,
		Profile:          cloneProfile(pi.Profile),
		PreviousProfile:  cloneProfile(pi.PreviousProfile),
		SampleTypeConfig: pi.SampleTypeConfig, // Read only.
		StartTime:        pi.StartTime,
		EndTime:          pi.EndTime,
		Key:              pi.Key.Clone(), // Can be modified at Normalized call.
		SpyName:          pi.SpyName,
		SampleRate:       pi.SampleRate,
		Units:            pi.Units,
		AggregationType:  pi.AggregationType,
	}
}

func cloneProfile(p io.Reader) io.Reader {
	if p == nil {
		return nil
	}
	switch x := p.(type) {
	case *bytes.Buffer:
		return bytes.NewBuffer(x.Bytes())
	case *PprofData:
		// FIXME(kolesnikovae): Ideally, we should clone it as well.
		//  As for now, we assume that the actual parsing happens just
		//  once. The problem is that caller (e.g scrapper) holds
		//  reference to *PprofData, therefore it can't be changed.
		x.Buffer = bytes.NewBuffer(x.Bytes())
		return x
	default:
		var b bytes.Buffer
		_, _ = io.Copy(&b, p)
		return &b
	}
}

func writePprof(ctx context.Context, putter storage.Putter, pi *PutInput) error {
	if len(pi.SampleTypeConfig) == 0 {
		pi.SampleTypeConfig = tree.DefaultSampleTypeMapping
	}

	if ok, err := tryUseParsedPprof(ctx, putter, pi); ok {
		return err
	}

	p := pprof.NewParser(pprof.ParserConfig{
		Putter:      putter,
		SampleTypes: pi.SampleTypeConfig,
		Labels:      pi.Key.Labels(),
		SpyName:     pi.SpyName,
	})

	if pi.PreviousProfile != nil {
		if err := p.ParsePprof(ctx, pi.StartTime, pi.EndTime, pi.PreviousProfile); err != nil {
			return err
		}
	}

	return p.ParsePprof(ctx, pi.StartTime, pi.EndTime, pi.Profile)
}

type PprofData struct {
	parser *pprof.Parser
	*bytes.Buffer
}

func tryUseParsedPprof(ctx context.Context, putter storage.Putter, pi *PutInput) (bool, error) {
	previous, ok := pi.PreviousProfile.(*PprofData)
	if !ok {
		return false, nil
	}
	current, ok := pi.Profile.(*PprofData)
	if !ok {
		return false, nil
	}

	parser := previous.parser
	if parser == nil {
		parser = pprof.NewParser(pprof.ParserConfig{
			Putter:      putter,
			SampleTypes: pi.SampleTypeConfig,
			Labels:      pi.Key.Labels(),
			SpyName:     pi.SpyName,
		})
		if err := parser.ParsePprof(ctx, pi.StartTime, pi.EndTime, pi.PreviousProfile); err != nil {
			return true, err
		}
	}

	if err := parser.ParsePprof(ctx, pi.StartTime, pi.EndTime, pi.Profile); err != nil {
		return true, err
	}

	current.parser = parser
	return true, nil
}
