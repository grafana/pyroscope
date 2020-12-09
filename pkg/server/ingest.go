package server

import (
	"net/http"
	"time"

	"github.com/petethepig/pyroscope/pkg/storage"
	"github.com/petethepig/pyroscope/pkg/storage/tree"
	"github.com/petethepig/pyroscope/pkg/testing"
	"github.com/petethepig/pyroscope/pkg/util/attime"
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
	parserFunc := parseIndividualLines
	if ip.grouped {
		parserFunc = parseGroups
	}

	if r.Header.Get("Content-Type") == "binary/octet-stream+trie" {
		parserFunc = parseTrie
	}

	t := tree.New()

	samples := 0
	i := 0
	testing.Profile("put-"+r.URL.Query().Get("from"), func() {
		parserFunc(r.Body, func(k []byte, v int) {
			samples += v
			i++
			t.Insert(k, uint64(v))
		})

		err := ctrl.s.Put(ip.from, ip.until, ip.storageKey, t)
		if err != nil {
			log.Fatal(err)
		}
		w.WriteHeader(200)
	})
}
