package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"google.golang.org/protobuf/proto"
)

func getProfile(name string) tree.Profile {
	var p tree.Profile

	gb, err := os.ReadFile("./fixtures/" + name + ".pprof.gz")
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

	return p
}

func generateHandler(_ string, p tree.Profile, sleep int) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
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
	m.HandleFunc("/debug/pprof/profile", generateHandler("cpu", getProfile("cpu"), 10))
	m.HandleFunc("/debug/pprof/heap", generateHandler("heap", getProfile("heap"), 0))

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
