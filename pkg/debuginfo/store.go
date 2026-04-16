package debuginfo

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"time"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/thanos-io/objstore"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	debuginfov1alpha1 "github.com/grafana/pyroscope/api/gen/proto/go/debuginfo/v1alpha1"
	"github.com/grafana/pyroscope/pkg/tenant"
)

type Config struct {
	Enabled            bool          `yaml:"-"`
	MaxUploadSize      int64         `yaml:"-"`
	UploadStalePeriod  time.Duration `yaml:"-"`
	UploadTimeout      time.Duration `yaml:"-"`
}

func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	f.BoolVar(&cfg.Enabled, "debug-info.enabled", true, "Enable debug info.")
	f.Int64Var(&cfg.MaxUploadSize, "debug-info.max-upload-size", 100*1024*1024, "Maximum size of a single debug info upload in bytes.")
	f.DurationVar(&cfg.UploadStalePeriod, "debug-info.max-upload-duration", time.Minute, "Period after which a pending upload is considered stale and can be retried.")
	f.DurationVar(&cfg.UploadTimeout, "debug-info.upload-timeout", 2*time.Minute, "Timeout for a single debug info upload request. Overrides server HTTP write timeout for this handler.")
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

func (s *Store) checkShouldInitiateUpload(
	md *debuginfov1alpha1.ObjectMetadata,
) (
	*debuginfov1alpha1.ShouldInitiateUploadResponse,
	error,
) {
	if md == nil {
		return &debuginfov1alpha1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: true,
			Reason:               ReasonFirstTimeSeen,
		}, nil
	}

	switch md.State {
	case debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING:
		if s.uploadIsStale(md) {
			return &debuginfov1alpha1.ShouldInitiateUploadResponse{
				ShouldInitiateUpload: true,
				Reason:               ReasonUploadStale,
			}, nil
		}

		return &debuginfov1alpha1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               ReasonUploadInProgress,
		}, nil
	case debuginfov1alpha1.ObjectMetadata_STATE_UPLOADED:
		return &debuginfov1alpha1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               ReasonDebuginfoAlreadyExists,
		}, nil
	default:
		return nil, fmt.Errorf("metadata inconsistency: unknown upload state")
	}
}

func (s *Store) ShouldInitiateUpload(
	ctx context.Context,
	req *connect.Request[debuginfov1alpha1.ShouldInitiateUploadRequest],
) (*connect.Response[debuginfov1alpha1.ShouldInitiateUploadResponse], error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	if !s.cfg.Enabled {
		return connect.NewResponse(&debuginfov1alpha1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               ReasonDisabled,
		}), nil
	}

	id, err := validateInit(req.Msg)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid request: %w", err))
	}

	md, err := s.fetchMetadata(ctx, tenantID, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch metadata: %w", err))
	}

	resp, err := s.checkShouldInitiateUpload(md)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed shouldInitiateUpload check: %w", err))
	}

	if resp.ShouldInitiateUpload {
		md = &debuginfov1alpha1.ObjectMetadata{
			File:      req.Msg.File,
			State:     debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING,
			StartedAt: timestamppb.New(time.Now()),
		}
		if err := s.writeMetadata(ctx, tenantID, id, md); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write uploading metadata: %w", err))
		}
	}

	return connect.NewResponse(resp), nil
}

func (s *Store) UploadHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.UploadTimeout > 0 {
			deadline := time.Now().Add(s.cfg.UploadTimeout)
			rc := http.NewResponseController(w)
			_ = rc.SetReadDeadline(deadline)
			_ = rc.SetWriteDeadline(deadline)
		}

		ctx := r.Context()
		if s.cfg.UploadTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, s.cfg.UploadTimeout)
			defer cancel()
		}

		tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		gnuBuildIDStr := mux.Vars(r)["gnu_build_id"]
		id, err := ValidateGnuBuildID(gnuBuildIDStr)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid gnu_build_id: %v", err), http.StatusBadRequest)
			return
		}

		l := log.With(s.logger, "gnu_build_id", id.gnuBuildID)

		md, err := s.fetchMetadata(ctx, tenantID, id)
		if err != nil {
			_ = level.Error(l).Log("msg", "failed to fetch metadata", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if md == nil || md.State != debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING {
			http.Error(w, "no pending upload for this build ID", http.StatusPreconditionFailed)
			return
		}

		if s.cfg.MaxUploadSize > 0 {
			r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadSize)
		}
		if err := s.bucket.Upload(ctx, ObjectPath(tenantID, id), r.Body); err != nil {
			_ = level.Error(l).Log("msg", "failed to upload debuginfo", "err", err)
			http.Error(w, "upload failed", http.StatusInternalServerError)
			return
		}

		_ = level.Debug(l).Log("msg", "debuginfo upload completed")
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Store) UploadFinished(
	ctx context.Context,
	req *connect.Request[debuginfov1alpha1.UploadFinishedRequest],
) (*connect.Response[debuginfov1alpha1.UploadFinishedResponse], error) {
	tenantID, err := tenant.ExtractTenantIDFromContext(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	id, err := ValidateGnuBuildID(req.Msg.GnuBuildId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid gnu_build_id: %w", err))
	}

	l := log.With(s.logger, "gnu_build_id", id.gnuBuildID)

	md, err := s.fetchMetadata(ctx, tenantID, id)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch metadata: %w", err))
	}
	if md == nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("no upload metadata found for build ID %s", req.Msg.GnuBuildId))
	}
	if md.State != debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("upload is not in uploading state"))
	}

	exists, err := s.bucket.Exists(ctx, ObjectPath(tenantID, id))
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check uploaded file: %w", err))
	}
	if !exists {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no uploaded file found for build ID %s", req.Msg.GnuBuildId))
	}

	md.State = debuginfov1alpha1.ObjectMetadata_STATE_UPLOADED
	md.FinishedAt = timestamppb.New(time.Now())
	if err := s.writeMetadata(ctx, tenantID, id, md); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write uploaded metadata: %w", err))
	}

	_ = level.Debug(l).Log("msg", "debuginfo upload finished")
	return connect.NewResponse(&debuginfov1alpha1.UploadFinishedResponse{}), nil
}

func validateInit(init *debuginfov1alpha1.ShouldInitiateUploadRequest) (*ValidGnuBuildID, error) {
	if init == nil {
		return nil, fmt.Errorf("first message expected to be init")
	}
	if init.File == nil {
		return nil, fmt.Errorf("init.File == nil")
	}
	switch init.File.Type {
	case debuginfov1alpha1.FileMetadata_TYPE_EXECUTABLE_FULL:
		return ValidateGnuBuildID(init.File.GnuBuildId)
	case debuginfov1alpha1.FileMetadata_TYPE_EXECUTABLE_NO_TEXT:
		return ValidateGnuBuildID(init.File.GnuBuildId)
	default:
		return nil, fmt.Errorf("init.File.Type(%d) is not valid", init.File.Type)
	}
}

func (s *Store) uploadIsStale(upload *debuginfov1alpha1.ObjectMetadata) bool {
	return upload.StartedAt.AsTime().Add(s.cfg.UploadStalePeriod + 2*time.Minute).Before(time.Now())
}

func (s *Store) fetchMetadata(ctx context.Context, tenantID string, id *ValidGnuBuildID) (*debuginfov1alpha1.ObjectMetadata, error) {
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

	dbginfo := &debuginfov1alpha1.ObjectMetadata{}
	if err := protojson.Unmarshal(content, dbginfo); err != nil {
		return nil, fmt.Errorf("unmarshal debuginfo metadata: %w", err)
	}
	return dbginfo, nil
}

func (s *Store) writeMetadata(ctx context.Context, tenantID string, id *ValidGnuBuildID, md *debuginfov1alpha1.ObjectMetadata) error {
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
