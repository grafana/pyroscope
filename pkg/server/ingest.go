package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/agent/types"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/structs/transporttrie"
	"github.com/pyroscope-io/pyroscope/pkg/util/attime"
)

// TODO: dedup this with the version in scrape package
type SampleTypeConfig struct {
	Units       string `json:"units,omitempty"`
	DisplayName string `json:"display-name,omitempty"`

	// TODO(kolesnikovae): Introduce Kind?
	//  In Go, we have at least the following combinations:
	//  instant:    Aggregation:avg && !Cumulative && !Sampled
	//  cumulative: Aggregation:sum && Cumulative  && !Sampled
	//  delta:      Aggregation:sum && !Cumulative && Sampled
	Aggregation string `json:"aggregation,omitempty"`
	Cumulative  bool   `json:"cumulative,omitempty"`
	Sampled     bool   `json:"sampled,omitempty"`
}

var DefaultSampleTypeMapping = map[string]*SampleTypeConfig{
	"samples": {
		DisplayName: "cpu",
		Units:       "samples",
		Sampled:     true,
	},
	"inuse_objects": {
		Units:       "objects",
		Aggregation: "avg",
	},
	"alloc_objects": {
		Units:      "objects",
		Cumulative: true,
	},
	"inuse_space": {
		Units:       "bytes",
		Aggregation: "avg",
	},
	"alloc_space": {
		Units:      "bytes",
		Cumulative: true,
	},
}

func (ctrl *Controller) ingestHandler(w http.ResponseWriter, r *http.Request) {
	pi, err := ingestParamsFromRequest(r)
	if err != nil {
		ctrl.writeInvalidParameterError(w, err)
		return
	}

	format := r.URL.Query().Get("format")
	contentType := r.Header.Get("Content-Type")
	inputs := []*storage.PutInput{}
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
	case strings.Contains(contentType, "multipart/form-data"):
		err := r.ParseMultipartForm(32 << 20) // maxMemory 32MB
		if err == nil {
			var profile *tree.Profile
			var prevProfile *tree.Profile
			f, _, err := r.FormFile("profile")
			if err == nil {
				profile, err = convert.ParsePprof(f)
			}
			f, _, err = r.FormFile("prev_profile")
			if err == nil {
				prevProfile, err = convert.ParsePprof(f)
			}

			// TODO: add error handling for all of these

			for _, sampleTypeStr := range profile.SampleTypes() {
				var t *SampleTypeConfig
				var ok bool
				if t, ok = DefaultSampleTypeMapping[sampleTypeStr]; !ok {
					continue
				}
				var tries map[string]*transporttrie.Trie
				var prevTries map[string]*transporttrie.Trie

				if profile != nil {
					tries = pprofToTries(pi.Key.Normalized(), sampleTypeStr, profile)
				}
				if prevProfile != nil {
					prevTries = pprofToTries(pi.Key.Normalized(), sampleTypeStr, prevProfile)
				}
				for trieKey, trie := range tries {
					// copy of pi
					input := *pi

					sk, _ := segment.ParseKey(trieKey)
					for k, v := range sk.Labels() {
						input.Key.Add(k, v)
					}
					suffix := sampleTypeStr
					if t.DisplayName != "" {
						suffix = t.DisplayName
					}
					input.Key = ensureKeyHasSuffix(pi.Key, "."+suffix)
					input.Val = tree.New()
					resTrie := trie
					if t.Cumulative {
						if prevTrie := prevTries[trieKey]; prevTrie != nil {
							resTrie = trie.Diff(prevTrie)
						} else {
							// TODO: error handling
							continue
						}
					}
					resTrie.Iterate(func(name []byte, val uint64) {
						input.Val.Insert(name, val)
					})
					input.Units = t.Units
					input.AggregationType = t.Aggregation
					if err = ctrl.storage.Put(&input); err != nil {
						ctrl.writeInternalServerError(w, err, "error happened while ingesting data")
						return
					}
				}
			}

		}
	default:
		err = convert.ParseGroups(r.Body, cb)
	}

	if err != nil {
		ctrl.writeError(w, http.StatusUnprocessableEntity, err, "error happened while parsing request body")
		return
	}

	if len(inputs) == 0 {
		inputs = append(inputs, pi)
	}

	for _, input := range inputs {
		if err = ctrl.storage.Put(input); err != nil {
			ctrl.writeInternalServerError(w, err, "error happened while ingesting data")
			return
		}
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

func pprofToTries(originalAppName, sampleTypeStr string, pprof *tree.Profile) map[string]*transporttrie.Trie {
	tries := map[string]*transporttrie.Trie{}

	sampleTypeConfig := DefaultSampleTypeMapping[sampleTypeStr]
	if sampleTypeConfig == nil {
		return tries
	}

	labelsCache := map[string]string{}

	// callbacks := map[*spy.Labels]func([]byte, int){}
	pprof.Get(sampleTypeStr, func(labels *spy.Labels, name []byte, val int) {
		appName := originalAppName
		if labels != nil {
			if newAppName, ok := labelsCache[labels.ID()]; ok {
				appName = newAppName
			} else if newAppName, err := mergeTagsWithAppName(appName, labels.Tags()); err == nil {
				appName = newAppName
				labelsCache[labels.ID()] = appName
			}
		}
		if _, ok := tries[appName]; !ok {
			tries[appName] = transporttrie.New()
		}
		tries[appName].Insert(name, uint64(val))
	})

	return tries
}

func ensureKeyHasSuffix(key *segment.Key, suffix string) *segment.Key {
	key = key.Clone()
	key.Add("__name__", ensureStringHasSuffix(key.AppName(), suffix))
	return key
}

func ensureStringHasSuffix(s, suffix string) string {
	if !strings.HasSuffix(s, suffix) {
		return s + suffix
	}
	return s
}

// mergeTagsWithAppName validates user input and merges explicitly specified
// tags with tags from app name.
//
// App name may be in the full form including tags (app.name{foo=bar,baz=qux}).
// Returned application name is always short, any tags that were included are
// moved to tags map. When merged with explicitly provided tags (config/CLI),
// last take precedence.
//
// App name may be an empty string. Tags must not contain reserved keys,
// the map is modified in place.
func mergeTagsWithAppName(appName string, tags map[string]string) (string, error) {
	k, err := segment.ParseKey(appName)
	if err != nil {
		return "", err
	}
	for tagKey, tagValue := range tags {
		if flameql.IsTagKeyReserved(tagKey) {
			continue
		}
		if err = flameql.ValidateTagKey(tagKey); err != nil {
			return "", err
		}
		k.Add(tagKey, tagValue)
	}
	return k.Normalized(), nil
}
