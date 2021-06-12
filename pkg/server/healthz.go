package server

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

func (ctrl *Controller) resolveAddress() (string, error) {
	parts := strings.Split(ctrl.cfg.APIBindAddr, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid bind address: %v", ctrl.cfg.APIBindAddr)
	}
	port := parts[1]

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addrs {
		if inet, ok := address.(*net.IPNet); ok && !inet.IP.IsLoopback() {
			if inet.IP.To4() != nil {
				return fmt.Sprintf("%s:%s", inet.IP.String(), port), nil
			}
		}
	}
	return "", errors.New("ip not found")
}

func (ctrl *Controller) healthz(w http.ResponseWriter, r *http.Request) {
	if ctrl.endpoint == "" {
		// resolve the host and port
		address, err := ctrl.resolveAddress()
		if err != nil {
			w.WriteHeader(503)
			w.Write([]byte(fmt.Sprintf("resolve address: %v", err)))
			return
		}
		ctrl.endpoint = fmt.Sprintf("http://%s/funny", address)
	}

	res, err := http.Get(ctrl.endpoint)
	if err != nil {
		w.WriteHeader(503)
		w.Write([]byte(fmt.Sprintf("http Get %v: %v", ctrl.endpoint, err)))
		return
	}
	if res.StatusCode == http.StatusNotFound {
		w.WriteHeader(200)
		w.Write([]byte("server is ready"))
	} else {
		w.WriteHeader(503)
		w.Write([]byte("server is not ready"))
	}
}
