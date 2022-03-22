package server

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/valyala/bytebufferpool"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/convert/jfr"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type ingestHandler struct {
	log        *logrus.Logger
	storage    *storage.Storage
	exporter   storage.MetricsExporter
	bufferPool *bytebufferpool.Pool
	onSuccess  func(pi *storage.PutInput)
}

func NewIngestHandler(log *logrus.Logger, st *storage.Storage, exporter storage.MetricsExporter, onSuccess func(pi *storage.PutInput)) http.Handler {
	return ingestHandler{
		log:        log,
		storage:    st,
		exporter:   exporter,
		bufferPool: &bytebufferpool.Pool{},
		onSuccess:  onSuccess,
	}
}

func (h ingestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pi, err := h.ingestParamsFromRequest(r)
	if err != nil {
		WriteError(h.log, w, http.StatusBadRequest, err, "invalid parameter")
		return
	}

	format := r.URL.Query().Get("format")
	contentType := r.Header.Get("Content-Type")
	cb := h.createParseCallback(pi)
	switch {
	case format == "trie", contentType == "binary/octet-stream+trie":
		tmpBuf := h.bufferPool.Get()
		defer h.bufferPool.Put(tmpBuf)
		err = transporttrie.IterateRaw(r.Body, tmpBuf.B, cb)
	case format == "tree", contentType == "binary/octet-stream+tree":
		err = convert.ParseTreeNoDict(r.Body, cb)
	case format == "lines":
		err = convert.ParseIndividualLines(r.Body, cb)
	case format == "jfr":
		err = jfr.ParseJFR(r.Body, h.storage, pi)
	case strings.Contains(contentType, "multipart/form-data"):
		err = writePprof(h.storage, pi, r)
	default:
		err = convert.ParseGroups(r.Body, cb)
	}

	if err != nil {
		WriteError(h.log, w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
		return
	}

	if pi.Val != nil {
		if err = h.storage.Put(pi); err != nil {
			WriteError(h.log, w, http.StatusInternalServerError, err, "error happened while ingesting data")
			return
		}
	}

	h.onSuccess(pi)
}

// revive:enable:cognitive-complexity
func (h ingestHandler) createParseCallback(pi *storage.PutInput) func([]byte, int) {
	pi.Val = tree.New()
	cb := pi.Val.InsertInt
	o, ok := h.exporter.Evaluate(pi)
	if !ok {
		return cb
	}
	return func(k []byte, v int) {
		o.Observe(k, v)
		cb(k, v)
	}
}

func (h ingestHandler) ingestParamsFromRequest(r *http.Request) (*storage.PutInput, error) {
	var (
		q   = r.URL.Query()
		pi  storage.PutInput
		err error
	)

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

func writePprof(s *storage.Storage, pi *storage.PutInput, r *http.Request) error {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		// maxMemory 32MB
		return err
	}
	w := pprof.NewProfileWriter(s, pi.Key.Labels(), tree.DefaultSampleTypeMapping)
	if err := writePprofFromForm(r, w, pi, "prev_profile"); err != nil {
		return err
	}
	return writePprofFromForm(r, w, pi, "profile")
}

func writePprofFromForm(r *http.Request, w *pprof.ProfileWriter, pi *storage.PutInput, name string) error {
	f, _, err := r.FormFile(name)
	switch {
	case err == nil:
	case errors.Is(err, http.ErrMissingFile):
		return nil
	default:
		return err
	}
	return pprof.DecodePool(f, func(p *tree.Profile) error {
		return w.WriteProfile(pi.StartTime, pi.EndTime, pi.SpyName, p)
	})
}
