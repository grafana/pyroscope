// Package csock implements a control socket with a simple human-readable API
package csock

import (
	"encoding/json"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	log "github.com/sirupsen/logrus"
)

type CSock struct {
	listener       net.Listener
	activeProfiles map[int]chan struct{}
	callback       func(r *Request) *Response
}

// NewCSock is a generic initializer. In most cases you want to use NewTCPCSock or NewUnixCSock.
func NewCSock(l net.Listener, cb func(r *Request) *Response) *CSock {
	sock := &CSock{
		listener:       l,
		activeProfiles: make(map[int]chan struct{}),
		callback:       cb,
	}

	return sock
}

type Request struct {
	SpyName       string `json:"spy_name"`
	ClientName    string `json:"client_name"`
	ClientVersion string `json:"client_version"`
	Command       string `json:"command"`
	Pid           int    `json:"pid"`
	ProfileID     int    `json:"profile_id"`
}

type Response struct {
	ProfileID int `json:"profile_id"`
}

func commandFromRequest(r *http.Request) string {
	s := r.URL.Path
	arr := strings.Split(s, "/")
	l := len(arr)
	if l == 0 {
		return ""
	}
	return arr[l-1]
}

func (c *CSock) ServeHTTP(rw http.ResponseWriter, r *http.Request) {
	buf, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Error(err) // TODO: handle
	}
	req := &Request{}
	json.Unmarshal(buf, &req)
	req.Command = commandFromRequest(r)
	resp := c.callback(req)
	rw.WriteHeader(200)
	b, err := json.Marshal(resp)
	if err != nil {
		log.Error(err) // TODO: handle
	}
	rw.Write(b)
}

func (c *CSock) CanonicalAddr() string {
	return c.listener.Addr().String()
}

func (c *CSock) Start() error {
	return http.Serve(c.listener, c)
}

func (c *CSock) Stop() error {
	return c.listener.Close()
}
