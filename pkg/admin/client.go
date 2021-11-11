package admin

import (
	"context"
	"net"
	"net/http"
	"time"
)

func newHttpClient(socketAddr string) http.Client {
	return http.Client{
		// since this is an IPC call
		// this is incredibly generous
		Timeout: 500 * time.Millisecond,

		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketAddr)
			},
		},
	}
}
