// Copyright 2024-2025 The Parca Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package debuginfo

import (
	"context"

	debuginfopb "github.com/grafana/pyroscope/pkg/parca/gen/proto/go/parca/debuginfo/v1alpha1"
)

type GrpcForwarder struct {
	debuginfopb.UnimplementedDebuginfoServiceServer
	client debuginfopb.DebuginfoServiceClient
}

func NewGRPCForwarder(client debuginfopb.DebuginfoServiceClient) *GrpcForwarder {
	return &GrpcForwarder{
		client: client,
	}
}

func (f *GrpcForwarder) ShouldInitiateUpload(ctx context.Context, req *debuginfopb.ShouldInitiateUploadRequest) (*debuginfopb.ShouldInitiateUploadResponse, error) {
	return f.client.ShouldInitiateUpload(ctx, req)
}

func (f *GrpcForwarder) InitiateUpload(ctx context.Context, req *debuginfopb.InitiateUploadRequest) (*debuginfopb.InitiateUploadResponse, error) {
	return f.client.InitiateUpload(ctx, req)
}

func (f *GrpcForwarder) MarkUploadFinished(ctx context.Context, req *debuginfopb.MarkUploadFinishedRequest) (*debuginfopb.MarkUploadFinishedResponse, error) {
	return f.client.MarkUploadFinished(ctx, req)
}
