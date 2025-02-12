package query_frontend

import (
	"context"
	"testing"

	"github.com/grafana/dskit/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	queryv1 "github.com/grafana/pyroscope/api/gen/proto/go/query/v1"
	"github.com/grafana/pyroscope/pkg/test/mocks/mockmetastorev1"
)

func Test_QueryFrontend_QueryMetadata(t *testing.T) {
	for _, test := range []struct {
		query    *queryv1.QueryRequest
		request  *metastorev1.QueryMetadataRequest
		response *metastorev1.QueryMetadataResponse
	}{
		{
			query: &queryv1.QueryRequest{LabelSelector: `{service_name="service-a"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{service_name="service-a"}`,
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_a"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: `{service_name!="service-a"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_a"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: `{service_name=~".*"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_c"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: `{foo="bar"}`},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_b"}},
			},
		},
		{
			query: &queryv1.QueryRequest{LabelSelector: "{}"},
			request: &metastorev1.QueryMetadataRequest{
				TenantId: []string{"org"},
				Query:    `{__tenant_dataset__="dataset_tsdb_index"}`,
				Labels:   []string{"__tenant_dataset__"},
			},
			response: &metastorev1.QueryMetadataResponse{
				Blocks: []*metastorev1.BlockMeta{{Id: "block_id_d"}},
			},
		},
	} {
		mockMetadataClient := new(mockmetastorev1.MockMetadataQueryServiceClient)
		ctx := user.InjectOrgID(context.Background(), "org")
		f := &QueryFrontend{metadataQueryClient: mockMetadataClient}

		mockMetadataClient.On("QueryMetadata", mock.Anything, test.request).
			Return(test.response, nil).
			Once()

		blocks, err := f.QueryMetadata(ctx, test.query)
		assert.NoError(t, err)
		assert.Equal(t, test.response.Blocks, blocks)
	}
}
