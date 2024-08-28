package util

import (
	"errors"

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
		var already prometheus.AlreadyRegisteredError
		ok := errors.As(err, &already)
		if ok {
			return already.ExistingCollector.(T)
		}
		panic(err)
	}
	return c
}

func Register(reg prometheus.Registerer, collectors ...prometheus.Collector) {
	if reg == nil {
		return
	}
	for _, collector := range collectors {
		err := reg.Register(collector)
		if err != nil {
			var alreadyRegisteredError prometheus.AlreadyRegisteredError
			ok := errors.As(err, &alreadyRegisteredError)
			if !ok {
				panic(err)
			}
		}
	}
}
