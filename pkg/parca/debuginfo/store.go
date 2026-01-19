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
	"fmt"
	"io"
	"path"
	"time"

	"buf.build/gen/go/parca-dev/parca/grpc/go/parca/debuginfo/v1alpha1/debuginfov1alpha1grpc"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/client"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	debuginfopb "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"
)

var ErrDebuginfoNotFound = errors.New("debuginfo not found")

type CacheProvider string

const (
	FILESYSTEM CacheProvider = "FILESYSTEM"
)

type Config struct {
	Bucket *client.BucketConfig `yaml:"bucket"`
	Cache  *CacheConfig         `yaml:"cache"`
}

type FilesystemCacheConfig struct {
	Directory string `yaml:"directory"`
}

type CacheConfig struct {
	Type   CacheProvider `yaml:"type"`
	Config interface{}   `yaml:"config"`
}

type MetadataManager interface {
	MarkAsUploading(ctx context.Context, buildID, uploadID, hash string, typ debuginfopb.DebuginfoType, startedAt *timestamppb.Timestamp) error
	MarkAsUploaded(ctx context.Context, buildID, uploadID string, typ debuginfopb.DebuginfoType, finishedAt *timestamppb.Timestamp) error
	Fetch(ctx context.Context, buildID string, typ debuginfopb.DebuginfoType) (*debuginfopb.Debuginfo, error)
}

type Store struct {
	debuginfov1alpha1grpc.UnimplementedDebuginfoServiceServer

	tracer trace.Tracer
	logger log.Logger

	bucket objstore.Bucket

	metadata MetadataManager

	signedUpload SignedUpload

	maxUploadDuration time.Duration
	maxUploadSize     int64

	timeNow func() time.Time
}

type SignedUploadClient interface {
	SignedPUT(ctx context.Context, objectKey string, size int64, expiry time.Time) (signedURL string, err error)
}

type SignedUpload struct {
	Enabled bool
	Client  SignedUploadClient
}

// NewStore returns a new debug info store.
func NewStore(
	tracer trace.Tracer,
	logger log.Logger,
	metadata MetadataManager,
	bucket objstore.Bucket,
	signedUpload SignedUpload,
	maxUploadDuration time.Duration,
	maxUploadSize int64,
) (*Store, error) {
	return &Store{
		tracer:            tracer,
		logger:            log.With(logger, "component", "debuginfo"),
		bucket:            bucket,
		metadata:          metadata,
		signedUpload:      signedUpload,
		maxUploadDuration: maxUploadDuration,
		maxUploadSize:     maxUploadSize,
		timeNow:           time.Now,
	}, nil
}

const (
	ReasonFirstTimeSeen                   = "First time we see this Build ID, therefore please upload!"
	ReasonUploadStale                     = "A previous upload was started but not finished and is now stale, so it can be retried."
	ReasonUploadInProgress                = "A previous upload is still in-progress and not stale yet (only stale uploads can be retried)."
	ReasonUploadInProgressButForced       = "A previous upload is in-progress, but accepting restart because it's requested to be forced."
	ReasonDebuginfoAlreadyExists          = "Debuginfo already exists and is not marked as invalid, therefore no new upload is needed."
	ReasonDebuginfoAlreadyExistsButForced = "Debuginfo already exists and is not marked as invalid, therefore wouldn't have accepted a new upload, but accepting it because it's requested to be forced."
	ReasonDebuginfoInvalid                = "Debuginfo already exists but is marked as invalid, therefore a new upload is needed. Hash the debuginfo and initiate the upload."
	ReasonDebuginfoEqual                  = "Debuginfo already exists and is marked as invalid, but the proposed hash is the same as the one already available, therefore the upload is not accepted as it would result in the same invalid debuginfos."
	ReasonDebuginfoNotEqual               = "Debuginfo already exists but is marked as invalid, therefore a new upload will be accepted."
)

// ShouldInitiateUpload returns whether an upload should be initiated for the
// given build ID. Checking if an upload should even be initiated allows the
// parca-agent to avoid extracting debuginfos unnecessarily from a binary.
func (s *Store) ShouldInitiateUpload(ctx context.Context, req *debuginfopb.ShouldInitiateUploadRequest) (resp *debuginfopb.ShouldInitiateUploadResponse, err error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("build_id", req.BuildId))

	defer func() {
		if resp != nil {
			level.Debug(s.logger).Log(
				"msg", "ShouldInitiateUpload result",
				"build_id", req.BuildId,
				"should_initiate", resp.ShouldInitiateUpload,
				"reason", resp.Reason,
			)
		}
	}()

	buildID := req.BuildId
	if err := validateInput(buildID); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	dbginfo, err := s.metadata.Fetch(ctx, buildID, req.Type)
	if err != nil && !errors.Is(err, ErrMetadataNotFound) {
		return nil, status.Error(codes.Internal, err.Error())
	} else if errors.Is(err, ErrMetadataNotFound) {
		// First time we see this Build ID.
		// parca used to check debugninfod here, but we remove this logic cause we hit debuginfod in symbolizer
		return &debuginfopb.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: true,
			Reason:               ReasonFirstTimeSeen,
		}, nil
	} else {
		// We have seen this Build ID before and there is metadata for it.

		switch dbginfo.Source {
		case debuginfopb.Debuginfo_SOURCE_UPLOAD:
			if dbginfo.Upload == nil {
				return nil, status.Error(codes.Internal, "metadata inconsistency: upload is nil")
			}

			switch dbginfo.Upload.State {
			case debuginfopb.DebuginfoUpload_STATE_UPLOADING:
				if req.Force {
					return &debuginfopb.ShouldInitiateUploadResponse{
						ShouldInitiateUpload: true,
						Reason:               ReasonUploadInProgressButForced,
					}, nil
				}

				if s.uploadIsStale(dbginfo.Upload) {
					return &debuginfopb.ShouldInitiateUploadResponse{
						ShouldInitiateUpload: true,
						Reason:               ReasonUploadStale,
					}, nil
				}

				return &debuginfopb.ShouldInitiateUploadResponse{
					ShouldInitiateUpload: false,
					Reason:               ReasonUploadInProgress,
				}, nil
			case debuginfopb.DebuginfoUpload_STATE_UPLOADED:
				if dbginfo.Quality == nil || !dbginfo.Quality.NotValidElf {
					if req.Force {
						return &debuginfopb.ShouldInitiateUploadResponse{
							ShouldInitiateUpload: true,
							Reason:               ReasonDebuginfoAlreadyExistsButForced,
						}, nil
					}

					return &debuginfopb.ShouldInitiateUploadResponse{
						ShouldInitiateUpload: false,
						Reason:               ReasonDebuginfoAlreadyExists,
					}, nil
				}

				if req.Hash == "" {
					return &debuginfopb.ShouldInitiateUploadResponse{
						ShouldInitiateUpload: true,
						Reason:               ReasonDebuginfoInvalid,
					}, nil
				}

				if dbginfo.Upload.Hash == req.Hash {
					return &debuginfopb.ShouldInitiateUploadResponse{
						ShouldInitiateUpload: false,
						Reason:               ReasonDebuginfoEqual,
					}, nil
				}

				return &debuginfopb.ShouldInitiateUploadResponse{
					ShouldInitiateUpload: true,
					Reason:               ReasonDebuginfoNotEqual,
				}, nil
			//case debuginfopb.DebuginfoUpload_STATE_PURGED:
			//	// Debuginfo was purged, allow re-uploading
			//	return &debuginfopb.ShouldInitiateUploadResponse{
			//		ShouldInitiateUpload: true,
			//		Reason:               ReasonDebuginfoPurged,
			//	}, nil
			default:
				return nil, status.Error(codes.Internal, "metadata inconsistency: unknown upload state")
			}
		default:
			return nil, status.Errorf(codes.Internal, "unknown debuginfo source %q", dbginfo.Source)
		}
	}
}

func (s *Store) InitiateUpload(ctx context.Context, req *debuginfopb.InitiateUploadRequest) (*debuginfopb.InitiateUploadResponse, error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("build_id", req.BuildId))

	if req.Hash == "" {
		return nil, status.Error(codes.InvalidArgument, "hash must be set")
	}
	if req.Size == 0 {
		return nil, status.Error(codes.InvalidArgument, "size must be set")
	}

	// We don't want to blindly accept upload initiation requests that
	// shouldn't have happened.
	shouldInitiateResp, err := s.ShouldInitiateUpload(ctx, &debuginfopb.ShouldInitiateUploadRequest{
		BuildId:     req.BuildId,
		BuildIdType: req.BuildIdType,
		Hash:        req.Hash,
		Force:       req.Force,
		Type:        req.Type,
	})
	if err != nil {
		return nil, err
	}
	if !shouldInitiateResp.ShouldInitiateUpload {
		if shouldInitiateResp.Reason == ReasonDebuginfoEqual {
			return nil, status.Error(codes.AlreadyExists, ReasonDebuginfoEqual)
		}
		return nil, status.Errorf(codes.FailedPrecondition, "upload should not have been attempted to be initiated, a previous check should have failed with: %s", shouldInitiateResp.Reason)
	}

	if req.Size > s.maxUploadSize {
		return nil, status.Errorf(codes.InvalidArgument, "upload size %d exceeds maximum allowed size %d", req.Size, s.maxUploadSize)
	}

	uploadID := uuid.New().String()
	uploadStarted := s.timeNow()
	uploadExpiry := uploadStarted.Add(s.maxUploadDuration)

	if !s.signedUpload.Enabled {
		if err := s.metadata.MarkAsUploading(ctx, req.BuildId, uploadID, req.Hash, req.Type, timestamppb.New(uploadStarted)); err != nil {
			return nil, fmt.Errorf("mark debuginfo upload as uploading via gRPC: %w", err)
		}

		return &debuginfopb.InitiateUploadResponse{
			UploadInstructions: &debuginfopb.UploadInstructions{
				BuildId:        req.BuildId,
				UploadId:       uploadID,
				UploadStrategy: debuginfopb.UploadInstructions_UPLOAD_STRATEGY_GRPC,
				Type:           req.Type,
			},
		}, nil
	}

	signedURL, err := s.signedUpload.Client.SignedPUT(ctx, objectPath(req.BuildId, req.Type), req.Size, uploadExpiry)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if err := s.metadata.MarkAsUploading(ctx, req.BuildId, uploadID, req.Hash, req.Type, timestamppb.New(uploadStarted)); err != nil {
		return nil, fmt.Errorf("mark debuginfo upload as uploading via signed URL: %w", err)
	}

	return &debuginfopb.InitiateUploadResponse{
		UploadInstructions: &debuginfopb.UploadInstructions{
			BuildId:        req.BuildId,
			UploadId:       uploadID,
			UploadStrategy: debuginfopb.UploadInstructions_UPLOAD_STRATEGY_SIGNED_URL,
			SignedUrl:      signedURL,
			Type:           req.Type,
		},
	}, nil
}

func (s *Store) MarkUploadFinished(ctx context.Context, req *debuginfopb.MarkUploadFinishedRequest) (*debuginfopb.MarkUploadFinishedResponse, error) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("build_id", req.BuildId))
	span.SetAttributes(attribute.String("upload_id", req.UploadId))

	buildID := req.BuildId
	if err := validateInput(buildID); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err := s.metadata.MarkAsUploaded(ctx, buildID, req.UploadId, req.Type, timestamppb.New(s.timeNow()))
	if errors.Is(err, ErrDebuginfoNotFound) {
		return nil, status.Error(codes.NotFound, "no debuginfo metadata found for build id")
	}
	if errors.Is(err, ErrUploadMetadataNotFound) {
		return nil, status.Error(codes.NotFound, "no debuginfo upload metadata found for build id")
	}
	if errors.Is(err, ErrUploadIDMismatch) {
		return nil, status.Error(codes.InvalidArgument, "upload id mismatch")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &debuginfopb.MarkUploadFinishedResponse{}, nil
}

func (s *Store) Upload(stream debuginfov1alpha1grpc.DebuginfoService_UploadServer) error {
	if s.signedUpload.Enabled {
		return status.Error(codes.Unimplemented, "signed URL uploads are the only supported upload strategy for this service")
	}

	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.Unknown, "failed to receive upload info: %q", err)
	}

	var (
		buildID  = req.GetInfo().BuildId
		uploadID = req.GetInfo().UploadId
		r        = &UploadReader{stream: stream}
		typ      = req.GetInfo().Type
	)

	ctx := stream.Context()
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("build_id", buildID))
	span.SetAttributes(attribute.String("upload_id", uploadID))

	if err := s.upload(ctx, buildID, uploadID, typ, r); err != nil {
		level.Debug(s.logger).Log(
			"msg", "debuginfo upload failed",
			"build_id", buildID,
			"upload_id", uploadID,
			"type", typ.String(),
			"err", err,
		)
		return err
	}

	level.Debug(s.logger).Log(
		"msg", "debuginfo upload completed",
		"build_id", buildID,
		"upload_id", uploadID,
		"type", typ.String(),
		"size", r.size,
	)

	return stream.SendAndClose(&debuginfopb.UploadResponse{
		BuildId: buildID,
		Size:    r.size,
	})
}

func (s *Store) upload(ctx context.Context, buildID, uploadID string, typ debuginfopb.DebuginfoType, r io.Reader) error {
	if err := validateInput(buildID); err != nil {
		return status.Errorf(codes.InvalidArgument, "invalid build ID: %q", err)
	}

	dbginfo, err := s.metadata.Fetch(ctx, buildID, typ)
	if err != nil {
		if errors.Is(err, ErrMetadataNotFound) {
			return status.Error(codes.FailedPrecondition, "metadata not found, this indicates that the upload was not previously initiated")
		}
		return status.Error(codes.Internal, err.Error())
	}

	if dbginfo.Upload == nil {
		return status.Error(codes.FailedPrecondition, "upload metadata not found, this indicates that the upload was not previously initiated")
	}

	if dbginfo.Upload.Id != uploadID {
		return status.Error(codes.InvalidArgument, "the upload ID does not match the one returned by the InitiateUpload call")
	}

	if err := s.bucket.Upload(ctx, objectPath(buildID, typ), r); err != nil {
		return status.Error(codes.Internal, fmt.Errorf("upload debuginfo: %w", err).Error())
	}

	return nil
}

func (s *Store) uploadIsStale(upload *debuginfopb.DebuginfoUpload) bool {
	return upload.StartedAt.AsTime().Add(s.maxUploadDuration + 2*time.Minute).Before(s.timeNow())
}

func validateInput(id string) error {
	if len(id) <= 2 {
		return errors.New("unexpectedly short input")
	}

	return nil
}

func objectPath(buildID string, typ debuginfopb.DebuginfoType) string {
	switch typ {
	case debuginfopb.DebuginfoType_DEBUGINFO_TYPE_EXECUTABLE:
		return path.Join(buildID, "executable")
	case debuginfopb.DebuginfoType_DEBUGINFO_TYPE_SOURCES:
		return path.Join(buildID, "sources")
	default:
		return path.Join(buildID, "debuginfo")
	}
}
