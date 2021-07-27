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
	o.exporter.Observe(i.Key, i.Val, 1)
	return nil
}
