package jfr

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/golang/protobuf/proto"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/util/form"
)

type RawProfile struct {
	FormDataContentType string
	RawData             []byte
}

func (p *RawProfile) Bytes() ([]byte, error) { return p.RawData, nil }

func (p *RawProfile) Parse(ctx context.Context, putter storage.Putter, _ storage.MetricsExporter, md ingestion.Metadata) error {
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
	var r io.Reader = bytes.NewReader(p.RawData)
	var err error
	if strings.Contains(p.FormDataContentType, "multipart/form-data") {
		if r, labels, err = loadJFRFromForm(r, p.FormDataContentType); err != nil {
			return err
		}
	}

	return ParseJFR(ctx, putter, r, &input, labels)
}

func (p *RawProfile) ContentType() string {
	if p.FormDataContentType == "" {
		return "binary/octet-stream"
	}
	return p.FormDataContentType
}

func loadJFRFromForm(r io.Reader, contentType string) (io.Reader, *LabelsSnapshot, error) {
	boundary, err := form.ParseBoundary(contentType)
	if err != nil {
		return nil, nil, err
	}

	f, err := multipart.NewReader(r, boundary).ReadForm(32 << 20)
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

	labelsField, err := form.ReadField(f, "labels")
	if err != nil {
		return nil, nil, err
	}
	var labels LabelsSnapshot
	if len(labelsField) > 0 {
		if err = proto.Unmarshal(labelsField, &labels); err != nil {
			return nil, nil, err
		}
	}

	return bytes.NewReader(jfrField), &labels, nil
}
