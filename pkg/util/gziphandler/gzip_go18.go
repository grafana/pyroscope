// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/nytimes/gziphandler/blob/2f8bb1d30d9d69c8e0c3714da5a9917125a87769/gzip_go18.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Copyright 2016-2017 The New York Times Company.
//go:build go1.8

package gziphandler

import "net/http"

// Push initiates an HTTP/2 server push.
// Push returns ErrNotSupported if the client has disabled push or if push
// is not supported on the underlying connection.
func (w *GzipResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if ok && pusher != nil {
		return pusher.Push(target, setAcceptEncodingForPushOptions(opts))
	}
	return http.ErrNotSupported
}

// setAcceptEncodingForPushOptions sets "Accept-Encoding" : "gzip" for PushOptions without overriding existing headers.
func setAcceptEncodingForPushOptions(opts *http.PushOptions) *http.PushOptions {

	if opts == nil {
		opts = &http.PushOptions{
			Header: http.Header{
				acceptEncoding: []string{"gzip"},
			},
		}
		return opts
	}

	if opts.Header == nil {
		opts.Header = http.Header{
			acceptEncoding: []string{"gzip"},
		}
		return opts
	}

	if encoding := opts.Header.Get(acceptEncoding); encoding == "" {
		opts.Header.Add(acceptEncoding, "gzip")
		return opts
	}

	return opts
}
