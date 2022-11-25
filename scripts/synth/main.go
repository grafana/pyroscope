package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/jzelinskie/must"
	"github.com/pyroscope-io/pyroscope/pkg/agent/spy"
	"github.com/pyroscope-io/pyroscope/pkg/convert"
	"github.com/pyroscope-io/pyroscope/pkg/storage/tree"
	"github.com/pyroscope-io/pyroscope/pkg/synth"
)

func main() {
	if len(os.Args) < 1 {
		fmt.Println("usage: synth <pprof file>")
		os.Exit(1)
	}
	filePath := os.Args[1]

	t := tree.New()

	b := must.NotError(ioutil.ReadFile(filePath))
	pprof := must.NotError(convert.ParsePprof(bytes.NewReader(b)))
	err := pprof.Get("samples", func(labels *spy.Labels, name []byte, val int) error {
		// logrus.Info("name: ", string(name))
		t.Insert(name, uint64(val))
		return nil
	})
	if err != nil {
		panic(err)
	}

	fmt.Println(must.NotError(synth.GenerateCode(t, "ruby")))
}
