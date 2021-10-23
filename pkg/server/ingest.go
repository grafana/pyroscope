package server

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

func (ctrl *Controller) ingestHandler(w http.ResponseWriter, r *http.Request) {
	pi, err := ingestParamsFromRequest(r)
	if err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	format := r.URL.Query().Get("format")
	contentType := r.Header.Get("Content-Type")
	cb := ctrl.createParseCallback(pi)
	switch {
	case format == "trie", contentType == "binary/octet-stream+trie":
		tmpBuf := ctrl.bufferPool.Get()
		defer ctrl.bufferPool.Put(tmpBuf)
		err = transporttrie.IterateRaw(r.Body, tmpBuf.B, cb)
	case format == "tree", contentType == "binary/octet-stream+tree":
		err = convert.ParseTreeNoDict(r.Body, cb)
	case format == "lines":
		err = convert.ParseIndividualLines(r.Body, cb)
	default:
		err = convert.ParseGroups(r.Body, cb)
	}

	if err != nil {
		ctrl.writeError(w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
		return
	}

	if err = ctrl.storage.Put(pi); err != nil {
		ctrl.writeInternalServerError(w, err, "error happened while ingesting data")
		return
	}

	ctrl.statsInc("ingest")
	ctrl.statsInc("ingest:" + pi.SpyName)
	ctrl.appStats.Add(hashString(pi.Key.AppName()))
}

func (ctrl *Controller) createParseCallback(pi *storage.PutInput) func([]byte, int) {
	pi.Val = tree.New()
	cb := pi.Val.InsertInt
	o, ok := ctrl.exporter.Evaluate(pi)
	if !ok {
		return cb
	}
	return func(k []byte, v int) {
		o.Observe(k, v)
		cb(k, v)
	}
}

func ingestParamsFromRequest(r *http.Request) (*storage.PutInput, error) {
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
			logrus.WithError(err).Errorf("invalid sample rate: %q", sr)
			pi.SampleRate = spy.DefaultSampleRate
		} else {
			pi.SampleRate = uint32(sampleRate)
		}
	} else {
		pi.SampleRate = spy.DefaultSampleRate
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
