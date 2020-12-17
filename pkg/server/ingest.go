package server

import (
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
	"github.com/sirupsen/logrus"
	log "github.com/sirupsen/logrus"
)

type ingestParams struct {
	grouped           bool
	format            string
	storageKey        *storage.Key
	samplingFrequency int
	modifiers         []string
	from              time.Time
	until             time.Time
}

func ingestParamsFromRequest(r *http.Request) *ingestParams {
	ip := &ingestParams{}
	q := r.URL.Query()
	ip.grouped = q.Get("grouped") != ""

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
	if r.Header.Get("Content-Type") == "binary/octet-stream+tree" {
		logrus.Info("ingest format = tree")
		var err error
		t, err = tree.DeserializeNoDict(r.Body)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		parserFunc := convert.ParseIndividualLines
		if ip.grouped {
			parserFunc = convert.ParseGroups
			logrus.Info("ingest format = groups")
		}

		if r.Header.Get("Content-Type") == "binary/octet-stream+trie" {
			logrus.Info("ingest format = trie")
			parserFunc = convert.ParseTrie
		}
		t = tree.New()
		parserFunc(r.Body, func(k []byte, v int) {
			t.Insert(k, uint64(v))
		})
	}

	err := ctrl.s.Put(ip.from, ip.until, ip.storageKey, t)
	if err != nil {
		log.Fatal(err)
	}
	w.WriteHeader(200)
}
