package queryfrontend

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	querierv1 "github.com/grafana/pyroscope/api/gen/proto/go/querier/v1"
	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
	phlaremodel "github.com/grafana/pyroscope/pkg/model"
	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockfrontend"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
)

func TestQueryFrontend_ProfileTypes(t *testing.T) {
	now := time.Now()
	const tenantID = "tenant1"
	tests := []struct {
		name                 string
		request              *querierv1.ProfileTypesRequest
		expectedStart        int64
		expectedEnd          int64
		expectedProfileTypes []string
		expectedError        error
	}{
		{
			name: "success with start and end",
			request: &querierv1.ProfileTypesRequest{
				Start: now.Add(-time.Hour * 2).UnixMilli(),
				End:   now.UnixMilli(),
			},
			expectedProfileTypes: []string{
				"a:b:c:d:e:f",
				"g:h:i:j:k:l",
			},
		},
		{
			name:          "success without start and end",
			expectedStart: now.Add(-time.Hour * 1).UnixMilli(),
			expectedEnd:   now.UnixMilli(),
			request:       &querierv1.ProfileTypesRequest{},
			expectedProfileTypes: []string{
				"a:b:c:d:e:f",
				"g:h:i:j:k:l",
			},
		},
		{
			name: "failure without start time",
			request: &querierv1.ProfileTypesRequest{
				Start: 0,
				End:   now.UnixMilli(),
			},
			expectedError: errors.New("invalid_argument: missing time range in the query"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockLimits := mockfrontend.NewMockLimits(t)
			// setup limits
			mockLimits.On("MaxQueryLookback", tenantID).Return(time.Hour * 24 * 7).Maybe()
			mockLimits.On("MaxQueryLength", tenantID).Return(time.Hour * 24 * 7).Maybe()
			defer mockLimits.AssertExpectations(t)

			mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)

			// validate underlying calls using mock implmentation
			var result = &metastorev1.QueryMetadataLabelsResponse{}
			var resultErr error
			mockMetadataClient.On("QueryMetadataLabels", mock.Anything, mock.Anything).Return(result, resultErr).Run(func(args mock.Arguments) {
				req := args.Get(1).(*metastorev1.QueryMetadataLabelsRequest)

				start := tt.expectedStart
				if start == 0 {
					start = tt.request.Start
				}
				require.Equal(t, start, req.StartTime)
				end := tt.expectedEnd
				if end == 0 {
					end = tt.request.End
				}
				require.Equal(t, end, req.EndTime)

				require.Equal(t, []string{tenantID}, req.TenantId)
				require.Equal(t, "{}", req.Query)
				require.Equal(t, []string{phlaremodel.LabelNameProfileType}, req.Labels)

				result.Labels = []*typesv1.Labels{
					{
						Labels: []*typesv1.LabelPair{{
							Name:  phlaremodel.LabelNameProfileType,
							Value: "a:b:c:d:e:f",
						}},
					},
					{
						Labels: []*typesv1.LabelPair{{
							Name:  phlaremodel.LabelNameProfileType,
							Value: "g:h:i:j:k:l",
						}},
					},
				}
				resultErr = nil
			}).Maybe()
			defer mockMetadataClient.AssertExpectations(t)

			logger := log.NewNopLogger()
			qf := &QueryFrontend{
				logger:              logger,
				metadataQueryClient: mockMetadataClient,
				limits:              mockLimits,
				now:                 func() time.Time { return now },
			}

			ctx := tenant.InjectTenantID(context.Background(), tenantID)

			// Execute the method
			req := connect.NewRequest(tt.request)
			resp, err := qf.ProfileTypes(ctx, req)

			// check for expected error
			if tt.expectedError != nil {
				require.Error(t, err)
				require.Equal(t, tt.expectedError.Error(), err.Error())
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			actualProfileTypes := make([]string, 0, len(resp.Msg.ProfileTypes))
			for _, pt := range resp.Msg.ProfileTypes {
				actualProfileTypes = append(actualProfileTypes, pt.ID)
			}
			require.Equal(t, tt.expectedProfileTypes, actualProfileTypes)
		})
	}
}
