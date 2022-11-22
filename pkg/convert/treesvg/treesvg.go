package treesvg

import (
	"bytes"
	"net/http"

	"github.com/google/pprof/driver"
	"github.com/google/pprof/profile"
)

func ToSVG(in []byte) ([]byte, error) {
	p, err := profile.Parse(bytes.NewReader(in))
	if err != nil {
		return nil, err
	}
	fetchObj := &fetch{p}
	out := &bytes.Buffer{}
	writerObj := &writer{out}
	flagsetObj := &flagset{}
	symObj := &sym{}
	objObj := &obj{}
	uiObj := &ui{}

	driver.PProf(&driver.Options{
		Writer:        writerObj,
		Flagset:       flagsetObj,
		Fetch:         fetchObj,
		Sym:           symObj,
		Obj:           objObj,
		UI:            uiObj,
		HTTPServer:    nil,
		HTTPTransport: http.DefaultTransport,
	})

	return out.Bytes(), nil
}
