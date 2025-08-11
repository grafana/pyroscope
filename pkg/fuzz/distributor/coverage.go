package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/coverage"
	"time"
)

const (
	metadataPath = "covmeta.data"
	countersPath = "covcounters.data.1.1"
)

// periodically dump coverage counters to file
func DumpCoverage(ctx context.Context) {
	if err := dumpCoverageMetadata(ctx); err != nil {
		log.Printf("Failed to write coverage metadata: %+v", err)
	}

	ticker := time.NewTicker(10 * time.Second)
	for {
		select {
		case <-ctx.Done():
			log.Printf("Coverage dumping has been stopped due to context cancellation")
			return
		case <-ticker.C:
			if err := dumpCoverageCounters(ctx); err != nil {
				log.Printf("Failed to write coverage counters: %v", err)
			}
		}
	}
}

// writes coverage metadata to file
func dumpCoverageMetadata(ctx context.Context) error {
	f, err := os.OpenFile(metadataPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open meta file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("Failed to close meta file: %+v", err)
		}
	}()

	if err := coverage.WriteMeta(f); err != nil {
		return fmt.Errorf("write meta data: %w", err)
	}
	return nil
}

// write coverage conters to file
func dumpCoverageCounters(ctx context.Context) error {
	f, err := os.OpenFile(countersPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open counters file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Printf("Failed to close counters file: %+v", err)
		}
	}()

	if err := coverage.WriteCounters(f); err != nil {
		return fmt.Errorf("write counters data: %w", err)
	}
	return nil
}
