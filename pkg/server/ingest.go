package server

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

type ingestParams struct {
	parserFunc      func(io.Reader) (*tree.Tree, error)
	storageKey      *segment.Key
	spyName         string
	sampleRate      uint32
	units           string
	aggregationType string
	modifiers       []string
	from            time.Time
	until           time.Time
}

func (ctrl *Controller) ingestHandler(w http.ResponseWriter, r *http.Request) {
	var ip ingestParams
	if err := ctrl.ingestParamsFromRequest(r, &ip); err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	var t *tree.Tree
	t, err := ip.parserFunc(r.Body)
	if err != nil {
		ctrl.writeError(w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
		return
	}

	err = ctrl.ingester.Put(&storage.PutInput{
		StartTime:       ip.from,
		EndTime:         ip.until,
		Key:             ip.storageKey,
		Val:             t,
		SpyName:         ip.spyName,
		SampleRate:      ip.sampleRate,
		Units:           ip.units,
		AggregationType: ip.aggregationType,
	})
	if err != nil {
		ctrl.writeInternalServerError(w, err, "error happened while ingesting data")
		return
	}

	ctrl.statsInc("ingest")
	ctrl.statsInc("ingest:" + ip.spyName)
	k := *ip.storageKey
	ctrl.appStats.Add(hashString(k.AppName()))
}

func (ctrl *Controller) ingestParamsFromRequest(r *http.Request, ip *ingestParams) error {
	q := r.URL.Query()
	format := q.Get("format")
	contentType := r.Header.Get("Content-Type")
	switch {
	case format == "tree", contentType == "binary/octet-stream+tree":
		ip.parserFunc = tree.DeserializeNoDict
	case format == "trie", contentType == "binary/octet-stream+trie":
		ip.parserFunc = wrapConvertFunction(convert.ParseTrie)
	case format == "lines":
		ip.parserFunc = wrapConvertFunction(convert.ParseIndividualLines)
	default:
		ip.parserFunc = wrapConvertFunction(convert.ParseGroups)
	}

	if qt := q.Get("from"); qt != "" {
		ip.from = attime.Parse(qt)
	} else {
		ip.from = time.Now()
	}

	if qt := q.Get("until"); qt != "" {
		ip.until = attime.Parse(qt)
	} else {
		ip.until = time.Now()
	}

	if sr := q.Get("sampleRate"); sr != "" {
		sampleRate, err := strconv.Atoi(sr)
		if err != nil {
			logrus.WithField("err", err).Errorf("invalid sample rate: %v", sr)
			ip.sampleRate = types.DefaultSampleRate
		} else {
			ip.sampleRate = uint32(sampleRate)
		}
	} else {
		ip.sampleRate = types.DefaultSampleRate
	}

	if sn := q.Get("spyName"); sn != "" {
		// TODO: error handling
		ip.spyName = sn
	} else {
		ip.spyName = "unknown"
	}

	if u := q.Get("units"); u != "" {
		ip.units = u
	} else {
		ip.units = "samples"
	}

	if at := q.Get("aggregationType"); at != "" {
		ip.aggregationType = at
	} else {
		ip.aggregationType = "sum"
	}

	var err error
	ip.storageKey, err = segment.ParseKey(q.Get("name"))
	if err != nil {
		return fmt.Errorf("name: %w", err)
	}
	return nil
}

func wrapConvertFunction(convertFunc func(r io.Reader, cb func(name []byte, val int)) error) func(io.Reader) (*tree.Tree, error) {
	return func(r io.Reader) (*tree.Tree, error) {
		t := tree.New()
		if err := convertFunc(r, func(k []byte, v int) {
			t.Insert(k, uint64(v))
		}); err != nil {
			return nil, err
		}
		return t, nil
	}
}
