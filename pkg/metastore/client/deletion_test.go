package metastoreclient

import (
	"context"
	"testing"

	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/grpcclient"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"github.com/grafana/pyroscope/v2/pkg/test"
	"github.com/grafana/pyroscope/v2/pkg/test/mocks/mockdiscovery"
)

func TestDeleteDatasetsDoesNotRetrySchedulingRejections(t *testing.T) {
	for _, code := range []codes.Code{codes.ResourceExhausted, codes.FailedPrecondition} {
		t.Run(code.String(), func(t *testing.T) {
			d := mockdiscovery.NewMockDiscovery(t)
			d.On("Subscribe", mock.Anything).Return()
			logger := test.NewTestingLogger(t)
			servers := createServers([]int{30030})
			mockServers, dialOpts := createMockServers(t, logger, servers)
			t.Cleanup(mockServers.Close)
			mockServers.servers[0].compactor.On("DeleteDatasets", mock.Anything, mock.Anything).
				Return((*metastorev1.DeleteDatasetsResponse)(nil), status.Error(code, "cannot schedule deletion")).Once()
			var config grpcclient.Config
			flagext.DefaultValues(&config)
			c := New(logger, config, d, dialOpts...)
			c.updateServers(servers)

			resp, err := c.DeleteDatasets(context.Background(), &metastorev1.DeleteDatasetsRequest{})
			require.Nil(t, resp)
			require.Equal(t, code, status.Code(err))
		})
	}
}
