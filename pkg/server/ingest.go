package server

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type Parser interface {
	Put(context.Context, *parser.PutInput) (error, error)
}

type ingestHandler struct {
	log       *logrus.Logger
	parser    Parser
	onSuccess func(pi *parser.PutInput)
}

func (ctrl *Controller) ingestHandler() http.Handler {
	p := parser.New(ctrl.log, ctrl.storage, ctrl.exporter)
	return NewIngestHandler(ctrl.log, p, func(pi *parser.PutInput) {
		ctrl.StatsInc("ingest")
		ctrl.StatsInc("ingest:" + pi.SpyName)
		ctrl.appStats.Add(hashString(pi.Key.AppName()))
	})
}

func NewIngestHandler(log *logrus.Logger, p Parser, onSuccess func(pi *parser.PutInput)) http.Handler {
	return ingestHandler{
		log:       log,
		parser:    p,
		onSuccess: onSuccess,
	}
}

func (h ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pi, err := h.ingestParamsFromRequest(r)
	if err != nil {
		WriteError(h.log, w, http.StatusBadRequest, err, "invalid parameter")
		return
	}

	// this method returns two errors to distinguish between parsing and ingestion errors
	// TODO(petethepig): maybe there's a more idiomatic way to do this?
	err, ingestErr := h.parser.Put(r.Context(), pi)

	if err != nil {
		WriteError(h.log, w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
		return
	}

	if ingestErr != nil {
		WriteError(h.log, w, http.StatusInternalServerError, err, "error happened while ingesting data")
		return
	}

	h.onSuccess(pi)
}

func (h ingestHandler) ingestParamsFromRequest(r *http.Request) (*parser.PutInput, error) {
	var (
		q   = r.URL.Query()
		pi  parser.PutInput
		err error
	)

	pi.Format = q.Get("format")
	pi.ContentType = r.Header.Get("Content-Type")
	pi.Body = r.Body
	pi.MultipartBoundary = boundaryFromRequest(r)

	pi.Key, err = segment.ParseKey(q.Get("name"))
	if err != nil {
		return nil, fmt.Errorf("name: %w", err)
	}

	if qt := q.Get("from"); qt != "" {
		pi.StartTime = attime.Parse(qt)
	} else {
		pi.StartTime = time.Now()
	}

	if qt := q.Get("until"); qt != "" {
		pi.EndTime = attime.Parse(qt)
	} else {
		pi.EndTime = time.Now()
	}

	if sr := q.Get("sampleRate"); sr != "" {
		sampleRate, err := strconv.Atoi(sr)
		if err != nil {
			h.log.WithError(err).Errorf("invalid sample rate: %q", sr)
			pi.SampleRate = types.DefaultSampleRate
		} else {
			pi.SampleRate = uint32(sampleRate)
		}
	} else {
		pi.SampleRate = types.DefaultSampleRate
	}

	if sn := q.Get("spyName"); sn != "" {
		// TODO: error handling
		pi.SpyName = sn
	} else {
		pi.SpyName = "unknown"
	}

	if u := q.Get("units"); u != "" {
		pi.Units = u
	} else {
		pi.Units = "samples"
	}

	if at := q.Get("aggregationType"); at != "" {
		pi.AggregationType = at
	} else {
		pi.AggregationType = "sum"
	}

	return &pi, nil
}

func boundaryFromRequest(r *http.Request) string {
	v := r.Header.Get("Content-Type")
	if v == "" {
		return ""
	}
	d, params, err := mime.ParseMediaType(v)
	if err != nil || !(d == "multipart/form-data") {
		return ""
	}
	boundary, ok := params["boundary"]
	if !ok {
		return ""
	}
	return boundary
}
