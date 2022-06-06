package pprof

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
)

type RawProfile struct {
	m sync.Mutex
	// Initializes lazily on Bytes, if not present.
	RawData  []byte // Represents raw request body as per ingestion API.
	Boundary string
	// Initializes lazily on Parse, if not present.
	Profile          *bytes.Buffer
	PreviousProfile  *bytes.Buffer
	SampleTypeConfig map[string]*tree.SampleTypeConfig
	parser           *Parser
}

func (p *RawProfile) Push(profile *bytes.Buffer) {
	p.m.Lock()
	p.RawData = nil
	p.PreviousProfile, p.Profile = p.Profile, profile
	p.m.Unlock()
}

func (p *RawProfile) Put(profile *bytes.Buffer) {
	p.m.Lock()
	p.RawData = nil
	p.Profile = profile
	p.m.Unlock()
}

const (
	formFieldProfile, formFileProfile                   = "profile", "profile.pprof"
	formFieldPreviousProfile, formFilePreviousProfile   = "prev_profile", "profile.pprof"
	formFieldSampleTypeConfig, formFileSampleTypeConfig = "sample_type_config", "sample_type_config.json"
)

func (p *RawProfile) Bytes() ([]byte, error) {
	p.m.Lock()
	defer p.m.Unlock()
	if p.RawData != nil {
		return p.RawData, nil
	}
	if p.Profile == nil && p.PreviousProfile == nil {
		return nil, nil
	}
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	ff, err := mw.CreateFormFile(formFieldProfile, formFileProfile)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(ff, p.Profile)
	if p.PreviousProfile != nil {
		if ff, err = mw.CreateFormFile(formFieldPreviousProfile, formFilePreviousProfile); err != nil {
			return nil, err
		}
		_, _ = io.Copy(ff, p.PreviousProfile)
	}
	if len(p.SampleTypeConfig) > 0 {
		if ff, err = mw.CreateFormFile(formFieldSampleTypeConfig, formFileSampleTypeConfig); err != nil {
			return nil, err
		}
		_ = json.NewEncoder(ff).Encode(p.SampleTypeConfig)
	}
	_ = mw.Close()
	p.RawData = b.Bytes()
	p.Boundary = mw.Boundary()
	return p.RawData, nil
}

func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
	p.m.Lock()
	defer p.m.Unlock()
	if p.Profile == nil && p.PreviousProfile == nil {
		if p.RawData == nil {
			return nil
		}
		if p.Boundary != "" {
			if err := p.loadPprofFromForm(); err != nil {
				return err
			}
		} else {
			p.Profile = bytes.NewBuffer(p.RawData)
		}
	}
	if p.Profile == nil {
		return nil
	}
	if p.parser == nil {
		sampleTypes := tree.DefaultSampleTypeMapping
		if p.SampleTypeConfig != nil {
			sampleTypes = p.SampleTypeConfig
		}
		p.parser = NewParser(ParserConfig{
			SpyName:     md.SpyName,
			Labels:      md.Key.Labels(),
			Putter:      putter,
			SampleTypes: sampleTypes,
		})
		if p.PreviousProfile != nil {
			if err := p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, p.PreviousProfile); err != nil {
				return err
			}
		}
	}

	return p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, p.Profile)
}

func (p *RawProfile) loadPprofFromForm() error {
	// maxMemory 32MB.
	// TODO(kolesnikovae): If the limit is exceeded, parts will be written
	//  to disk. It may be better to limit the request body size to be sure
	//  that they loaded into memory entirely.
	form, err := multipart.NewReader(bytes.NewReader(p.RawData), p.Boundary).ReadForm(32 << 20)
	if err != nil {
		return err
	}
	p.Profile, err = formField(form, formFieldProfile)
	if err != nil {
		return err
	}
	p.PreviousProfile, err = formField(form, formFieldPreviousProfile)
	if err != nil {
		return err
	}
	p.SampleTypeConfig, err = parseSampleTypesConfig(form)
	return err
}

func formField(form *multipart.Form, name string) (r *bytes.Buffer, err error) {
	files, ok := form.File[name]
	if !ok || len(files) == 0 {
		return nil, nil
	}
	fh := files[0]
	if fh.Size == 0 {
		return nil, nil
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer func() {
		err = f.Close()
	}()
	b := bytes.NewBuffer(make([]byte, 0, fh.Size))
	if _, err = io.Copy(b, f); err != nil {
		return nil, err
	}
	return b, nil
}

func parseSampleTypesConfig(form *multipart.Form) (map[string]*tree.SampleTypeConfig, error) {
	r, err := formField(form, formFieldSampleTypeConfig)
	if err != nil || r == nil {
		return nil, err
	}
	var config map[string]*tree.SampleTypeConfig
	return config, json.NewDecoder(r).Decode(&config)
}
