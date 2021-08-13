package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

type target struct {
	url     *url.URL
	matcher *regexp.Regexp
}

var (
	// input flags
	bindAddr    string
	logLevel    string
	targetsPath string

	// list of upstream targets
	targetsMutex sync.RWMutex
	targets      []target
)

func main() {
	flag.StringVar(&bindAddr, "bind-addr", ":4040", "bind address for http server")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.StringVar(&targetsPath, "targets-path", "./generate-targets.sh", "path to a script that generates upstream targets")
	flag.Parse()

	setupLogging()
	go updateTargets()
	startProxy()
}

func setupLogging() {
	l, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.Fatal(err)
	}
	logrus.SetLevel(l)
}

func startProxy() {
	logrus.WithFields(logrus.Fields{
		"bind-addr":    bindAddr,
		"targets-path": targetsPath,
	}).Info("config")

	err := http.ListenAndServe(bindAddr, http.HandlerFunc(handleConn))
	logrus.WithError(err).WithField("bindAddr", bindAddr).Error("error listening")
}

func handleConn(w http.ResponseWriter, r *http.Request) {
	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logrus.WithError(err).Error("failed to read body")
	}

	targetsMutex.RLock()
	targetsCopy := make([]target, len(targets))
	copy(targetsCopy, targets)
	targetsMutex.RUnlock()

	for _, t := range targetsCopy {
		appName := r.URL.Query().Get("name")
		if t.matcher.MatchString(appName) {
			logrus.WithField("target", t).Debug("uploading to upstream")
			reader := bytes.NewReader(b)

			r.URL.Scheme = t.url.Scheme
			r.URL.Host = t.url.Host

			resp, err := http.Post(r.URL.String(), r.Header.Get("Content-Type"), reader)
			logrus.WithField("resp", resp).Debug("response")
			if err != nil {
				logrus.WithError(err).WithField("target", t).Error("failed to upload to target")
			}
		}
	}
}

func updateTargets() {
	for {
		ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)
		cmd := exec.CommandContext(ctx, targetsPath)
		buf := bytes.Buffer{}
		cmd.Stdout = &buf
		err := cmd.Run()
		if err != nil {
			logrus.WithError(err).Error("failed to generate targets")
		}
		targetsMutex.Lock()
		targets = generateTargets(bytes.NewReader(buf.Bytes()))
		logrus.Debug("new targets:")
		for _, t := range targets {
			logrus.Debugf("* %s %s", t.url, t.matcher)
		}
		targetsMutex.Unlock()
		time.Sleep(10 * time.Second)
	}
}

func generateTargets(r io.Reader) []target {
	var targets []target
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		arr := strings.SplitN(line, " ", 2)
		if len(arr) < 2 {
			arr = append(arr, ".*")
		}

		url, err := url.ParseRequestURI(arr[0])
		if err != nil {
			continue
		}
		matcher, err := regexp.Compile(arr[1])
		if err != nil {
			continue
		}
		targets = append(targets, target{
			url:     url,
			matcher: matcher,
		})
	}
	return targets
}
