package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/server/httputils"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type Parser interface {
	Put(context.Context, *parser.PutInput) error
}

type ingestHandler struct {
	log       *logrus.Logger
	parser    Parser
	onSuccess func(pi *parser.PutInput)
	httpUtils httputils.Utils
}

func (ctrl *Controller) ingestHandler() http.Handler {
	p := parser.New(ctrl.log, ctrl.putter, ctrl.exporter)
	return NewIngestHandler(ctrl.log, p, func(pi *parser.PutInput) {
		ctrl.StatsInc("ingest")
		ctrl.StatsInc("ingest:" + pi.SpyName)
		ctrl.appStats.Add(hashString(pi.Key.AppName()))
	}, ctrl.httpUtils)
}

func NewIngestHandler(log *logrus.Logger, p Parser, onSuccess func(pi *parser.PutInput), httpUtils httputils.Utils) http.Handler {
	return ingestHandler{
		log:       log,
		parser:    p,
		onSuccess: onSuccess,
		httpUtils: httpUtils,
	}
}

func (h ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pi, err := h.ingestParamsFromRequest(r)
	if err != nil {
		h.httpUtils.WriteError(r, w, http.StatusBadRequest, err, "invalid parameter")
		return
	}

	err = h.parser.Put(r.Context(), pi)
	switch {
	case err == nil:
		h.onSuccess(pi)
	case storage.IsIngestionError(err):
		h.httpUtils.WriteError(r, w, http.StatusInternalServerError, err, "error happened while ingesting data")
	default:
		h.httpUtils.WriteError(r, w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
	}
}

func (h ingestHandler) ingestParamsFromRequest(r *http.Request) (*parser.PutInput, error) {
	var (
		q   = r.URL.Query()
		pi  parser.PutInput
		err error
	)

	format := q.Get("format")
	contentType := r.Header.Get("Content-Type")
	switch {
	default:
		pi.Format = parser.Groups
	case format == "trie", contentType == "binary/octet-stream+trie":
		pi.Format = parser.Trie
	case format == "tree", contentType == "binary/octet-stream+tree":
		pi.Format = parser.Tree
	case format == "lines":
		pi.Format = parser.Lines
	case format == "jfr":
		pi.Format = parser.JFR
	case format == "pprof":
		pi.Format = parser.Pprof
	case strings.Contains(contentType, "multipart/form-data"):
		pi.Format = parser.Pprof
		if err = loadPprofFromForm(&pi, r); err != nil {
			return nil, err
		}
	}

	if pi.Profile == nil {
		if err = loadProfileFromBody(&pi, r); err != nil {
			return nil, err
		}
	}

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
		// TODO(petethepig): add validation for these?
		pi.Units = metadata.Units(u)
	} else {
		pi.Units = metadata.SamplesUnits
	}

	if at := q.Get("aggregationType"); at != "" {
		// TODO(petethepig): add validation for these?
		pi.AggregationType = metadata.AggregationType(at)
	} else {
		pi.AggregationType = metadata.SumAggregationType
	}

	return &pi, nil
}

func loadProfileFromBody(pi *parser.PutInput, r *http.Request) error {
	bufferSize := int64(64 << 10) // Defaults to 64K body size
	if r.ContentLength > 0 {
		bufferSize = r.ContentLength
	}
	buf := bytes.NewBuffer(make([]byte, bufferSize))
	if _, err := io.Copy(buf, r.Body); err != nil {
		return err
	}
	pi.Profile = buf
	return nil
}

func loadPprofFromForm(pi *parser.PutInput, r *http.Request) error {
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		return err
	}
	boundary, ok := params["boundary"]
	if !ok {
		return fmt.Errorf("malformed multipart content type header")
	}
	// maxMemory 32MB.
	// TODO(kolesnikovae): If the limit is exceeded, parts will be written
	//  to disk. It may be better to limit the request body size to be sure
	//  that they loaded into memory entirely.
	form, err := multipart.NewReader(r.Body, boundary).ReadForm(32 << 20)
	if err != nil {
		return err
	}

	pi.Profile, err = formField(form, "profile")
	if err != nil {
		return err
	}
	pi.PreviousProfile, err = formField(form, "prev_profile")
	if err != nil {
		return err
	}
	pi.SampleTypeConfig, err = parseSampleTypesConfig(form)
	if err != nil {
		return err
	}
	if pi.SampleTypeConfig == nil {
		pi.SampleTypeConfig = tree.DefaultSampleTypeMapping
	}

	return nil
}

func formField(form *multipart.Form, name string) (io.ReadCloser, error) {
	files, ok := form.File[name]
	if !ok || len(files) == 0 {
		return nil, nil
	}
	f, err := files[0].Open()
	if err != nil {
		return nil, err
	}
	return f, nil
}

func parseSampleTypesConfig(form *multipart.Form) (map[string]*tree.SampleTypeConfig, error) {
	r, err := formField(form, "sample_type_config")
	if err != nil || r == nil {
		return nil, err
	}
	d := json.NewDecoder(r)
	var config map[string]*tree.SampleTypeConfig
	if err = d.Decode(&config); err != nil {
		return nil, err
	}
	return config, nil
}
