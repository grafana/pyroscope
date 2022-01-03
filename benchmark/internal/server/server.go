package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
)

func getProfiles(name string) []tree.Profile {
	res := []tree.Profile{}

	var p tree.Profile

	paths, err := filepath.Glob("./fixtures/" + name + "-*.pprof")
	if err != nil {
		panic(err)
	}
	for _, path := range paths {
		logrus.Infof("parsing profile %s", path)
		gb, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		gr, err := gzip.NewReader(bytes.NewReader(gb))
		if err != nil {
			panic(err)
		}
		b, err := io.ReadAll(gr)
		if err != nil {
			panic(err)
		}
		if err := proto.Unmarshal(b, &p); err != nil {
			panic(err)
		}
		res = append(res, p)
	}

	return res
}

func generateHandler(profiles []tree.Profile, sleep int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		i := int(time.Now().Unix()/10) % len(profiles)
		p := profiles[i]
		gw := gzip.NewWriter(w)
		t := time.Now()
		p.TimeNanos = t.UnixNano()

		marshalled, err := proto.Marshal(&p)
		if err != nil {
			panic(err)
		}

		if sleep > 0 {
			time.Sleep(time.Duration(sleep) * time.Second)
		}

		gw.Write(marshalled)
		gw.Close()
	}
}

func StartServer() {
	m := http.NewServeMux()
	m.HandleFunc("/debug/pprof/profile", generateHandler(getProfiles("cpu"), 10))
	m.HandleFunc("/debug/pprof/heap", generateHandler(getProfiles("heap"), 0))

	s := &http.Server{
		Addr:           ":4042",
		Handler:        m,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	s.ListenAndServe()
}
