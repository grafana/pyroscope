// AppNameMetrics periodically syncs with an ApplicationMetadata store
// And exports applications name as metrics
// The primary use case is to be able to integrate with grafana variables (https://grafana.com/docs/grafana/latest/datasources/prometheus/template-variables/)
// It uses the format `pyroscope_app{name="$APP_NAME"} 1`
package server

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"github.com/sirupsen/logrus"
)

type AppLister interface {
	List(context.Context) ([]appmetadata.ApplicationMetadata, error)
}

type AppNameMetrics struct {
	logger       *logrus.Logger
	reg          prometheus.Registerer
	stop         chan struct{}
	done         chan struct{}
	syncInterval time.Duration
	appLister    AppLister

	metrics *prometheus.GaugeVec
}

func NewAppNameMetrics(l *logrus.Logger, syncInterval time.Duration, reg prometheus.Registerer, appLister AppLister) *AppNameMetrics {
	m :=
		prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "pyroscope_apps",
				Help: "A metric with a constant '1' value labeled by name",
			},
			[]string{"name"},
		)
	reg.MustRegister(m)

	return &AppNameMetrics{
		logger:       l,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		syncInterval: syncInterval,
		appLister:    appLister,
		metrics:      m,
	}
}

// TODO: leaky, only used for tests
func (a *AppNameMetrics) Gauge() *prometheus.GaugeVec {
	return a.metrics
}

func (a *AppNameMetrics) updateMetrics(ctx context.Context) error {
	apps, err := a.appLister.List(ctx)
	if err != nil {
		return err
	}

	// Reset so that deleted apps won't show up
	a.metrics.Reset()
	for _, app := range apps {
		a.metrics.WithLabelValues(app.FQName).Set(1)
	}

	return nil
}

func (a *AppNameMetrics) Start() {
	ctx := context.Background()
	err := a.updateMetrics(ctx)
	if err != nil {
		a.logger.WithError(err).Error("failed to get app names")
	}

	defer close(a.done)
	ticker := time.NewTicker(a.syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			err = a.updateMetrics(ctx)
			if err != nil {
				a.logger.WithError(err).Error("failed to get app names")
			}
		}
	}
}

func (a *AppNameMetrics) Stop() {
	close(a.stop)
	<-a.done
}
