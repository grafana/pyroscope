package pyroscope

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"

	"github.com/grafana/pyroscope/pkg/tenant"
	httputil "github.com/grafana/pyroscope/pkg/util/http"
	"github.com/grafana/pyroscope/pkg/validation"

	"github.com/grafana/pyroscope/pkg/og/convert/speedscope"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/pkg/og/agent/types"
	"github.com/grafana/pyroscope/pkg/og/convert/jfr"
	"github.com/grafana/pyroscope/pkg/og/convert/pprof"
	"github.com/grafana/pyroscope/pkg/og/convert/profile"
	"github.com/grafana/pyroscope/pkg/og/ingestion"
	"github.com/grafana/pyroscope/pkg/og/storage/metadata"
	"github.com/grafana/pyroscope/pkg/og/util/attime"
)

// Copy-pasted from
// https://github.com/grafana/pyroscope/blob/main/pkg/server/ingest.go
// with minor changes to make it propagate http response codes.
type ingestHandler struct {
	log      log.Logger
	ingester ingestion.Ingester
}

func NewIngestHandler(l log.Logger, p ingestion.Ingester) http.Handler {
	return ingestHandler{
		log:      level.Error(l),
		ingester: p,
	}
}

func (h ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sp, ctx := opentracing.StartSpanFromContext(ctx, "ingestHandler.ServeHTTP")
	defer sp.Finish()

	tenantID, _ := tenant.ExtractTenantIDFromContext(ctx)
	sp.SetTag("tenant_id", tenantID)
	input, err := h.parseInputMetadataFromRequest(ctx, r)
	if err != nil {
		msg := "failed to parse request metadata"
		sp.LogFields(otlog.Error(err), otlog.String("msg", msg))
		_ = h.log.Log("msg", msg, "err", err, "orgID", tenantID)
		httputil.ErrorWithStatus(w, err, http.StatusBadRequest)
		return
	}

	if err := readInputRawDataFromRequest(ctx, r, input); err != nil {
		var status int
		var maxBytesError *http.MaxBytesError
		switch {
		case errors.As(err, &maxBytesError):
			err = fmt.Errorf("request body too large: %w", err)
			status = http.StatusRequestEntityTooLarge
			validation.DiscardedBytes.WithLabelValues(string(validation.BodySizeLimit), tenantID).Add(float64(maxBytesError.Limit))
			validation.DiscardedProfiles.WithLabelValues(string(validation.BodySizeLimit), tenantID).Add(float64(1))
		default:
			status = http.StatusRequestTimeout
		}

		msg := "failed to read request body"
		sp.LogFields(otlog.Error(err), otlog.String("msg", msg))
		_ = h.log.Log("msg", msg, "err", err, "orgID", tenantID)
		httputil.ErrorWithStatus(w, err, status)
		return
	}

	err = h.ingester.Ingest(ctx, input)
	if err != nil {
		if ingestion.IsIngestionError(err) {
			msg := "failed to convert profile"
			sp.LogFields(otlog.Error(err), otlog.String("msg", msg))
			_ = h.log.Log("msg", msg, "err", err, "orgID", tenantID)
			httputil.Error(w, err)
		} else {
			msg := "failed to ingest profile"
			sp.LogFields(otlog.Error(err), otlog.String("msg", msg))
			httputil.ErrorWithStatus(w, err, http.StatusUnprocessableEntity)
		}
	}
}

func (h ingestHandler) parseInputMetadataFromRequest(_ context.Context, r *http.Request) (*ingestion.IngestInput, error) {
	var (
		q     = r.URL.Query()
		input ingestion.IngestInput
		err   error
	)

	input.Metadata.LabelSet, err = labelset.Parse(q.Get("name"))
	if err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}

	if qt := q.Get("from"); qt != "" {
		input.Metadata.StartTime = attime.Parse(qt)
	} else {
		input.Metadata.StartTime = time.Now()
	}

	if qt := q.Get("until"); qt != "" {
		input.Metadata.EndTime = attime.Parse(qt)
	} else {
		input.Metadata.EndTime = time.Now()
	}

	if sr := q.Get("sampleRate"); sr != "" {
		sampleRate, err := strconv.Atoi(sr)
		if err != nil {
			_ = h.log.Log(
				"err", err,
				"msg", fmt.Sprintf("invalid sample rate: %q", sr),
			)
			input.Metadata.SampleRate = types.DefaultSampleRate
		} else {
			input.Metadata.SampleRate = uint32(sampleRate)
		}
	} else {
		input.Metadata.SampleRate = types.DefaultSampleRate
	}

	if sn := q.Get("spyName"); sn != "" {
		// TODO: error handling
		input.Metadata.SpyName = sn
	} else {
		input.Metadata.SpyName = "unknown"
	}

	if u := q.Get("units"); u != "" {
		// TODO(petethepig): add validation for these?
		input.Metadata.Units = metadata.Units(u)
	} else {
		input.Metadata.Units = metadata.SamplesUnits
	}

	if at := q.Get("aggregationType"); at != "" {
		// TODO(petethepig): add validation for these?
		input.Metadata.AggregationType = metadata.AggregationType(at)
	} else {
		input.Metadata.AggregationType = metadata.SumAggregationType
	}

	return &input, nil
}

func readInputRawDataFromRequest(ctx context.Context, r *http.Request, input *ingestion.IngestInput) error {
	var (
		sp          = opentracing.SpanFromContext(ctx)
		format      = r.URL.Query().Get("format")
		contentType = r.Header.Get("Content-Type")
	)
	if sp != nil {
		sp.SetTag("format", format)
		sp.SetTag("content_type", contentType)
	}

	buf := bytes.NewBuffer(make([]byte, 0, 64<<10))
	n, err := io.Copy(buf, r.Body)
	if err != nil {
		return fmt.Errorf("error reading request body bytes_read %d: %w", n, err)
	}

	if sp != nil {
		sp.SetTag("content_length", n)
	}
	b := buf.Bytes()

	switch {
	default:
		input.Format = ingestion.FormatGroups
	case format == "trie", contentType == "binary/octet-stream+trie":
		input.Format = ingestion.FormatTrie
	case format == "tree", contentType == "binary/octet-stream+tree":
		input.Format = ingestion.FormatTree
	case format == "lines":
		input.Format = ingestion.FormatLines

	case format == "jfr":
		input.Format = ingestion.FormatJFR
		input.Profile = &jfr.RawProfile{
			FormDataContentType: contentType,
			RawData:             b,
		}

	case format == "pprof":
		input.Format = ingestion.FormatPprof
		input.Profile = &pprof.RawProfile{
			RawData: b,
		}

	case format == "speedscope":
		input.Format = ingestion.FormatSpeedscope
		input.Profile = &speedscope.RawProfile{
			RawData: b,
		}

	case strings.Contains(contentType, "multipart/form-data"):
		input.Profile = &pprof.RawProfile{
			FormDataContentType: contentType,
			RawData:             b,
		}
	}

	if input.Profile == nil {
		input.Profile = &profile.RawProfile{
			Format:  input.Format,
			RawData: b,
		}
	}
	return nil
}
