// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/grpcutil/carrier.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package httpgrpcutil

import (
	"errors"

	"github.com/opentracing/opentracing-go"

	"github.com/grafana/phlare/pkg/util/httpgrpc"
)

// Used to transfer trace information from/to HTTP request.
type HttpgrpcHeadersCarrier httpgrpc.HTTPRequest

func (c *HttpgrpcHeadersCarrier) Set(key, val string) {
	c.Headers = append(c.Headers, &httpgrpc.Header{
		Key:    key,
		Values: []string{val},
	})
}

func (c *HttpgrpcHeadersCarrier) ForeachKey(handler func(key, val string) error) error {
	for _, h := range c.Headers {
		for _, v := range h.Values {
			if err := handler(h.Key, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func GetParentSpanForRequest(tracer opentracing.Tracer, req *httpgrpc.HTTPRequest) (opentracing.SpanContext, error) {
	if tracer == nil {
		return nil, nil
	}

	carrier := (*HttpgrpcHeadersCarrier)(req)
	extracted, err := tracer.Extract(opentracing.HTTPHeaders, carrier)
	if errors.Is(err, opentracing.ErrSpanContextNotFound) {
		err = nil
	}
	return extracted, err
}
