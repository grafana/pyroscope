package server_test

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/pyroscope-io/pyroscope/pkg/model/appmetadata"
	"github.com/pyroscope-io/pyroscope/pkg/server"
	"github.com/sirupsen/logrus"
)

type MockAppLister struct {
	appNames []appmetadata.ApplicationMetadata
	err      error
}

func (m MockAppLister) List(context.Context) ([]appmetadata.ApplicationMetadata, error) {
	return m.appNames, m.err
}

func TestFoo(t *testing.T) {
	mockAppLister := MockAppLister{
		appNames: []appmetadata.ApplicationMetadata{
			{FQName: "myapp.cpu"},
			{FQName: "myapp2"},
		},
		err: nil,
	}

	reg := prometheus.NewRegistry()
	// TODO: noop logger
	log := logrus.New()
	appNameMetrics := server.NewAppNameMetrics(log, time.Millisecond, reg, mockAppLister)

	// Helpers
	assertNumOfMetrics := func() {
		metricsLength := testutil.CollectAndCount(appNameMetrics.Gauge())
		if metricsLength != len(mockAppLister.appNames) {
			t.Fatalf("expected to have found '%d' metrics but found '%d'", len(mockAppLister.appNames), metricsLength)
		}
	}

	assertLabelValuePresence := func(labelValue string) {
		appLabelValue := testutil.ToFloat64(appNameMetrics.Gauge().WithLabelValues(labelValue))

		expectedAppLabelValue := 1.0
		if expectedAppLabelValue != appLabelValue {
			t.Fatalf("expected value of app=%s to be %f but found %f", labelValue, expectedAppLabelValue, appLabelValue)
		}
	}

	go func() { appNameMetrics.Start() }()
	t.Cleanup(appNameMetrics.Stop)

	// Artificial delay
	time.Sleep(time.Millisecond * 5)

	assertNumOfMetrics()
	for _, v := range mockAppLister.appNames {
		assertLabelValuePresence(v.FQName)
	}

	// Replace the first app for `myapp3`
	mockAppLister.appNames[0].FQName = "myapp3"

	time.Sleep(time.Millisecond * 5)
	for _, v := range mockAppLister.appNames {
		assertLabelValuePresence(v.FQName)
	}
	assertNumOfMetrics()

}
