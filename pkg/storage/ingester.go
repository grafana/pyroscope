package storage

import (
	"github.com/pyroscope-io/pyroscope/pkg/exporter"
)

type Ingester interface {
	Put(*PutInput) error
}

type IngestionObserver struct {
	storage  *Storage
	exporter *exporter.MetricsExporter
}

func NewIngestionObserver(storage *Storage, exporter *exporter.MetricsExporter) IngestionObserver {
	return IngestionObserver{
		storage:  storage,
		exporter: exporter,
	}
}

func (o IngestionObserver) Put(i *PutInput) error {
	if err := o.storage.Put(i); err != nil {
		return err
	}
	var m float64 = 1
	if i.Units == "" {
		// Sample duration in nanoseconds.
		m = 1e9 / float64(i.SampleRate)
	}
	o.exporter.Observe(i.Key, i.Val, m)
	return nil
}
