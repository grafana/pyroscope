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
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"buf.build/gen/go/parca-dev/parca/grpc/go/parca/debuginfo/v1alpha1/debuginfov1alpha1grpc"
	"github.com/go-kit/log"
	"github.com/grafana/dskit/middleware"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metadata2 "google.golang.org/grpc/metadata"

	debuginfopb "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"

	"github.com/grafana/pyroscope/pkg/tenant"
	"github.com/grafana/pyroscope/pkg/util"
)

func TestStore(t *testing.T) {
	const orgID = "test-orgid"
	grpcCtx := metadata2.AppendToOutgoingContext(t.Context(), "x-scope-orgid", orgID)
	tenantCtx := tenant.InjectTenantID(t.Context(), orgID)

	tracer := noop.NewTracerProvider().Tracer("")
	logger := log.NewNopLogger()
	bucket := objstore.NewInMemBucket()

	metadata := NewObjectStoreMetadata(logger, bucket)
	s, err := NewStore(
		tracer,
		logger,
		metadata,
		bucket,
		SignedUpload{
			Enabled: false,
		},
		time.Minute*15,
		1024*1024*1024,
	)
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	defer grpcServer.GracefulStop()
	debuginfov1alpha1grpc.RegisterDebuginfoServiceServer(grpcServer, s)

	handler := middleware.Merge(
		middleware.Func(func(h http.Handler) http.Handler {
			return h2c.NewHandler(h, &http2.Server{})
		}),
		util.AuthenticateUser(true),
	).Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		grpcServer.ServeHTTP(w, r)
	}))

	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()

	u, err := url.Parse(httpServer.URL)
	require.NoError(t, err)
	conn, err := grpc.NewClient(u.Host, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	debuginfoClient := debuginfov1alpha1grpc.NewDebuginfoServiceClient(conn)
	grpcUploadClient := NewGrpcUploadClient(debuginfoClient)

	b := bytes.NewBuffer(nil)
	for i := 0; i < 1024; i++ {
		b.Write([]byte("a"))
	}
	for i := 0; i < 1024; i++ {
		b.Write([]byte("b"))
	}
	for i := 0; i < 1024; i++ {
		b.Write([]byte("c"))
	}

	// Totally wrong order of upload protocol sequence.
	_, err = grpcUploadClient.Upload(grpcCtx, &debuginfopb.UploadInstructions{BuildId: "abcd"}, bytes.NewReader(b.Bytes()))
	require.EqualError(t, err, "rpc error: code = FailedPrecondition desc = metadata not found, this indicates that the upload was not previously initiated")

	// Simulate we initiated this upload 30 minutes ago.
	s.timeNow = func() time.Time { return time.Now().Add(-30 * time.Minute) }

	shouldInitiateResp, err := debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd"})
	require.NoError(t, err)
	require.True(t, shouldInitiateResp.ShouldInitiateUpload)
	require.Equal(t, ReasonFirstTimeSeen, shouldInitiateResp.Reason)

	_, err = debuginfoClient.InitiateUpload(grpcCtx, &debuginfopb.InitiateUploadRequest{
		BuildId: "abcd",
		Hash:    "foo",
		Size:    2,
	})
	require.NoError(t, err)

	// An upload is already in progress. So we should not initiate another one.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd"})
	require.NoError(t, err)
	require.False(t, shouldInitiateResp.ShouldInitiateUpload)
	require.Equal(t, ReasonUploadInProgress, shouldInitiateResp.Reason)

	// But with force=true, we should be able to restart an ongoing upload.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd", Force: true})
	require.NoError(t, err)
	require.True(t, shouldInitiateResp.ShouldInitiateUpload)
	require.Equal(t, ReasonUploadInProgressButForced, shouldInitiateResp.Reason)

	// Set time to current time, where the upload should be expired. So we can initiate a new one.
	s.timeNow = time.Now

	// Correct upload flow.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd"})
	require.NoError(t, err)
	require.True(t, shouldInitiateResp.ShouldInitiateUpload)
	require.Equal(t, ReasonUploadStale, shouldInitiateResp.Reason)

	initiateResp, err := debuginfoClient.InitiateUpload(grpcCtx, &debuginfopb.InitiateUploadRequest{
		BuildId: "abcd",
		Hash:    "foo",
		Size:    2,
	})
	require.NoError(t, err)

	size, err := grpcUploadClient.Upload(grpcCtx, initiateResp.UploadInstructions, bytes.NewReader(b.Bytes()))
	require.NoError(t, err)
	require.Equal(t, 3072, int(size))

	_, err = debuginfoClient.MarkUploadFinished(grpcCtx, &debuginfopb.MarkUploadFinishedRequest{BuildId: "abcd", UploadId: initiateResp.UploadInstructions.UploadId})
	require.NoError(t, err)

	// Verify the file was stored with the tenant ID prefix.
	obj, err := s.bucket.Get(tenantCtx, "test-orgid/abcd/debuginfo")
	require.NoError(t, err)

	content, err := io.ReadAll(obj)
	require.NoError(t, err)
	require.Equal(t, 3072, len(content))
	require.Equal(t, b.Bytes(), content)

	// Uploads should not be asked to be initiated again since so far there is
	// nothing wrong with the upload. It uploaded successfully and is not
	// marked invalid.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd"})
	require.NoError(t, err)
	require.False(t, shouldInitiateResp.ShouldInitiateUpload)
	require.Equal(t, ReasonDebuginfoAlreadyExists, shouldInitiateResp.Reason)

	// But with force=true, we should be able to re-upload existing valid debuginfo.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd", Force: true})
	require.NoError(t, err)
	require.True(t, shouldInitiateResp.ShouldInitiateUpload)
	require.Equal(t, ReasonDebuginfoAlreadyExistsButForced, shouldInitiateResp.Reason)

	// If asynchronously we figured out the debuginfo was not a valid ELF file,
	// we should allow uploading something else. Don't test the whole upload
	// flow again, just the ShouldInitiateUpload part.
	// Use tenantCtx for direct metadata operations since they need the tenant injected in context.
	require.NoError(t, metadata.SetQuality(tenantCtx, "abcd", debuginfopb.DebuginfoType_DEBUGINFO_TYPE_DEBUGINFO_UNSPECIFIED, &debuginfopb.DebuginfoQuality{NotValidElf: true}))
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{BuildId: "abcd"})
	require.NoError(t, err)
	require.Equal(t, ReasonDebuginfoInvalid, shouldInitiateResp.Reason)
	require.True(t, shouldInitiateResp.ShouldInitiateUpload)

	// But we won't accept it if the hash is the same.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{
		BuildId: "abcd",
		Hash:    "foo",
	})
	require.NoError(t, err)
	require.Equal(t, ReasonDebuginfoEqual, shouldInitiateResp.Reason)
	require.False(t, shouldInitiateResp.ShouldInitiateUpload)

	// An initiation request would error.
	_, err = debuginfoClient.InitiateUpload(grpcCtx, &debuginfopb.InitiateUploadRequest{
		BuildId: "abcd",
		Hash:    "foo",
		Size:    2,
	})
	require.EqualError(t, err, "rpc error: code = AlreadyExists desc = Debuginfo already exists and is marked as invalid, but the proposed hash is the same as the one already available, therefore the upload is not accepted as it would result in the same invalid debuginfos.")

	// If the hash is different, we will accept it.
	shouldInitiateResp, err = debuginfoClient.ShouldInitiateUpload(grpcCtx, &debuginfopb.ShouldInitiateUploadRequest{
		BuildId: "abcd",
		Hash:    "bar",
	})
	require.NoError(t, err)
	require.Equal(t, ReasonDebuginfoNotEqual, shouldInitiateResp.Reason)
	require.True(t, shouldInitiateResp.ShouldInitiateUpload)
}
