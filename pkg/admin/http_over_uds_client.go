package admin

import (
	"context"
	"net"
	"net/http"
	"time"
)

// NewHTTPOverUDSClient creates a http client that communicates via UDS
func NewHTTPOverUDSClient(socketAddr string) (*http.Client, error) {
	if socketAddr == "" {
		return nil, ErrInvalidSocketPathname
	}
	// TODO:
	// other kinds of validations?

	return &http.Client{
		// since this is an IPC call
		// this is incredibly generous
		Timeout: 500 * time.Millisecond,

		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketAddr)
			},
		},
	}, nil
}
