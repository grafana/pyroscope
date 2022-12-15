package pprof

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/pyroscope-io/pyroscope/pkg/util/cumulativepprof"
	"io"
	"mime/multipart"
	"sync"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
)

type RawProfile struct {
	// parser is stateful: it holds parsed previous profile
	// which is necessary for cumulative profiles that require
	// two consecutive profiles.
	parser *Parser
	// References the next profile in the sequence (cumulative type only).
	next *RawProfile

	mergers *cumulativepprof.Mergers

	m sync.Mutex
	// Initializes lazily on Bytes, if not present.
	RawData             []byte // Represents raw request body as per ingestion API.
	FormDataContentType string // Set optionally, if RawData is multipart form.
	// Initializes lazily on Parse, if not present.
	Profile          []byte // Represents raw pprof data.
	PreviousProfile  []byte // Used for cumulative type only.
	SkipExemplars    bool
	SampleTypeConfig map[string]*tree.SampleTypeConfig
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
func (p *RawProfile) Push(profile []byte, cumulative, mergeCumulative bool) *RawProfile {
	p.m.Lock()
	p.Profile = profile
	p.RawData = nil
	if cumulative {
		n := &RawProfile{
			SampleTypeConfig: p.SampleTypeConfig,
		}
		// N.B the parser state is only propagated
		// after successful Parse call.
		n.PreviousProfile = p.Profile
		p.next = n
		if mergeCumulative {
			mergers := p.mergers
			if mergers == nil {
				mergers = cumulativepprof.NewMergers()
			}
			err := p.mergeCumulativeLocked(mergers)
			if err == nil {
				n.mergers = mergers
			}
		}
	}
	p.m.Unlock()
	return p.next
}

func (p *RawProfile) MergeCumulative(ms *cumulativepprof.Mergers) error {
	p.m.Lock()
	defer p.m.Unlock()
	return p.mergeCumulativeLocked(ms)
}

func (p *RawProfile) mergeCumulativeLocked(ms *cumulativepprof.Mergers) error {
	if p.Profile == nil && p.PreviousProfile == nil && p.RawData != nil && p.FormDataContentType != "" {
		err := p.loadPprofFromForm()
		if err != nil {
			return err
		}
	}
	if p.PreviousProfile == nil {
		return ErrCumulativeMergeNoPreviousProfile
	}
	merged, stConfig, err := ms.Merge(p.PreviousProfile, p.Profile, p.SampleTypeConfig)
	if err != nil {
		return err
	}
	var mergedProfileBytes bytes.Buffer
	err = merged.Write(&mergedProfileBytes)
	if err != nil {
		return err
	}
	p.Profile = mergedProfileBytes.Bytes()
	p.PreviousProfile = nil
	p.SampleTypeConfig = stConfig
	p.RawData = nil
	return nil
}

const (
	formFieldProfile, formFileProfile                   = "profile", "profile.pprof"
	formFieldPreviousProfile, formFilePreviousProfile   = "prev_profile", "profile.pprof"
	formFieldSampleTypeConfig, formFileSampleTypeConfig = "sample_type_config", "sample_type_config.json"
)
var (
	ErrCumulativeMergeNoPreviousProfile = errors.New("no previous profile for cumulative merge")
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
	if len(p.Profile) == 0 && len(p.PreviousProfile) == 0 {
		// Check if RawProfile was initialized with RawData.
		if p.RawData == nil {
			// Zero profile, nothing to parse.
			return nil
		}
		if p.FormDataContentType != "" {
			// The profile was ingested as a multipart form. Load parts to
			// Profile, PreviousProfile, and SampleTypeConfig.
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
			SpyName:             md.SpyName,
			Labels:              md.Key.Labels(),
			Putter:              putter,
			SampleTypes:         sampleTypes,
			SkipExemplars:       p.SkipExemplars,
			StackFrameFormatter: StackFrameFormatterForSpyName(md.SpyName),
		})

		if p.PreviousProfile != nil {
			// Ignore non-cumulative samples from the PreviousProfile
			// to avoid duplicates: although, presence of PreviousProfile
			// tells that there are cumulative sample types, it may also
			// include regular ones.
			filter := p.parser.sampleTypesFilter
			p.parser.sampleTypesFilter = func(s string) bool {
				if filter != nil {
					return filter(s) && sampleTypes[s].Cumulative
				}
				return sampleTypes[s].Cumulative
			}
			if err := p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, bytes.NewReader(p.PreviousProfile)); err != nil {
				return err
			}
			p.parser.sampleTypesFilter = filter
		}
	}

	if err := p.parser.ParsePprof(ctx, md.StartTime, md.EndTime, bytes.NewReader(p.Profile)); err != nil {
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
