package pprof

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"sync"

	"github.com/grafana/pyroscope/pkg/og/convert/pprof/streaming"
	"github.com/grafana/pyroscope/pkg/og/stackbuilder"

	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/og/util/form"
)

type RawProfile struct {
	// parser is stateful: it holds parsed previous profile
	// which is necessary for cumulative profiles that require
	// two consecutive profiles.
	parser ParserInterface
	// References the next profile in the sequence (cumulative type only).
	next *RawProfile

	m sync.Mutex
	// Initializes lazily on Bytes, if not present.
	RawData             []byte // Represents raw request body as per ingestion API.
	FormDataContentType string // Set optionally, if RawData is multipart form.
	// Initializes lazily on Parse, if not present.
	Profile             []byte // Represents raw pprof data.
	PreviousProfile     []byte // Used for cumulative type only.
	SkipExemplars       bool
	StreamingParser     bool
	PoolStreamingParser bool
	ArenasEnabled       bool
	SampleTypeConfig    map[string]*tree.SampleTypeConfig
}

func (p *RawProfile) ContentType() string {
	if p.FormDataContentType == "" {
		return "binary/octet-stream"
	}
	return p.FormDataContentType
}

// Push loads data from profile to RawProfile making it eligible for
// Bytes and Parse calls.
//
// Returned RawProfile should be used at the next Push: the method
// established relationship between these two RawProfiles in order
// to propagate internal pprof parser state lazily on a successful
// Parse call. This is necessary for cumulative profiles that require
// two consecutive samples to calculate the diff. If parser is not
// present due to a failure, or sequence violation, the profiles will
// be re-parsed.
func (p *RawProfile) Push(profile []byte, cumulative bool) *RawProfile {
	p.m.Lock()
	p.Profile = profile
	p.RawData = nil
	n := &RawProfile{
		SampleTypeConfig: p.SampleTypeConfig,
	}
	if cumulative {
		// N.B the parser state is only propagated
		// after successful Parse call.
		n.PreviousProfile = p.Profile
		p.next = n
	}
	p.m.Unlock()
	return p.next
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
		// RawProfile was initialized with RawData or
		// Bytes has been already called.
		return p.RawData, nil
	}
	// Build multipart form.
	if len(p.Profile) == 0 && len(p.PreviousProfile) == 0 {
		return nil, nil
	}
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	ff, err := mw.CreateFormFile(formFieldProfile, formFileProfile)
	if err != nil {
		return nil, err
	}
	_, _ = io.Copy(ff, bytes.NewReader(p.Profile))
	if len(p.PreviousProfile) > 0 {
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
	cont, err := p.handleRawData()
	if err != nil || !cont {
		return err
	}

	if p.parser == nil {
		sampleTypes := p.getSampleTypes()
		if p.StreamingParser {
			config := streaming.ParserConfig{
				SpyName:     md.SpyName,
				Labels:      md.Key.Labels(),
				Putter:      putter,
				SampleTypes: sampleTypes,
				Formatter:   streaming.StackFrameFormatterForSpyName(md.SpyName),
			}
			if p.PoolStreamingParser {
				parser := streaming.VTStreamingParserFromPool(config)
				p.parser = parser
				defer func() {
					parser.ResetCache()
					parser.ReturnToPool()
					p.parser = nil
				}()
			} else {
				if p.ArenasEnabled {
					config.ArenasEnabled = true
				}
				parser := streaming.NewStreamingParser(config)
				p.parser = parser
				if p.ArenasEnabled {
					defer parser.FreeArena()
				}
			}
		} else {
			p.parser = NewParser(ParserConfig{
				SpyName:             md.SpyName,
				Labels:              md.Key.Labels(),
				Putter:              putter,
				SampleTypes:         sampleTypes,
				SkipExemplars:       p.SkipExemplars,
				StackFrameFormatter: StackFrameFormatterForSpyName(md.SpyName),
			})
		}

		if p.PreviousProfile != nil {
			// Ignore non-cumulative samples from the PreviousProfile
			// to avoid duplicates: although, presence of PreviousProfile
			// tells that there are cumulative sample types, it may also
			// include regular ones.
			cumulativeOnly := true
			if err := p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, p.PreviousProfile, cumulativeOnly); err != nil {
				return err
			}
		}
	}

	if err := p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, p.Profile, false); err != nil {
		return err
	}

	// Propagate parser to the next profile, if it is present.
	if p.next != nil {
		p.next.m.Lock()
		p.next.parser = p.parser
		p.next.m.Unlock()
	}

	return nil
}

func (p *RawProfile) ParseWithWriteBatch(c context.Context, wb stackbuilder.WriteBatchFactory, md ingestion.Metadata) error {
	cont, err := p.handleRawData()
	if err != nil || !cont {
		return err
	}
	parser := streaming.NewStreamingParser(streaming.ParserConfig{
		SpyName:       md.SpyName,
		Labels:        md.Key.Labels(),
		SampleTypes:   p.getSampleTypes(),
		Formatter:     streaming.StackFrameFormatterForSpyName(md.SpyName),
		ArenasEnabled: true,
	})
	defer parser.FreeArena()
	return parser.ParseWithWriteBatch(streaming.ParseWriteBatchInput{
		Context:   c,
		StartTime: md.StartTime, EndTime: md.EndTime,
		Profile: p.Profile, Previous: p.PreviousProfile,
		WriteBatchFactory: wb,
	})
}

func (p *RawProfile) getSampleTypes() map[string]*tree.SampleTypeConfig {
	sampleTypes := tree.DefaultSampleTypeMapping
	if p.SampleTypeConfig != nil {
		sampleTypes = p.SampleTypeConfig
	}
	return sampleTypes
}

func (p *RawProfile) handleRawData() (cont bool, err error) {
	if len(p.Profile) == 0 && len(p.PreviousProfile) == 0 {
		// Check if RawProfile was initialized with RawData.
		if p.RawData == nil {
			// Zero profile, nothing to parse.
			return false, nil
		}
		if p.FormDataContentType != "" {
			// The profile was ingested as a multipart form. loadStacktraces parts to
			// Profile, PreviousProfile, and SampleTypeConfig.
			if err := p.loadPprofFromForm(); err != nil {
				return false, err
			}
		} else {
			p.Profile = p.RawData
		}
	}
	if len(p.Profile) == 0 {
		return false, nil
	}
	return true, nil
}

func (p *RawProfile) loadPprofFromForm() error {
	boundary, err := form.ParseBoundary(p.FormDataContentType)
	if err != nil {
		return err
	}

	f, err := multipart.NewReader(bytes.NewReader(p.RawData), boundary).ReadForm(32 << 20)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.RemoveAll()
	}()

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
