// Copyright 2022-2025 The Parca Authors
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
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"buf.build/gen/go/parca-dev/parca/grpc/go/parca/debuginfo/v1alpha1/debuginfov1alpha1grpc"
	debuginfopb "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"
)

var ErrDebuginfoAlreadyExists = errors.New("debug info already exists")

const (
	// ChunkSize 8MB is the size of the chunks in which debuginfo files are
	// uploaded and downloaded. AWS S3 has a minimum of 5MB for multi-part uploads
	// and a maximum of 15MB, and a default of 8MB.
	ChunkSize = 1024 * 1024 * 8
	// MaxMsgSize is the maximum message size the server can receive or send. By default, it is 64MB.
	MaxMsgSize = 1024 * 1024 * 64
)

type GrpcDebuginfoUploadServiceClient interface {
	Upload(ctx context.Context, opts ...grpc.CallOption) (debuginfov1alpha1grpc.DebuginfoService_UploadClient, error)
}

type GrpcUploadClient struct {
	GrpcDebuginfoUploadServiceClient
}

func NewGrpcUploadClient(client GrpcDebuginfoUploadServiceClient) *GrpcUploadClient {
	return &GrpcUploadClient{client}
}

func (c *GrpcUploadClient) Upload(ctx context.Context, uploadInstructions *debuginfopb.UploadInstructions, r io.Reader) (uint64, error) {
	return c.grpcUpload(ctx, uploadInstructions, r)
}

func (c *GrpcUploadClient) grpcUpload(ctx context.Context, uploadInstructions *debuginfopb.UploadInstructions, r io.Reader) (uint64, error) {
	stream, err := c.GrpcDebuginfoUploadServiceClient.Upload(ctx, grpc.MaxCallSendMsgSize(MaxMsgSize))
	if err != nil {
		return 0, fmt.Errorf("initiate upload: %w", err)
	}

	err = stream.Send(&debuginfopb.UploadRequest{
		Data: &debuginfopb.UploadRequest_Info{
			Info: &debuginfopb.UploadInfo{
				UploadId: uploadInstructions.UploadId,
				BuildId:  uploadInstructions.BuildId,
				Type:     uploadInstructions.Type,
			},
		},
	})
	if err != nil {
		if err := sentinelError(err); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("send upload info: %w", err)
	}

	reader := bufio.NewReader(r)

	buffer := make([]byte, ChunkSize)

	bytesSent := 0
	for {
		n, err := reader.Read(buffer)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("read next chunk (%d bytes sent so far): %w", bytesSent, err)
		}

		err = stream.Send(&debuginfopb.UploadRequest{
			Data: &debuginfopb.UploadRequest_ChunkData{
				ChunkData: buffer[:n],
			},
		})
		bytesSent += n
		if errors.Is(err, io.EOF) {
			// When the stream is closed, the server will send an EOF.
			// To get the correct error code, we need the status.
			// So receive the message and check the status.
			err = stream.RecvMsg(nil)
			if err := sentinelError(err); err != nil {
				return 0, err
			}
			return 0, fmt.Errorf("send chunk: %w", err)
		}
		if err != nil {
			return 0, fmt.Errorf("send next chunk (%d bytes sent so far): %w", bytesSent, err)
		}
	}

	// It returns io.EOF when the stream completes successfully.
	res, err := stream.CloseAndRecv()
	if errors.Is(err, io.EOF) {
		return res.Size, nil
	}
	if err != nil {
		// On any other error, the stream is aborted and the error contains the RPC status.
		if err := sentinelError(err); err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("close and receive: %w", err)
	}
	return res.Size, nil
}

// sentinelError checks underlying error for grpc.StatusCode and returns if it's a known and expected error.
func sentinelError(err error) error {
	if sts, ok := status.FromError(err); ok {
		if sts.Code() == codes.AlreadyExists {
			return ErrDebuginfoAlreadyExists
		}
		if sts.Code() == codes.FailedPrecondition {
			return err
		}
	}
	return nil
}
