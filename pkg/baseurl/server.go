package baseurl

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/sirupsen/logrus"
)

var upstream string
var baseURL string
var listenAddr string

// newProxy takes target host and creates a reverse proxy
func newProxy(targetHost string) (http.Handler, error) {
	target, err := url.Parse(targetHost)
	if err != nil {
		return nil, err
	}

	p := httputil.NewSingleHostReverseProxy(target)
	p.Director = func(r *http.Request) {
		logrus.Info("before: ", r.URL.String())
		r.URL.Scheme = target.Scheme
		r.URL.Host = target.Host
		logrus.Info("target.Host", target.Host)
		r.URL.Path = strings.ReplaceAll(r.URL.Path, target.Path, "")
		r.URL.RawPath = strings.ReplaceAll(r.URL.RawPath, target.Path, "")
		logrus.Info("after: ", r.URL.String())
	}

	f := func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, target.Path) {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(fmt.Sprintf("Please visit pyroscope at %s", target.Path)))
			return
		}
		p.ServeHTTP(w, r)
	}
	return http.HandlerFunc(f), nil
}

func Start(cfg *config.Server) {
	proxy, err := newProxy("http://localhost" + cfg.APIBindAddr + cfg.BaseURL)

	if err != nil {
		panic(err)
	}

	logger := logrus.New()
	w := logger.Writer()
	server := &http.Server{
		Addr:           cfg.BaseURLBindAddr,
		Handler:        proxy,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   15 * time.Second,
		IdleTimeout:    30 * time.Second,
		MaxHeaderBytes: 1 << 20,

		ErrorLog: log.New(w, "", 0),
	}

	server.ListenAndServe()
}
