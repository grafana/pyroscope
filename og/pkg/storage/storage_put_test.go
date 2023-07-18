//go:build !windows
// +build !windows

package storage

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/config"
	"github.com/pyroscope-io/pyroscope/pkg/flameql"
	"github.com/pyroscope-io/pyroscope/pkg/health"
	"github.com/pyroscope-io/pyroscope/pkg/storage/metadata"
	"github.com/pyroscope-io/pyroscope/pkg/testing"
	"github.com/pyroscope-io/pyroscope/pkg/testing/load"
	"github.com/sirupsen/logrus"
)

var _ = Describe("storage package", func() {
	logrus.SetLevel(logrus.InfoLevel)
	testing.WithConfig(func(cfg **config.Config) {
		JustBeforeEach(func() {
			var err error
			s, err = New(NewConfig(&(*cfg).Server), logrus.StandardLogger(), prometheus.NewRegistry(), new(health.Controller), NoopApplicationMetadataService{})
			Expect(err).ToNot(HaveOccurred())
		})

		writeFn := func(input load.Input) {
			Expect(s.Put(context.Background(), &PutInput{
				StartTime:       input.StartTime,
				EndTime:         input.EndTime,
				Key:             input.Key,
				Val:             input.Val,
				SpyName:         input.SpyName,
				SampleRate:      input.SampleRate,
				Units:           input.Units,
				AggregationType: input.AggregationType,
			})).ToNot(HaveOccurred())
		}

		Context("Put", func() {
			It("can be called concurrently", func() {
				defer func() {
					Expect(s.Close()).ToNot(HaveOccurred())
				}()

				seed := int(time.Now().UnixNano())
				const appName = "test.app.cpu"

				app := load.NewApp(seed, appName, load.AppConfig{
					SpyName:         "debugspy",
					SampleRate:      100,
					Units:           "samples",
					AggregationType: metadata.SumAggregationType,
					Trees:           10,
					TreeConfig: load.TreeConfig{
						MaxSymLen: 32,
						MaxDepth:  32,
						Width:     16,
					},
					Tags: []load.Tag{
						{
							Name:        "hostname",
							Cardinality: 10,
							MinLen:      16,
							MaxLen:      16,
						},
					},
				})

				suite := load.NewStorageWriteSuite(load.StorageWriteSuiteConfig{
					Seed:     seed,
					Sources:  100,
					Interval: 10 * time.Second,
					Period:   time.Minute,
					Writers:  8,
					WriteFn:  writeFn,
				})
				suite.AddApp(app)
				suite.Start()

				output, err := s.Get(context.Background(), &GetInput{
					StartTime: time.Time{},
					EndTime:   maxTime,
					Query:     &flameql.Query{AppName: appName},
				})

				Expect(err).ToNot(HaveOccurred())
				Expect(app.MergedTree().String()).To(Equal(output.Tree.String()))
				Expect(s.segments.CacheSize()).To(Equal(uint64(10)))
			})
		})
	})
})
