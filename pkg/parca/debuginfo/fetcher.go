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
	"context"
	"errors"
	"io"

	debuginfopb "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"

	"github.com/thanos-io/objstore"
)

var (
	ErrUnknownDebuginfoSource = errors.New("unknown debuginfo source")
	ErrNotUploadedYet         = errors.New("debuginfo not uploaded yet")
	ErrDebuginfoPurged        = errors.New("debuginfo has been purged")
)

type Fetcher struct {
	debuginfodClients DebuginfodClients
	bucket            objstore.Bucket
}

func NewFetcher(
	debuginfodClients DebuginfodClients,
	bucket objstore.Bucket,
) *Fetcher {
	return &Fetcher{
		debuginfodClients: debuginfodClients,
		bucket:            bucket,
	}
}

func (f *Fetcher) FetchDebuginfo(ctx context.Context, dbginfo *debuginfopb.Debuginfo) (io.ReadCloser, error) {
	switch dbginfo.Source {
	case debuginfopb.Debuginfo_SOURCE_UPLOAD:
		return f.fetchFromBucket(ctx, dbginfo)
	case debuginfopb.Debuginfo_SOURCE_DEBUGINFOD:
		return f.fetchFromDebuginfod(ctx, dbginfo)
	default:
		return nil, ErrUnknownDebuginfoSource
	}
}

func (f *Fetcher) fetchFromBucket(ctx context.Context, dbginfo *debuginfopb.Debuginfo) (io.ReadCloser, error) {
	return f.bucket.Get(ctx, objectPath(dbginfo.BuildId, dbginfo.Type))
}

func (f *Fetcher) fetchFromDebuginfod(ctx context.Context, dbginfo *debuginfopb.Debuginfo) (io.ReadCloser, error) {
	if len(dbginfo.DebuginfodServers) == 0 {
		return nil, errors.New("no debuginfod servers")
	}

	// Servers are stored in order of preference.
	debuginfodServer := dbginfo.DebuginfodServers[0]

	return f.debuginfodClients.Get(ctx, debuginfodServer, dbginfo.BuildId)
}
