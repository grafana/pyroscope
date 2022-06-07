package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/convert/pprof"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

var (
	ErrConvertPutInputToRequest = errors.New("failed to convert putInput into a http.Request")
	ErrMakingRequest            = errors.New("failed to make request")
	ErrNotOkResponse            = errors.New("response not ok")
)

type Client struct {
	log    *logrus.Logger
	config config.RemoteWrite
	client *http.Client
}

func NewClient(logger *logrus.Logger, cfg config.RemoteWrite) *Client {
	client := &http.Client{
		// TODO(eh-am): make timeout configurable
		Timeout: time.Second * 15,
	}

	return &Client{
		log:    logger,
		config: cfg,
		client: client,
	}
}

func (r *Client) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	req, err := r.ingestInputToRequest(in)
	if err != nil {
		return multierror.Append(err, ErrConvertPutInputToRequest)
	}

	r.enhanceWithAuth(req)

	req = req.WithContext(ctx)
	r.log.Debugf("Making request to %s", req.URL.String())
	res, err := r.client.Do(req)
	if err != nil {
		return multierror.Append(err, ErrMakingRequest)
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		// read all the response body
		respBody, _ := ioutil.ReadAll(res.Body)
		return multierror.Append(ErrNotOkResponse, fmt.Errorf("status code: '%d'. body: '%s'", res.StatusCode, respBody))
	}

	return nil
}

func (r *Client) ingestInputToRequest(in *ingestion.IngestInput) (*http.Request, error) {
	b, err := in.Profile.Bytes()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", r.config.Address+"/ingest", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}

	r.enhanceWithTags(in.Metadata.Key)

	params := req.URL.Query()
	contentType := "binary/octet-stream"
	if p, ok := in.Profile.(*pprof.RawProfile); ok && p.Boundary != "" {
		contentType = multipartContentType(p.Boundary)
	} else {
		params.Set("format", string(in.Format))
	}

	params.Set("name", in.Metadata.Key.Normalized())
	params.Set("from", strconv.FormatInt(in.Metadata.StartTime.Unix(), 10))
	params.Set("until", strconv.FormatInt(in.Metadata.EndTime.Unix(), 10))
	params.Set("sampleRate", strconv.FormatUint(uint64(in.Metadata.SampleRate), 10))
	params.Set("spyName", in.Metadata.SpyName)
	params.Set("units", in.Metadata.Units.String())
	params.Set("aggregationType", in.Metadata.AggregationType.String())
	req.URL.RawQuery = params.Encode()

	req.Header.Set("Content-Type", contentType)

	return req, nil
}

func multipartContentType(b string) string {
	// We must quote the boundary if it contains any of the
	// tspecials characters defined by RFC 2045, or space.
	if strings.ContainsAny(b, `()<>@,;:\"/[]?= `) {
		b = `"` + b + `"`
	}
	return "multipart/form-data; boundary=" + b
}

func (r *Client) enhanceWithTags(key *segment.Key) {
	labels := key.Labels()
	for tag, value := range r.config.Tags {
		labels[tag] = value
	}
}

func (r *Client) enhanceWithAuth(req *http.Request) {
	token := r.config.AuthToken

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}
