package treesvg_test

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/pyroscope-io/pyroscope/pkg/convert/treesvg"
)

func TestSVG(t *testing.T) {
	b, err := ioutil.ReadFile("../../../pkg/convert/testdata/cpu.pprof")
	if err != nil {
		t.Fatal(err)
	}
	actual, err := treesvg.ToSVG(b)
	if err != nil {
		t.Fatal(err)
	}

	expected, err := ioutil.ReadFile("./testdata/out.svg")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("expected %d, got %d", len(expected), len(actual))
	}
}
