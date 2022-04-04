// Package parser deals with parsing various incoming formats
package parser

import (
	"context"
	"io"
	"mime/multipart"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/convert/jfr"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"
)

type ParserStorage interface {
	storage.Putter
	storage.Enqueuer
}

type PutInput struct {
	Format            string
	ContentType       string
	Body              io.Reader
	MultipartBoundary string

	// these parameters are the same as the ones in storage.PutInput
	StartTime       time.Time
	EndTime         time.Time
	Key             *segment.Key
	SpyName         string
	SampleRate      uint32
	Units           string
	AggregationType string
}

type Parser struct {
	log        *logrus.Logger
	storage    ParserStorage
	exporter   storage.MetricsExporter
	bufferPool *bytebufferpool.Pool
}

func New(log *logrus.Logger, s ParserStorage, exporter storage.MetricsExporter) *Parser {
	return &Parser{
		log:        log,
		storage:    s,
		exporter:   exporter,
		bufferPool: &bytebufferpool.Pool{},
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

// Put takes parser.PutInput, turns it into storage.PutIntput and enqueues it for a write
func (p *Parser) Put(ctx context.Context, in *PutInput) (err error, pErr error) {
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
	switch {
	case in.Format == "trie", in.ContentType == "binary/octet-stream+trie":
		tmpBuf := p.bufferPool.Get()
		defer p.bufferPool.Put(tmpBuf)
		err = transporttrie.IterateRaw(in.Body, tmpBuf.B, cb)
	case in.Format == "tree", in.ContentType == "binary/octet-stream+tree":
		err = convert.ParseTreeNoDict(in.Body, cb)
	case in.Format == "lines":
		err = convert.ParseIndividualLines(in.Body, cb)
	case in.Format == "jfr":
		err = jfr.ParseJFR(ctx, in.Body, p.storage, pi)
	case strings.Contains(in.ContentType, "multipart/form-data"):
		err = writePprof(ctx, p.storage, in)
	default:
		err = convert.ParseGroups(in.Body, cb)
	}

	if err != nil {
		return err, pErr
	}

	// with some formats we write directly to storage (e.g look at "multipart/form-data" above)
	// TODO(petethepig): this is unintuitive and error prone, need to refactor at some point
	if pi.Val != nil {
		pErr = p.storage.Put(ctx, pi)
	}
	return err, pErr
}

func writePprof(ctx context.Context, s ParserStorage, pi *PutInput) error {
	// maxMemory 32MB
	form, err := multipart.NewReader(pi.Body, pi.MultipartBoundary).ReadForm(32 << 20)

	if err != nil {
		return err
	}
	w := pprof.NewProfileWriter(s, pi.Key.Labels(), tree.DefaultSampleTypeMapping)
	if err := writePprofFromForm(ctx, form, w, pi, "prev_profile"); err != nil {
		return err
	}
	return writePprofFromForm(ctx, form, w, pi, "profile")
}

func writePprofFromForm(ctx context.Context, form *multipart.Form, w *pprof.ProfileWriter, pi *PutInput, name string) error {
	files, ok := form.File[name]
	if !ok || len(files) == 0 {
		return nil
	}
	f, err := files[0].Open()
	if err != nil {
		return err
	}
	return pprof.DecodePool(f, func(p *tree.Profile) error {
		return w.WriteProfile(ctx, pi.StartTime, pi.EndTime, pi.SpyName, p)
	})
}
