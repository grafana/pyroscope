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
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"regexp"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/objstore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	debuginfov1 "github.com/grafana/pyroscope/api/gen/proto/go/debuginfo/v1"
	debuginforeader "github.com/grafana/pyroscope/pkg/debuginfo/reader"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type Config struct {
	Enabled           bool          `yaml:"-"`
	MaxUploadSize     int64         `yaml:"-"`
	MaxUploadDuration time.Duration `yaml:"-"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "debug-info.enabled", true, "Enable debug info.")
	f.Int64Var(&cfg.MaxUploadSize, "debug-info.max-upload-size", 100*1024*1024, "Maximum size of a single debug info upload in bytes.")
	f.DurationVar(&cfg.MaxUploadDuration, "debug-info.max-upload-duration", time.Minute, "Maximum duration of a single debug info upload.")
}

type Store struct {
	logger log.Logger

	bucket objstore.Bucket

	cfg Config
}

// NewStore returns a new debug info store.
func NewStore(
	logger log.Logger,
	bucket objstore.Bucket,
	cfg Config,
) (*Store, error) {
	if cfg.Enabled && bucket == nil {
		return nil, errors.New("enabled debug info requires a bucket")
	}
	return &Store{
		logger: log.With(logger, "component", "debuginfo"),
		bucket: bucket,
		cfg:    cfg,
	}, nil
}

const (
	ReasonFirstTimeSeen          = "First time we see this Build ID, therefore please upload!"
	ReasonUploadStale            = "A previous upload was started but not finished and is now stale, so it can be retried."
	ReasonUploadInProgress       = "A previous upload is still in-progress and not stale yet (only stale uploads can be retried)."
	ReasonDebuginfoAlreadyExists = "Debuginfo already exists and is not marked as invalid, therefore no new upload is needed."
	ReasonDisabled               = "DebugInfo upload disabled"
)

func (s *Store) shouldInitiateUpload(
	md *debuginfov1.ObjectMetadata,
) (
	*debuginfov1.ShouldInitiateUploadResponse,
	error,
) {

	if md == nil {
		return &debuginfov1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: true,
			Reason:               ReasonFirstTimeSeen,
		}, nil
	}

	switch md.State {
	case debuginfov1.ObjectMetadata_STATE_UPLOADING:
		if s.uploadIsStale(md) {
			return &debuginfov1.ShouldInitiateUploadResponse{
				ShouldInitiateUpload: true,
				Reason:               ReasonUploadStale,
			}, nil
		}

		return &debuginfov1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               ReasonUploadInProgress,
		}, nil
	case debuginfov1.ObjectMetadata_STATE_UPLOADED:
		return &debuginfov1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               ReasonDebuginfoAlreadyExists,
		}, nil
	default:
		return nil, fmt.Errorf("metadata inconsistency: unknown upload state")
	}
}

func (s *Store) Upload(ctx context.Context, stream *connect.BidiStream[debuginfov1.UploadRequest, debuginfov1.UploadResponse]) error {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	req, err := stream.Receive()
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to receive init request: %w", err))
	}
	init := req.GetInit()
	id, err := validateInit(init)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("invalid init request: %w", err))
	}

	l := log.With(s.logger, "gnu_build_id", init.File.GNU)

	if !s.cfg.Enabled { // move this to 	shouldInitiateUpload func

		return stream.Send(&debuginfov1.UploadResponse{
			Data: &debuginfov1.UploadResponse_Init{
				Init: &debuginfov1.ShouldInitiateUploadResponse{
					ShouldInitiateUpload: false,
					Reason:               ReasonDisabled,
				},
			},
		})
	}

	md, err := s.fetchMetadata(ctx, tenantID, id)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch metadata: %w", err))
	}

	initResponse, err := s.shouldInitiateUpload(md)
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed shouldInitiateUpload check: %w", err))
	}

	err = stream.Send(&debuginfov1.UploadResponse{
		Data: &debuginfov1.UploadResponse_Init{
			Init: initResponse,
		},
	})
	if err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to send init response: %w", err))
	}
	if !initResponse.ShouldInitiateUpload {
		return nil
	}
	md = &debuginfov1.ObjectMetadata{
		File:       init.File,
		State:      debuginfov1.ObjectMetadata_STATE_UPLOADING,
		StartedAt:  timestamppb.New(time.Now()),
		FinishedAt: nil,
	}
	if err := s.writeMetadata(ctx, tenantID, id, md); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write uploading metadata: %w", err))
	}

	r := debuginforeader.New(ctx, readChunksFromStream(stream))
	if err := s.bucket.Upload(ctx, ObjectPath(tenantID, id), r); err != nil {
		return status.Error(codes.Internal, fmt.Errorf("upload debuginfo: %w", err).Error())
	}
	md.FinishedAt = timestamppb.New(time.Now())
	if err := s.writeMetadata(ctx, tenantID, id, md); err != nil {
		return connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write uploaded metadata: %w", err))
	}

	_ = level.Debug(l).Log(
		"msg", "debuginfo upload completed",
		"size", r.Size(),
	)
	return nil
}

func readChunksFromStream(stream *connect.BidiStream[debuginfov1.UploadRequest, debuginfov1.UploadResponse]) func() ([]byte, error) {
	return func() ([]byte, error) {
		req, err := stream.Receive()
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		if err != nil {
			return nil, fmt.Errorf("receive from stream: %w", err)
		}
		chunk := req.GetChunk()
		if chunk == nil || len(chunk.Chunk) == 0 {
			return nil, fmt.Errorf("receive from stream: chunk is nil")
		}
		return chunk.Chunk, nil
	}
}

func validateInit(init *debuginfov1.ShouldInitiateUploadRequest) (*ValidGnuBuildID, error) {
	if init == nil {
		return nil, fmt.Errorf("first message expected to be init")
	}
	if init.File == nil {
		return nil, fmt.Errorf("init.FileData == nil")
	}
	switch init.File.Type {
	case debuginfov1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_FULL:
		return ValidateGnuBuildID(init.File.GNU)
	case debuginfov1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_NO_TEXT:
		return ValidateGnuBuildID(init.File.GNU)
	default:
		return nil, fmt.Errorf("init.FileData.Type(%d) is not valid", init.File.Type)
	}
}

func (s *Store) uploadIsStale(upload *debuginfov1.ObjectMetadata) bool {
	return upload.StartedAt.AsTime().Add(s.cfg.MaxUploadDuration + 2*time.Minute).Before(time.Now())
}

func (s *Store) fetchMetadata(ctx context.Context, tenantID string, id *ValidGnuBuildID) (*debuginfov1.ObjectMetadata, error) {
	r, err := s.bucket.Get(ctx, MetadataObjectPath(tenantID, id))
	if err != nil {
		if s.bucket.IsObjNotFoundErr(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("fetch debuginfo metadata from object storage: %w", err)
	}
	defer r.Close()

	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read debuginfo metadata from object storage: %w", err)
	}

	dbginfo := &debuginfov1.ObjectMetadata{}
	if err := protojson.Unmarshal(content, dbginfo); err != nil {
		return nil, fmt.Errorf("unmarshal debuginfo metadata: %w", err)
	}
	return dbginfo, nil
}

func (s *Store) writeMetadata(ctx context.Context, tenantID string, id *ValidGnuBuildID, md *debuginfov1.ObjectMetadata) error {
	bs, err := protojson.Marshal(md)
	if err != nil {
		return fmt.Errorf("marshal debuginfo metadata: %w", err)
	}

	return s.bucket.Upload(ctx, MetadataObjectPath(tenantID, id), bytes.NewReader(bs))
}

var gnuRegex = regexp.MustCompile("^[a-fA-F0-9]{2,40}$")

type ValidGnuBuildID struct {
	gnuBuildID string
}

func ValidateGnuBuildID(gnuBuildID string) (*ValidGnuBuildID, error) {
	if !gnuRegex.MatchString(gnuBuildID) {
		return nil, fmt.Errorf("invalid gnuBuildID %q", gnuBuildID)
	}

	return &ValidGnuBuildID{gnuBuildID}, nil
}

const bucketPrefix = "debug-info"

func ObjectPath(tenantID string, id *ValidGnuBuildID) string {
	return path.Join(bucketPrefix, tenantID, id.gnuBuildID, "exe")
}

func MetadataObjectPath(tenantID string, id *ValidGnuBuildID) string {
	return path.Join(bucketPrefix, tenantID, id.gnuBuildID, "metadata")
}
