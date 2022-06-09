package remotewrite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
)

var (
	ErrConvertPutInputToRequest = errors.New("failed to convert putInput into a http.Request")
	ErrMakingRequest            = errors.New("failed to make request")
	ErrNotOkResponse            = errors.New("response not ok")
)

type Client struct {
	log     logrus.FieldLogger
	config  config.RemoteWriteTarget
	client  *http.Client
	metrics *clientMetrics
}

func NewClient(logger logrus.FieldLogger, reg prometheus.Registerer, targetName string, cfg config.RemoteWriteTarget) *Client {
	client := &http.Client{
		Timeout: cfg.Timeout,
	}

	metrics := newClientMetrics(reg, targetName, cfg.Address)
	metrics.mustRegister()

	return &Client{
		log:     logger,
		config:  cfg,
		client:  client,
		metrics: metrics,
	}
}

func (r *Client) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	req, err := r.ingestInputToRequest(in)
	if err != nil {
		return multierror.Append(err, ErrConvertPutInputToRequest)
	}

	r.enhanceWithAuth(req)

	req = req.WithContext(ctx)

	dump, _ := httputil.DumpRequestOut(req, true)
	// ignore errors
	if err == nil {
		r.metrics.sentBytes.Add(float64(len(dump)))
	}

	start := time.Now()
	res, err := r.client.Do(req)
	if err != nil {
		return multierror.Append(err, ErrMakingRequest)
	}
	duration := time.Since(start)

	r.metrics.responseTime.With(prometheus.Labels{
		"code": strconv.FormatInt(int64(res.StatusCode), 10),
	}).Observe(duration.Seconds())
	defer res.Body.Close()

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
	if in.Format != "" {
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

	req.Header.Set("Content-Type", in.Profile.ContentType())

	return req, nil
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
