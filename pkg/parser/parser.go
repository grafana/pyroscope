// Package parser deals with parsing various incoming formats
package parser

import (
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"

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
	Units           metadata.Units
	AggregationType metadata.AggregationType
}

type Parser struct {
	log        *logrus.Logger
	putter     storage.Putter
	exporter   storage.MetricsExporter
	bufferPool *bytebufferpool.Pool
}

func New(log *logrus.Logger, s storage.Putter, exporter storage.MetricsExporter) *Parser {
	return &Parser{
		log:        log,
		putter:     s,
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
		err = jfr.ParseJFR(ctx, in.Body, p.putter, pi)
	case in.Format == "pprof":
		err = writePprofFromBody(ctx, p.putter, in)
	case strings.Contains(in.ContentType, "multipart/form-data"):
		err = writePprofFromForm(ctx, p.putter, in)
	default:
		err = convert.ParseGroups(in.Body, cb)
	}

	if err != nil {
		return err
	}

	// with some formats we write directly to storage (e.g look at "multipart/form-data" above)
	// TODO(petethepig): this is unintuitive and error prone, need to refactor at some point
	if pi.Val == nil {
		return nil
	}

	if err = p.putter.Put(ctx, pi); err != nil {
		return storage.IngestionError{Err: err}
	}

	return nil
}

func writePprofFromBody(ctx context.Context, s storage.Putter, pi *PutInput) error {
	w := pprof.NewProfileWriter(s, pprof.ProfileWriterConfig{
		SampleTypes: tree.DefaultSampleTypeMapping,
		Labels:      pi.Key.Labels(),
		SpyName:     pi.SpyName,
	})
	return pprof.DecodePool(pi.Body, func(p *tree.Profile) error {
		return w.WriteProfile(ctx, pi.StartTime, pi.EndTime, p)
	})
}

func writePprofFromForm(ctx context.Context, s storage.Putter, pi *PutInput) error {
	// maxMemory 32MB
	form, err := multipart.NewReader(pi.Body, pi.MultipartBoundary).ReadForm(32 << 20)
	if err != nil {
		return err
	}

	sampleTypesConfig, err := parseSampleTypesConfig(form)
	if err != nil {
		return err
	}
	if sampleTypesConfig == nil {
		sampleTypesConfig = tree.DefaultSampleTypeMapping
	}

	w := pprof.NewProfileWriter(s, pprof.ProfileWriterConfig{
		SampleTypes: sampleTypesConfig,
		Labels:      pi.Key.Labels(),
		SpyName:     pi.SpyName,
	})
	if err = writePprofFromFormField(ctx, form, w, pi, "prev_profile"); err != nil {
		return err
	}
	return writePprofFromFormField(ctx, form, w, pi, "profile")
}

func writePprofFromFormField(ctx context.Context, form *multipart.Form, w *pprof.ProfileWriter, pi *PutInput, name string) error {
	r, err := formField(form, name)
	if err != nil {
		return err
	}
	if r == nil {
		return nil
	}
	return pprof.DecodePool(r, func(p *tree.Profile) error {
		return w.WriteProfile(ctx, pi.StartTime, pi.EndTime, p)
	})
}

func formField(form *multipart.Form, name string) (io.Reader, error) {
	files, ok := form.File[name]
	if !ok || len(files) == 0 {
		return nil, nil
	}
	f, err := files[0].Open()
	if err != nil {
		return nil, err
	}
	return f, nil
}

func parseSampleTypesConfig(form *multipart.Form) (map[string]*tree.SampleTypeConfig, error) {
	r, err := formField(form, "sample_type_config")
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}

	d := json.NewDecoder(r)

	var config map[string]*tree.SampleTypeConfig
	err = d.Decode(&config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
