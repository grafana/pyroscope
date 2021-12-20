package main

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

func main() {
	http.HandleFunc("/debug/pprof/profile", generateHandler("cpu", getProfile("cpu"), 10))
	http.HandleFunc("/debug/pprof/heap", generateHandler("heap", getProfile("heap"), 0))

	http.ListenAndServe(":4042", nil)
}
