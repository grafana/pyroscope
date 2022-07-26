package server

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

type RemoteReadHandler struct {
	cfg   config.RemoteRead
	proxy http.Handler
	url   *url.URL
}

func (*Controller) remoteReadHandler(r config.RemoteRead) (http.HandlerFunc, error) {
	f, err := NewRemoteReadHandler(r)
	if err != nil {
		return nil, err
	}
	return f.ServeHTTP, nil
}

func NewRemoteReadHandler(r config.RemoteRead) (*RemoteReadHandler, error) {
	u, err := url.Parse(r.Address)
	if err != nil {
		return nil, err
	}

	p, err := newProxy(r.Address, r.AuthToken)
	if err != nil {
		return nil, err
	}

	return &RemoteReadHandler{
		cfg:   r,
		proxy: p,
		url:   u,
	}, nil
}

func (rh *RemoteReadHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rh.proxy.ServeHTTP(w, r)
}

// newProxy takes target host and creates a reverse proxy
func newProxy(targetAddress, token string) (http.Handler, error) {
	target, err := url.Parse(targetAddress)
	if err != nil {
		return nil, err
	}

	p := httputil.NewSingleHostReverseProxy(target)
	p.ErrorHandler = func(rw http.ResponseWriter, req *http.Request, err error) {
		logrus.WithError(err).Error("error proxying request")
	}

	p.Director = func(r *http.Request) {
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		r.Host = target.Host
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Cookie", "")
	}

	f := func(w http.ResponseWriter, r *http.Request) {
		p.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f), nil
}
