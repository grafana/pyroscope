package admin_test

import (
	"time"

	"github.com/pyroscope-io/pyroscope/pkg/admin"
)

func createHttpClientWithFastTimeout(socketAddr string) admin.HTTPClient {
	fastTimeout := admin.WithTimeout(time.Millisecond * 10)
	http, err := admin.NewHTTPOverUDSClient(socketAddr, fastTimeout)
	if err != nil {
		panic(err)
	}
	return http
}
