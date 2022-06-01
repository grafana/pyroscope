package remotewrite

import (
	"context"
	"net/http"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/parser"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/sirupsen/logrus"
)

type Client struct {
	log    *logrus.Logger
	config RemoteWriteConfig
	client *http.Client
}

type RemoteWriteConfig struct {
	Address   string            `def:"" desc:"server that implements the pyroscope /ingest endpoint" mapstructure:"address"`
	AuthToken string            `def:"" desc:"authorization token used to upload profiling data" mapstructure:"auth-token"`
	Tags      map[string]string `name:"tag" def:"" desc:"tag in key=value form. The flag may be specified multiple times" mapstructure:"tags"`
}

func NewClient(logger *logrus.Logger, config RemoteWriteConfig) *Client {
	client := &http.Client{
		// TODO(eh-am): make timeout configurable
		Timeout: time.Second * 5,
	}

	return &Client{
		log:    logger,
		config: config,
		client: client,
	}
}

func (r *Client) Put(ctx context.Context, put parser.PutInput) {
	req, err := r.putInputToRequest(put)
	if err != nil {
		r.log.Error("Error writing putInputToRequest", err)
		return
	}

	r.log.Debugf("Making request to %s", req.URL.String())
	res, err := r.client.Do(req)
	if err != nil {
		r.log.Error("Failed to write to remote. Dropping it", err)
		return
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		// TODO(eh-am): print the error message if there's any?
		r.log.Errorf("Request to remote failed with statusCode: '%d'", res.StatusCode)
	}
}

func (r *Client) putInputToRequest(pi parser.PutInput) (*http.Request, error) {
	// TODO(eh-am): copy put.Profile?
	req, err := http.NewRequest("POST", r.config.Address, pi.Profile)
	if err != nil {
		return nil, err
	}

	params := req.URL.Query()
	params.Set("name", pi.Key.Normalized())
	//	params.Set("from", put.StartTime)
	//	params.Set("until", nil)
	//	params.Set("format", nil)
	//	params.Set("sampleRate", nil)
	//	params.Set("spyName", nil)
	//	params.Set("units", nil)
	//	params.Set("aggregationType", nil)

	req.URL.RawQuery = params.Encode()

	return req, nil
}

func (r *Client) enhanceWithTags(key *segment.Key) {
	labels := key.Labels()
	for tag, value := range r.config.Tags {
		labels[tag] = value
	}
}
