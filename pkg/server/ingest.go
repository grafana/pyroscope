package server

import (
	"bytes"
	"fmt"
	"github.com/go-kit/kit/log/logrus"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/convert/speedscope"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/convert/jfr"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/convert/profile"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type ingestHandler struct {
	log       log.Logger
	ingester  ingestion.Ingester
	onSuccess func(*ingestion.IngestInput)
	httpUtils httputils.ErrorUtils
}

func (ctrl *Controller) ingestHandler() http.Handler {
	return NewIngestHandler(logrus.NewLogger(ctrl.log), ctrl.ingestser, func(pi *ingestion.IngestInput) {
		ctrl.StatsInc("ingest")
		ctrl.StatsInc("ingest:" + pi.Metadata.SpyName)
		ctrl.appStats.Add(hashString(pi.Metadata.Key.AppName()))
	}, ctrl.httpUtils)
}

func NewIngestHandler(log log.Logger, p ingestion.Ingester, onSuccess func(*ingestion.IngestInput), httpUtils httputils.ErrorUtils) http.Handler {
	return ingestHandler{
		log:       log,
		ingester:  p,
		onSuccess: onSuccess,
		httpUtils: httpUtils,
	}
}

func (h ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	input, err := h.ingestInputFromRequest(r)
	if err != nil {
		h.httpUtils.WriteError(r, w, http.StatusBadRequest, err, "invalid parameter")
		return
	}

	err = h.ingester.Ingest(r.Context(), input)
	switch {
	case err == nil:
		h.onSuccess(input)
	case ingestion.IsIngestionError(err):
		h.httpUtils.WriteError(r, w, http.StatusInternalServerError, err, "error happened while ingesting data")
	default:
		h.httpUtils.WriteError(r, w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
	}
}

func (h ingestHandler) ingestInputFromRequest(r *http.Request) (*ingestion.IngestInput, error) {
	var (
		q     = r.URL.Query()
		input ingestion.IngestInput
		err   error
	)

	input.Metadata.Key, err = segment.ParseKey(q.Get("name"))
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
			_ = level.Error(h.log).Log("err", err,
				"msg", fmt.Sprintf("invalid sample rate: %q", sr))
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

	b, err := copyBody(r)
	if err != nil {
		return nil, err
	}

	format := q.Get("format")
	contentType := r.Header.Get("Content-Type")
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
			StreamingParser:     true,
			PoolStreamingParser: true,
		}
	}

	if input.Profile == nil {
		input.Profile = &profile.RawProfile{
			Format:  input.Format,
			RawData: b,
		}
	}

	return &input, nil
}

func copyBody(r *http.Request) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 64<<10))
	if _, err := io.Copy(buf, r.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
