package frontend

import (
	"context"

	"connectrpc.com/connect"

	vcsv1 "github.com/grafana/pyroscope/api/gen/proto/go/vcs/v1"
	"github.com/grafana/pyroscope/pkg/util/connectgrpc"
)

func (f *Frontend) GithubApp(ctx context.Context, req *connect.Request[vcsv1.GithubAppRequest]) (*connect.Response[vcsv1.GithubAppResponse], error) {
	return connectgrpc.RoundTripUnary[vcsv1.GithubAppRequest, vcsv1.GithubAppResponse](ctx, f, req)
}

func (f *Frontend) GithubLogin(ctx context.Context, req *connect.Request[vcsv1.GithubLoginRequest]) (*connect.Response[vcsv1.GithubLoginResponse], error) {
	return connectgrpc.RoundTripUnary[vcsv1.GithubLoginRequest, vcsv1.GithubLoginResponse](ctx, f, req)
}

func (f *Frontend) GetFile(ctx context.Context, req *connect.Request[vcsv1.GetFileRequest]) (*connect.Response[vcsv1.GetFileResponse], error) {
	return connectgrpc.RoundTripUnary[vcsv1.GetFileRequest, vcsv1.GetFileResponse](ctx, f, req)
}

func (f *Frontend) GetCommit(ctx context.Context, req *connect.Request[vcsv1.GetCommitRequest]) (*connect.Response[vcsv1.GetCommitResponse], error) {
	return connectgrpc.RoundTripUnary[vcsv1.GetCommitRequest, vcsv1.GetCommitResponse](ctx, f, req)
}
