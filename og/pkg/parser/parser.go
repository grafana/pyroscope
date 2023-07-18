// Package parser deals with parsing various incoming formats
package parser

import (
	"context"

	"github.com/sirupsen/logrus"

	"github.com/pyroscope-io/pyroscope/pkg/ingestion"
	"github.com/pyroscope-io/pyroscope/pkg/storage"
)

type Parser struct {
	log      *logrus.Logger
	putter   storage.Putter
	exporter storage.MetricsExporter
}

func New(log *logrus.Logger, s storage.Putter, exporter storage.MetricsExporter) *Parser {
	return &Parser{
		log:      log,
		putter:   s,
		exporter: exporter,
	}
}

func (p *Parser) Ingest(ctx context.Context, in *ingestion.IngestInput) error {
	updateMetrics(in)
	return in.Profile.Parse(ctx, p.putter, p.exporter, in.Metadata)
}
