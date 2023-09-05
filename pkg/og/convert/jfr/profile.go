package jfr

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/golang/protobuf/proto"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"

	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/util/form"
)

type RawProfile struct {
	FormDataContentType string
	RawData             []byte
}

func (p *RawProfile) Bytes() ([]byte, error) { return p.RawData, nil }

func (p *RawProfile) ParseToPprof(ctx context.Context, md ingestion.Metadata) (*phlaremodel.PushRequest, error) {
	input := storage.PutInput{
		StartTime:       md.StartTime,
		EndTime:         md.EndTime,
		Key:             md.Key,
		SpyName:         md.SpyName,
		SampleRate:      md.SampleRate,
		Units:           md.Units,
		AggregationType: md.AggregationType,
	}

	labels := new(LabelsSnapshot)
	rawSize := len(p.RawData)
	var r = p.RawData
	var err error
	if strings.Contains(p.FormDataContentType, "multipart/form-data") {
		if r, labels, err = loadJFRFromForm(r, p.FormDataContentType); err != nil {
			return nil, err
		}
	}

	res, err := ParseJFR(r, &input, labels)
	if err != nil {
		return nil, err
	}
	res.RawProfileSize = rawSize
	res.RawProfileType = "jfr"
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

func loadJFRFromForm(r []byte, contentType string) ([]byte, *LabelsSnapshot, error) {
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

	var labels LabelsSnapshot
	if len(labelsField) > 0 {
		labelsField, err = decompress(labelsField)
		if err != nil {
			return nil, nil, fmt.Errorf("loadJFRFromForm failed to decompress labels: %w", err)
		}
		if err = proto.Unmarshal(labelsField, &labels); err != nil {
			return nil, nil, fmt.Errorf("failed to parse labels form field: %w", err)
		}
	}

	return jfrField, &labels, nil
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
