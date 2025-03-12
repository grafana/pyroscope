package pyroscope

import (
	"context"
	"testing"
	"time"

	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockpyroscope"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestPutLabelHandling(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]string
		expectedLabels map[string]string
	}{
		{
			name: "No service_name adds service_name",
			labels: map[string]string{
				"__name__": "testapp",
			},
			expectedLabels: map[string]string{
				"service_name": "testapp",
			},
		},
		{
			name: "With service_name adds app_name",
			labels: map[string]string{
				"__name__":     "testapp",
				"service_name": "custom-service",
			},
			expectedLabels: map[string]string{
				"service_name": "custom-service",
				"app_name":     "testapp",
			},
		},
		{
			name: "With service_name and app_name doesn't duplicate app_name",
			labels: map[string]string{
				"__name__":     "testapp",
				"service_name": "custom-service",
				"app_name":     "existing-app",
			},
			expectedLabels: map[string]string{
				"service_name": "custom-service",
				"app_name":     "existing-app",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := mockpyroscope.NewMockPushService(t)
			adapter := &pyroscopeIngesterAdapter{
				svc: mockService,
				log: log.NewNopLogger(),
			}

			mockService.On("Push", mock.Anything, mock.MatchedBy(func(req *connect.Request[pushv1.PushRequest]) bool {
				if len(req.Msg.Series) > 0 {
					labels := req.Msg.Series[0].Labels

					// Check expected labels
					labelMap := make(map[string]string)
					labelCounts := make(map[string]int)
					for _, label := range labels {
						labelMap[label.Name] = label.Value
						labelCounts[label.Name]++
					}

					// Verify expected values
					for key, val := range tt.expectedLabels {
						assert.Equal(t, val, labelMap[key], "Label %s should have value %s", key, val)
					}

					// Verify no duplicates
					for name, count := range labelCounts {
						assert.Equal(t, 1, count, "Label %s appears %d times, should appear exactly once", name, count)
					}
				}
				return true
			})).Return(&connect.Response[pushv1.PushResponse]{}, nil)

			ls := labelset.New(tt.labels)
			putInput := &storage.PutInput{
				LabelSet:   ls,
				StartTime:  time.Now(),
				Val:        tree.New(),
				SampleRate: 100,
				SpyName:    "testapp",
			}

			err := adapter.Put(context.Background(), putInput)
			require.NoError(t, err)
		})
	}
}
