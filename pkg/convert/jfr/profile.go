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
	jfrField, err = decompress(jfrField)
	if err != nil {
		return nil, nil, fmt.Errorf("loadJFRFromForm failed to decompress jfr: %w", err)
	}

	labelsField, err := form.ReadField(f, "labels")
	if err != nil {
		return nil, nil, err
	}
	labelsField, err = decompress(labelsField)
	if err != nil {
		return nil, nil, fmt.Errorf("loadJFRFromForm failed to decompress labels: %w", err)
	}
	var labels LabelsSnapshot
	if len(labelsField) > 0 {
		if err = proto.Unmarshal(labelsField, &labels); err != nil {
			return nil, nil, err
		}
	}

	return bytes.NewReader(jfrField), &labels, nil
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
