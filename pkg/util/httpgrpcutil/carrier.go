// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/grpcutil/carrier.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package httpgrpcutil

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel"

	"github.com/grafana/pyroscope/pkg/util/httpgrpc"
)

// Used to transfer trace information from/to HTTP request.
type HttpgrpcHeadersCarrier httpgrpc.HTTPRequest

// Set implements propagation.TextMapCarrier for OTel.
func (c *HttpgrpcHeadersCarrier) Set(key, val string) {
	c.Headers = append(c.Headers, &httpgrpc.Header{
		Key:    key,
		Values: []string{val},
	})
}

// ForeachKey conforms to the opentracing.TextMapReader interface (kept for backward compatibility).
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

// Get implements propagation.TextMapCarrier for OTel.
func (c *HttpgrpcHeadersCarrier) Get(key string) string {
	for _, h := range c.Headers {
		if strings.EqualFold(h.Key, key) {
			return h.Values[0]
		}
	}
	return ""
}

// Keys implements propagation.TextMapCarrier for OTel.
func (c *HttpgrpcHeadersCarrier) Keys() []string {
	keys := make([]string, len(c.Headers))
	for i, h := range c.Headers {
		keys[i] = h.Key
	}
	return keys
}

// GetParentContextForRequest extracts parent trace context from HTTP request headers using OTel propagation.
func GetParentContextForRequest(req *httpgrpc.HTTPRequest) context.Context {
	carrier := (*HttpgrpcHeadersCarrier)(req)
	return otel.GetTextMapPropagator().Extract(context.Background(), carrier)
}

