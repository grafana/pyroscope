package remotewrite

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/storage/segment"
	"github.com/sirupsen/logrus"
)

type TrafficShadower struct {
	log     *logrus.Logger
	handler http.Handler
	config  config.RemoteWrite
}

func NewTrafficShadower(logger *logrus.Logger, handler http.Handler, cfg config.RemoteWrite) *TrafficShadower {
	return &TrafficShadower{
		log:     logger,
		handler: handler,
		config:  cfg,
	}
}

// ServeHTTP requests shadows traffic to the remote server
// Then offloads to the original handler
func (t TrafficShadower) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r2, err := t.cloneRequest(r)
	if err != nil {
		t.log.Error("Failed to clone request", err)
		return
	}

	// TODO(eh-am): do this in parallel?
	t.log.Debugf("Sending to remote")
	t.sendToRemote(w, r2)

	t.log.Debugf("Sending to original")
	t.sendToOriginal(w, r)
}

func (TrafficShadower) cloneRequest(r *http.Request) (*http.Request, error) {
	// clones the request
	r2 := r.Clone(r.Context())

	// r.Clone just copies the io.Reader, which means whoever reads first wins it
	// Therefore we need to duplicate the body manually
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}

	r.Body = ioutil.NopCloser(bytes.NewReader(body))
	r2.Body = ioutil.NopCloser(bytes.NewReader(body))

	return r2, nil
}

func (t TrafficShadower) sendToRemote(_ http.ResponseWriter, r *http.Request) {
	host := t.config.Address
	token := t.config.AuthToken

	u, _ := url.Parse(host)

	r.RequestURI = ""
	r.URL.Host = u.Host
	r.URL.Scheme = u.Scheme
	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = u.Host

	// needs to happen after
	t.enhanceWithTags(r)

	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}

	client := &http.Client{
		// TODO(eh-am): make timeout configurable
		Timeout: time.Second * 5,
	}

	t.log.Debugf("Making request to %s", r.URL.String())
	res, err := client.Do(r)
	if err != nil {
		t.log.Error("Failed to shadow request. Dropping it", err)
		return
	}

	if !(res.StatusCode >= 200 && res.StatusCode < 300) {
		// TODO(eh-am): print the error message if there's any?
		t.log.Errorf("Request to remote failed with statusCode: '%d'", res.StatusCode)
	}
}

func (t TrafficShadower) sendToOriginal(w http.ResponseWriter, r *http.Request) {
	t.handler.ServeHTTP(w, r)
}

// enhanceWithTags updates the app name with the global tags
func (t TrafficShadower) enhanceWithTags(r *http.Request) {
	appName := r.URL.Query().Get("name")

	if appName == "" {
		t.log.Errorf("Expected to find queryParam 'name' but found nothing. Could not add tags to request")
		return
	}

	k, err := segment.ParseKey(appName)
	if err != nil {
		t.log.Errorf("Failed to parse key from app name. Could not add tags to request")
		return
	}

	for tag, value := range t.config.Tags {
		k.Labels()[tag] = value
	}

	logrus.Debug("enhancing with tags", k.Normalized())
	params := r.URL.Query()
	params.Set("name", k.Normalized())
	r.URL.RawQuery = params.Encode()
}
