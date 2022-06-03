// Copyright 2021 The Parca Authors
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
	"bytes"
	"context"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	debuginfopb "github.com/parca-dev/parca/gen/proto/go/parca/debuginfo/v1alpha1"
)

type UploadReader struct {
	stream debuginfopb.DebugInfoService_UploadServer
	cur    io.Reader
	size   uint64
}

func (r *UploadReader) Read(p []byte) (int, error) {
	if r.cur == nil {
		var err error
		r.cur, err = r.next()
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil {
			return 0, fmt.Errorf("get first upload chunk: %w", err)
		}
	}
	i, err := r.cur.Read(p)
	if err != nil && err != io.EOF {
		return 0, fmt.Errorf("read upload chunk (%d bytes read so far): %w", r.size, err)
	}
	if err == io.EOF {
		r.cur, err = r.next()
		if err == io.EOF {
			return 0, io.EOF
		}
		if err != nil {
			return 0, fmt.Errorf("get next upload chunk (%d bytes read so far): %w", r.size, err)
		}
		i, err = r.cur.Read(p)
		if err != nil {
			return 0, fmt.Errorf("read next upload chunk (%d bytes read so far): %w", r.size, err)
		}
	}

	r.size += uint64(i)
	return i, nil
}

func (r *UploadReader) next() (io.Reader, error) {
	err := contextError(r.stream.Context())
	if err != nil {
		return nil, err
	}

	req, err := r.stream.Recv()
	if err == io.EOF {
		return nil, io.EOF
	}
	if err != nil {
		return nil, fmt.Errorf("receive from stream: %w", err)
	}

	return bytes.NewBuffer(req.GetChunkData()), nil
}

func contextError(ctx context.Context) error {
	switch ctx.Err() {
	case context.Canceled:
		return status.Error(codes.Canceled, "request is canceled")
	case context.DeadlineExceeded:
		return status.Error(codes.DeadlineExceeded, "deadline is exceeded")
	default:
		return nil
	}
}
