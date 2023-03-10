package util

import (
	"github.com/prometheus/client_golang/prometheus"
)

// RegisterOrGet registers the collector c with the provided registerer.
// If the registerer is nil, the collector is returned without registration.
// If the collector is already registered, the existing collector is returned.
func RegisterOrGet[T prometheus.Collector](reg prometheus.Registerer, c T) T {
	if reg == nil {
		return c
	}
	err := reg.Register(c)
	if err != nil {
		already, ok := err.(prometheus.AlreadyRegisteredError)
		if ok {
			return already.ExistingCollector.(T)
		}
		panic(err)
	}
	return c
}
