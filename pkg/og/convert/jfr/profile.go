package jfr

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	jfrPprof "github.com/grafana/jfr-parser/pprof"
	jfrPprofPyroscope "github.com/grafana/jfr-parser/pprof/pyroscope"
	distributormodel "github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/pprof"

	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/util/form"
)

type RawProfile struct {
	FormDataContentType string
	RawData             []byte
}

func (p *RawProfile) Bytes() ([]byte, error) { return p.RawData, nil }

func (p *RawProfile) ParseToPprof(_ context.Context, md ingestion.Metadata) (*distributormodel.PushRequest, error) {
	input := jfrPprof.ParseInput{
		StartTime:  md.StartTime,
		EndTime:    md.EndTime,
		SampleRate: int64(md.SampleRate),
	}

	labels := new(jfrPprof.LabelsSnapshot)
	rawSize := len(p.RawData)
	var r = p.RawData
	var err error
	if strings.Contains(p.FormDataContentType, "multipart/form-data") {
		if r, labels, err = loadJFRFromForm(r, p.FormDataContentType); err != nil {
			return nil, err
		}
	}

	profiles, err := jfrPprof.ParseJFR(r, &input, labels)
	if err != nil {
		return nil, err
	}
	res := new(distributormodel.PushRequest)
	for _, req := range profiles.Profiles {
		seriesLabels := jfrPprofPyroscope.Labels(md.Key.Labels(), profiles.JFREvent, req.Metric, md.Key.AppName(), md.SpyName)
		res.Series = append(res.Series, &distributormodel.ProfileSeries{
			Labels: seriesLabels,
			Samples: []*distributormodel.ProfileSample{
				{
					Profile: pprof.RawFromProto(req.Profile),
				},
			},
		})
	}
	res.RawProfileSize = rawSize
	res.RawProfileType = distributormodel.RawProfileTypeJFR
	return res, err
}

func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
	return fmt.Errorf("parsing to Tree/storage.Putter is no longer supported")
}

func (p *RawProfile) ContentType() string {
	if p.FormDataContentType == "" {
		return "binary/octet-stream"
	}
	return p.FormDataContentType
}

func loadJFRFromForm(r []byte, contentType string) ([]byte, *jfrPprof.LabelsSnapshot, error) {
	boundary, err := form.ParseBoundary(contentType)
	if err != nil {
		return nil, nil, err
	}

	f, err := multipart.NewReader(bytes.NewBuffer(r), boundary).ReadForm(32 << 20)
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		_ = f.RemoveAll()
	}()

	jfrField, err := form.ReadField(f, "jfr")
	if err != nil {
		return nil, nil, err
	}
	if jfrField == nil {
		return nil, nil, fmt.Errorf("jfr field is required")
	}
	jfrField, err = decompress(jfrField)
	if err != nil {
		return nil, nil, fmt.Errorf("loadJFRFromForm failed to decompress jfr: %w", err)
	}

	labelsField, err := form.ReadField(f, "labels")
	if err != nil {
		return nil, nil, err
	}

	var labels = new(jfrPprof.LabelsSnapshot)
	if len(labelsField) > 0 {
		labelsField, err = decompress(labelsField)
		if err != nil {
			return nil, nil, fmt.Errorf("loadJFRFromForm failed to decompress labels: %w", err)
		}
		if err = labels.UnmarshalVT(labelsField); err != nil {
			return nil, nil, fmt.Errorf("failed to parse labels form field: %w", err)
		}
	}

	return jfrField, labels, nil
}

func decompress(bs []byte) ([]byte, error) {
	var err error
	if len(bs) < 2 {
		return nil, fmt.Errorf("failed to read magic")
	} else if bs[0] == 0x1f && bs[1] == 0x8b {
		var gzipr *gzip.Reader
		gzipr, err = gzip.NewReader(bytes.NewReader(bs))
		defer gzipr.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read gzip header: %w", err)
		}
		buf := bytes.NewBuffer(nil)
		if _, err = io.Copy(buf, gzipr); err != nil {
			return nil, fmt.Errorf("failed to decompress jfr: %w", err)
		}
		return buf.Bytes(), nil
	} else {
		return bs, nil
	}
}
