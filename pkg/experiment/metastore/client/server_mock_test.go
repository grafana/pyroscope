package metastoreclient

import (
	"context"
	compactorv1 "github.com/grafana/pyroscope/api/gen/proto/go/compactor/v1"
	metastorev1 "github.com/grafana/pyroscope/api/gen/proto/go/metastore/v1"
	"google.golang.org/grpc"
)

//
//type mockServer struct {
//	metastore *mockmetastorev1.MockMetastoreServiceServer
//	compactor *mockcompactorv1.MockCompactionPlannerServer
//	metastorev1.UnsafeMetastoreServiceServer
//	compactorv1.UnsafeCompactionPlannerServer
//	srv *grpc.Server
//}
//
//func (m mockServer) PollCompactionJobs(ctx context.Context, request *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
//	return m.compactor.PollCompactionJobs(ctx, request)
//}
//
//func (m mockServer) GetCompactionJobs(ctx context.Context, request *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
//	return m.compactor.GetCompactionJobs(ctx, request)
//}
//
//func (m mockServer) AddBlock(ctx context.Context, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
//	return m.metastore.AddBlock(ctx, request)
//}
//
//func (m mockServer) QueryMetadata(ctx context.Context, request *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
//	return m.metastore.QueryMetadata(ctx, request)
//}
//
//func (m mockServer) ReadIndex(ctx context.Context, request *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
//	return m.metastore.ReadIndex(ctx, request)
//}

type mockServer struct {
	metastorev1.UnsafeMetastoreServiceServer
	compactorv1.UnsafeCompactionPlannerServer
	srv *grpc.Server

	pollCompactionJobs func(ctx context.Context, request *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error)
	getCompactionJobs  func(ctx context.Context, request *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error)
	addBlock           func(ctx context.Context, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error)
	queryMetadata      func(ctx context.Context, request *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error)
	readIndex          func(ctx context.Context, request *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error)
}

func (m mockServer) PollCompactionJobs(ctx context.Context, request *compactorv1.PollCompactionJobsRequest) (*compactorv1.PollCompactionJobsResponse, error) {
	return m.pollCompactionJobs(ctx, request)
}

func (m mockServer) GetCompactionJobs(ctx context.Context, request *compactorv1.GetCompactionRequest) (*compactorv1.GetCompactionResponse, error) {
	return m.getCompactionJobs(ctx, request)
}

func (m mockServer) AddBlock(ctx context.Context, request *metastorev1.AddBlockRequest) (*metastorev1.AddBlockResponse, error) {
	return m.addBlock(ctx, request)
}

func (m mockServer) QueryMetadata(ctx context.Context, request *metastorev1.QueryMetadataRequest) (*metastorev1.QueryMetadataResponse, error) {
	return m.queryMetadata(ctx, request)
}

func (m mockServer) ReadIndex(ctx context.Context, request *metastorev1.ReadIndexRequest) (*metastorev1.ReadIndexResponse, error) {
	return m.readIndex(ctx, request)
}
