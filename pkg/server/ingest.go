package server

import (
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/sirupsen/logrus"
)

type ingestParams struct {
	parserFunc      func(io.Reader) (*tree.Tree, error)
	storageKey      *storage.Key
	spyName         string
	sampleRate      int
	units           string
	aggregationType string
	modifiers       []string
	from            time.Time
	until           time.Time
}

func wrapConvertFunction(convertFunc func(r io.Reader, cb func(name []byte, val int)) error) func(io.Reader) (*tree.Tree, error) {
	return func(r io.Reader) (*tree.Tree, error) {
		t := tree.New()
		convertFunc(r, func(k []byte, v int) {
			t.Insert(k, uint64(v))
		})

		return t, nil
	}
}

func ingestParamsFromRequest(r *http.Request) *ingestParams {
	ip := &ingestParams{}
	q := r.URL.Query()

	format := q.Get("format")

	if format == "tree" || r.Header.Get("Content-Type") == "binary/octet-stream+tree" {
		ip.parserFunc = tree.DeserializeNoDict
	} else if format == "trie" || r.Header.Get("Content-Type") == "binary/octet-stream+trie" {
		ip.parserFunc = wrapConvertFunction(convert.ParseTrie)
	} else if format == "lines" {
		ip.parserFunc = wrapConvertFunction(convert.ParseIndividualLines)
	} else {
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
		// TODO: error handling
		ip.sampleRate, _ = strconv.Atoi(sr)
	} else {
		ip.sampleRate = 100
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
	ip.storageKey, err = storage.ParseKey(q.Get("name"))
	if err != nil {
		logrus.Error("parsing error:", err)
	}

	return ip
}

func (ctrl *Controller) ingestHandler(w http.ResponseWriter, r *http.Request) {
	ip := ingestParamsFromRequest(r)

	var t *tree.Tree
	t, err := ip.parserFunc(r.Body)
	if err != nil {
		logrus.WithField("err", err).Error("error happened while parsing data")
		return
	}

	err = ctrl.s.Put(&storage.PutInput{
		StartTime:  ip.from,
		EndTime:    ip.until,
		Key:        ip.storageKey,
		Val:        t,
		SpyName:    ip.spyName,
		SampleRate: ip.sampleRate,
		Units:      ip.units,
	})
	if err != nil {
		logrus.WithField("err", err).Error("error happened while inserting data")
		return
	}
	ctrl.statsInc("ingest")
	ctrl.statsInc("ingest:" + ip.spyName)
	k := *ip.storageKey
	ctrl.appStats.Add(hashString(k.AppName()))
	w.WriteHeader(200)
}
