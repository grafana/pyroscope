package pyroscope

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	pushv1 "github.com/grafana/pyroscope/api/gen/proto/go/push/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	"github.com/grafana/pyroscope/api/model/labelset"
	"github.com/grafana/pyroscope/pkg/distributor/model"
	"github.com/grafana/pyroscope/pkg/og/storage"
	"github.com/grafana/pyroscope/pkg/og/storage/tree"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockLabelCapturingPushService struct {
	Labels []*typesv1.LabelPair
}

func (m *MockLabelCapturingPushService) Push(ctx context.Context, req *connect.Request[pushv1.PushRequest]) (*connect.Response[pushv1.PushResponse], error) {
	if len(req.Msg.Series) > 0 {
		m.Labels = req.Msg.Series[0].Labels
	}
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

func (m *MockLabelCapturingPushService) PushParsed(ctx context.Context, req *model.PushRequest) (*connect.Response[pushv1.PushResponse], error) {
	if len(req.Series) > 0 {
		m.Labels = req.Series[0].Labels
	}
	return connect.NewResponse(&pushv1.PushResponse{}), nil
}

func TestPutLabelHandling(t *testing.T) {
	tests := []struct {
		name           string
		labels         map[string]string // Initial labels to set
		expectedLabels map[string]string // Expected labels after processing
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
			// Setup mock service
			mockService := &MockLabelCapturingPushService{}
			adapter := &pyroscopeIngesterAdapter{
				svc: mockService,
				log: log.NewNopLogger(),
			}

			// Create a minimal PutInput
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

			labelMap := make(map[string]string)
			for _, label := range mockService.Labels {
				labelMap[label.Name] = label.Value
			}

			// Check expected labels exist with correct values
			for key, val := range tt.expectedLabels {
				assert.Equal(t, val, labelMap[key], "Label %s should have value %s", key, val)
			}

			// Check for duplicates
			labelCounts := make(map[string]int)
			for _, label := range mockService.Labels {
				labelCounts[label.Name]++
			}
			for name, count := range labelCounts {
				assert.Equal(t, 1, count, "Label %s appears %d times, should appear exactly once", name, count)
			}
		})
	}
}
