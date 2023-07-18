package admin

import (
	"context"
	"net"
	"net/http"
	"time"
)

type ClientOption func(*http.Client)

// NewHTTPOverUDSClient creates a http client that communicates via UDS (unix domain sockets)
func NewHTTPOverUDSClient(socketAddr string, opts ...ClientOption) (*http.Client, error) {
	if socketAddr == "" {
		return nil, ErrInvalidSocketPathname
	}
	// TODO:
	// other kinds of validations?

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketAddr)
			},
		},
	}

	for _, opt := range opts {
		opt(client)
	}

	return client, nil
}

func WithTimeout(d time.Duration) ClientOption {
	return func(c *http.Client) {
		c.Timeout = d
	}
}
