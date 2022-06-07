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
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
)

type RawProfile struct {
	m sync.Mutex
	// Initializes lazily on Bytes, if not present.
	RawData             []byte // Represents raw request body as per ingestion API.
	FormDataContentType string // Set optionally, if RawData is multipart form.
	// Initializes lazily on Parse, if not present.
	Profile          []byte
	PreviousProfile  []byte
	SampleTypeConfig map[string]*tree.SampleTypeConfig

	parser *Parser
}

func (p *RawProfile) ContentType() string {
	if p.FormDataContentType == "" {
		return "binary/octet-stream"
	}
	return p.FormDataContentType
}

func (p *RawProfile) Push(profile []byte, cumulative bool) {
	p.m.Lock()
	p.RawData = nil
	if cumulative {
		p.PreviousProfile = p.Profile
	}
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
	_, _ = io.Copy(ff, bytes.NewReader(p.Profile))
	if p.PreviousProfile != nil {
		if ff, err = mw.CreateFormFile(formFieldPreviousProfile, formFilePreviousProfile); err != nil {
			return nil, err
		}
		_, _ = io.Copy(ff, bytes.NewReader(p.PreviousProfile))
	}
	if len(p.SampleTypeConfig) > 0 {
		if ff, err = mw.CreateFormFile(formFieldSampleTypeConfig, formFileSampleTypeConfig); err != nil {
			return nil, err
		}
		_ = json.NewEncoder(ff).Encode(p.SampleTypeConfig)
	}
	_ = mw.Close()
	p.RawData = b.Bytes()
	p.FormDataContentType = mw.FormDataContentType()
	return p.RawData, nil
}

func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
	p.m.Lock()
	defer p.m.Unlock()
	if p.Profile == nil && p.PreviousProfile == nil {
		if p.RawData == nil {
			return nil
		}
		if p.FormDataContentType != "" {
			if err := p.loadPprofFromForm(); err != nil {
				return err
			}
		} else {
			p.Profile = p.RawData
		}
	}
	if len(p.Profile) == 0 {
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
			if err := p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, bytes.NewReader(p.PreviousProfile)); err != nil {
				return err
			}
		}
	}

	return p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, bytes.NewReader(p.Profile))
}

func (p *RawProfile) loadPprofFromForm() error {
	boundary, err := form.ParseBoundary(p.FormDataContentType)
	if err != nil {
		return err
	}

	// maxMemory 32MB.
	// TODO(kolesnikovae): If the limit is exceeded, parts will be written
	//  to disk. It may be better to limit the request body size to be sure
	//  that they loaded into memory entirely.
	f, err := multipart.NewReader(bytes.NewReader(p.RawData), boundary).ReadForm(32 << 20)
	if err != nil {
		return err
	}
	p.Profile, err = form.ReadField(f, formFieldProfile)
	if err != nil {
		return err
	}
	p.PreviousProfile, err = form.ReadField(f, formFieldPreviousProfile)
	if err != nil {
		return err
	}

	r, err := form.ReadField(f, formFieldSampleTypeConfig)
	if err != nil || r == nil {
		return err
	}
	var config map[string]*tree.SampleTypeConfig
	if err = json.Unmarshal(r, &config); err != nil {
		return err
	}
	p.SampleTypeConfig = config
	return nil
}
