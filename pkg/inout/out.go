package inout

import (
	"net/http"
	"strconv"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
)

func (io InOut) RequestFromPutInput(pi *parser.PutInput, address string) (*http.Request, error) {
	// Clone the PutInput, since other people may try to read its fields (eg profile)
	pi = pi.Clone()

	body, contentType, err := io.bodyCreator.Create(pi)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", address, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", contentType)

	params := req.URL.Query()
	params.Set("name", pi.Key.Normalized())
	params.Set("from", strconv.FormatInt(pi.StartTime.Unix(), 10))
	params.Set("until", strconv.FormatInt(pi.EndTime.Unix(), 10))
	params.Set("sampleRate", strconv.FormatUint(uint64(pi.SampleRate), 10))
	params.Set("spyName", pi.SpyName)
	params.Set("units", pi.Units.String())
	params.Set("aggregationType", pi.AggregationType.String())

	// This is dumb, but the ingester expects:
	// all parsers expect pprof to set a format
	// when format=='pprof' and a previousProfile is set, use multipart and NOT set a format
	if pi.Format != parser.Pprof || (pi.Format == parser.Pprof && pi.PreviousProfile == nil) {
		params.Set("format", pi.Format.String())
	}

	req.URL.RawQuery = params.Encode()

	return req, nil
}
