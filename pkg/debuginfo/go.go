package debuginfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	debuginfogrpc "buf.build/gen/go/parca-dev/parca/grpc/go/parca/debuginfo/v1alpha1/debuginfov1alpha1grpc"
	debuginfov1alpha1 "buf.build/gen/go/parca-dev/parca/protocolbuffers/go/parca/debuginfo/v1alpha1"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/google/uuid"
	"github.com/grafana/dskit/server"
	"github.com/grafana/dskit/services"
	"github.com/grafana/pyroscope/pkg/objstore"
	"github.com/grafana/pyroscope/pkg/symbolizer"
	"github.com/grafana/pyroscope/pkg/util/httpgrpc"
	"google.golang.org/grpc"
)

type Handler struct {
	http.Handler
	services.Service

	l      log.Logger
	bucket objstore.Bucket

	inflight map[uploadID]*upload
	mu       sync.Mutex
}

func NewHandler(l log.Logger, cfg server.Config, bucket objstore.Bucket) *Handler {
	s := httpgrpc.NewGrpcServer(cfg)
	d := &Handler{
		Handler:  s,
		l:        l,
		bucket:   bucket,
		inflight: make(map[uploadID]*upload),
	}
	d.Service = services.NewBasicService(d.starting, d.running, d.stopping)
	debuginfogrpc.RegisterDebuginfoServiceServer(s, d)
	return d
}

type uploadID string
type upload struct {
	id  uploadID
	req *debuginfov1alpha1.InitiateUploadRequest
	buf *bytes.Buffer
	mu  sync.Mutex
}

func (d *Handler) Upload(g grpc.ClientStreamingServer[debuginfov1alpha1.UploadRequest, debuginfov1alpha1.UploadResponse]) error {
	ctx, cancel := context.WithTimeout(g.Context(), 10*time.Second)
	defer cancel()
	m, err := g.Recv()
	if err != nil {
		return errors.New("failed to receive upload request")
	}
	if !m.HasInfo() {
		return errors.New("expected upload request to have info")
	}
	info := m.GetInfo()
	uploadId := uploadID(info.GetUploadId())
	info.GetBuildId()

	d.mu.Lock()
	u := d.inflight[uploadId]
	d.mu.Unlock()

	defer d.cleanup(u)
	if u == nil {
		return errors.New("upload id not found")
	}
	for {
		// todo ctx timeout
		m, err = g.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			return errors.New("failed to receive upload request")
		}
		if !m.HasChunkData() {

			return errors.New("expected upload request to have chunk data")
		}
		chunk := m.GetChunkData()
		u.mu.Lock()
		u.buf.Write(chunk)
		u.mu.Unlock()
	}
	if err = d.convert(ctx, u); err != nil {
		return fmt.Errorf("failed to convert debug info: %w", err)
	}
	return g.SendAndClose(&debuginfov1alpha1.UploadResponse{
		BuildId: u.req.BuildId,
		Size:    uint64(u.buf.Len()),
	})
}

func (d *Handler) ShouldInitiateUpload(ctx context.Context, request *debuginfov1alpha1.ShouldInitiateUploadRequest) (*debuginfov1alpha1.ShouldInitiateUploadResponse, error) {
	_ = level.Debug(d.l).Log(
		"msg", "should initiate upload",
		"build_id", request.BuildId,
		"build_id_type", request.BuildIdType,
		"hash", request.Hash,
	)
	buildID := request.GetBuildId()
	exists, err := d.buildIdExists(ctx, buildID, request.BuildIdType)
	if err != nil {
		return nil, err
	}
	if exists {
		return &debuginfov1alpha1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               fmt.Sprintf("file with build id %s already exists", buildID),
		}, nil
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	uploadInProgress := false
	for _, u := range d.inflight {
		if u.req.BuildId == buildID && u.req.BuildIdType == request.BuildIdType {
			uploadInProgress = true
			break
		}

	}
	if uploadInProgress {
		return &debuginfov1alpha1.ShouldInitiateUploadResponse{
			ShouldInitiateUpload: false,
			Reason:               fmt.Sprintf("upload for build id %s already in progress", buildID),
		}, nil
	}
	return &debuginfov1alpha1.ShouldInitiateUploadResponse{
		ShouldInitiateUpload: true,
	}, nil
}

func (d *Handler) InitiateUpload(_ context.Context, req *debuginfov1alpha1.InitiateUploadRequest) (*debuginfov1alpha1.InitiateUploadResponse, error) {
	id := newUploadID()
	d.mu.Lock()
	defer d.mu.Unlock()
	u := &upload{
		id:  id,
		req: req,
		buf: bytes.NewBuffer(nil),
		// todo do not keep in memory - initiate bucket put
	}
	d.inflight[id] = u
	_ = level.Debug(d.l).Log("msg", "initiated upload", "upload_id", id,
		"build_id", req.BuildId,
		"build_id_type", req.BuildIdType,
		"hash", req.Hash,
	)

	return &debuginfov1alpha1.InitiateUploadResponse{
		UploadInstructions: &debuginfov1alpha1.UploadInstructions{
			BuildId:        req.BuildId,
			UploadId:       string(id),
			UploadStrategy: debuginfov1alpha1.UploadInstructions_UPLOAD_STRATEGY_GRPC,
		},
	}, nil
}

func (d *Handler) MarkUploadFinished(ctx context.Context, req *debuginfov1alpha1.MarkUploadFinishedRequest) (*debuginfov1alpha1.MarkUploadFinishedResponse, error) {
	return &debuginfov1alpha1.MarkUploadFinishedResponse{}, nil
}

// todo extract this logic into symbolizer package, we may need to change prefixes in the future
// todo tenant isolation
// todo: add support for other id types with prefixes?
// todo: add support for original keep
// todo : submit a separate change for this
func (d *Handler) buildIdExists(ctx context.Context, buildID string, typ debuginfov1alpha1.BuildIDType) (bool, error) {
	return d.bucket.Exists(ctx, buildID)
}

func (d *Handler) cleanup(u *upload) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.inflight, u.id)

}

// todo do not convert here? do it lazy in the symbolizer? optionally?
func (d *Handler) convert(ctx context.Context, u *upload) error {
	lidiaBytes, errMetric, err := symbolizer.ProcessELFData(u.buf.Bytes())
	if err != nil {
		_ = errMetric // todo
		return fmt.Errorf("failed to convert debug info: %w", err)
	}
	// todo tenant isolation
	// todo sanitize build id
	// todo santitize buil id  type
	// todo use build id type in the file name
	err = d.bucket.Upload(ctx, u.req.BuildId, bytes.NewReader(lidiaBytes))
	if err != nil {
		return fmt.Errorf("failed to upload debug info: %w", err)
	}
	return err
}

func (d *Handler) starting(ctx context.Context) error {
	return nil
}

func (d *Handler) running(ctx context.Context) error {
	_ = <-ctx.Done()
	return nil
}

func (d *Handler) stopping(err error) error {
	panic("TODO")
}

func newUploadID() uploadID {
	return uploadID(uuid.NewString())
}
