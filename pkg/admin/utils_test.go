package admin_test

import (
	"github.com/pyroscope-io/pyroscope/pkg/admin"
	"time"
)

func createHttpClientWithFastTimeout(socketAddr string) admin.HTTPClient {
	fastTimeout := admin.WithTimeout(time.Millisecond * 1)
	http, err := admin.NewHTTPOverUDSClient(socketAddr, fastTimeout)
	if err != nil {
		panic(err)
	}
	return http
}
