// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/block_upload.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/tenant"
	"github.com/grafana/regexp"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/thanos-io/objstore"

	"github.com/grafana/mimir/pkg/storage/bucket"
	"github.com/grafana/mimir/pkg/storage/sharding"
	mimir_tsdb "github.com/grafana/mimir/pkg/storage/tsdb"
	"github.com/grafana/mimir/pkg/storage/tsdb/block"
	"github.com/grafana/mimir/pkg/util"
	util_log "github.com/grafana/mimir/pkg/util/log"
)

const (
	uploadingMetaFilename       = "uploading-meta.json" // Name of the file that stores a block's meta file while it's being uploaded
	validationFilename          = "validation.json"     // Name of the file that stores a heartbeat time and possibly an error message
	validationHeartbeatInterval = 1 * time.Minute       // Duration of time between heartbeats of an in-progress block upload validation
	validationHeartbeatTimeout  = 5 * time.Minute       // Maximum duration of time to wait until a validation is able to be restarted
	maximumMetaSizeBytes        = 1 * 1024 * 1024       // 1 MiB, maximum allowed size of an uploaded block's meta.json file
)

var (
	maxBlockUploadSizeBytesFormat = "block exceeds the maximum block size limit of %d bytes"
	rePath                        = regexp.MustCompile(`^(index|chunks/\d{6})$`)
)

// StartBlockUpload handles request for starting block upload.
//
// Starting the uploading of a block means to upload a meta file and verify that the upload can
// go ahead. In practice this means to check that the (complete) block isn't already in block
// storage, and that the meta file is valid.
func (c *MultitenantCompactor) StartBlockUpload(w http.ResponseWriter, r *http.Request) {
	blockID, tenantID, err := c.parseBlockUploadParameters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	requestID := hexTimeNowNano()
	logger := log.With(
		util_log.WithContext(ctx, c.logger),
		"feature", "block upload",
		"block", blockID,
		"operation", "start block upload",
		"request_id", requestID,
	)

	userBkt := bucket.NewUserBucketClient(tenantID, c.bucketClient, c.cfgProvider)
	if _, _, err := c.checkBlockState(ctx, userBkt, blockID, false); err != nil {
		writeBlockUploadError(err, "can't check block state", logger, w, requestID)
		return
	}

	content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maximumMetaSizeBytes))
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			err = httpError{
				message:    fmt.Sprintf("The block metadata was too large (maximum size allowed is %d bytes)", maximumMetaSizeBytes),
				statusCode: http.StatusRequestEntityTooLarge,
			}
		}
		writeBlockUploadError(err, "failed reading body", logger, w, requestID)
		return
	}

	var meta block.Meta
	if err := json.Unmarshal(content, &meta); err != nil {
		err = httpError{
			message:    "malformed request body",
			statusCode: http.StatusBadRequest,
		}
		writeBlockUploadError(err, "failed unmarshaling block meta json", logger, w, requestID)
		return
	}

	if err := c.createBlockUpload(ctx, &meta, logger, userBkt, tenantID, blockID); err != nil {
		writeBlockUploadError(err, "failed creating block upload", logger, w, requestID)
		return
	}

	level.Info(logger).Log("msg", "started block upload")

	w.WriteHeader(http.StatusOK)
}

// FinishBlockUpload handles request for finishing block upload.
//
// Finishing block upload performs block validation, and if all checks pass, marks block as finished
// by uploading meta.json file.
func (c *MultitenantCompactor) FinishBlockUpload(w http.ResponseWriter, r *http.Request) {
	blockID, tenantID, err := c.parseBlockUploadParameters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	requestID := hexTimeNowNano()
	logger := log.With(
		util_log.WithContext(ctx, c.logger),
		"feature", "block upload",
		"block", blockID,
		"operation", "complete block upload",
		"request_id", requestID,
	)

	userBkt := bucket.NewUserBucketClient(tenantID, c.bucketClient, c.cfgProvider)
	m, _, err := c.checkBlockState(ctx, userBkt, blockID, true)
	if err != nil {
		writeBlockUploadError(err, "can't check block state", logger, w, requestID)
		return
	}

	// This should not happen, as checkBlockState with requireUploadInProgress=true returns nil error
	// only if uploading-meta.json file exists.
	if m == nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if c.cfgProvider.CompactorBlockUploadValidationEnabled(tenantID) {
		maxConcurrency := int64(c.compactorCfg.MaxBlockUploadValidationConcurrency)
		currentValidations := c.blockUploadValidations.Inc()
		decreaseActiveValidationsInDefer := true
		defer func() {
			if decreaseActiveValidationsInDefer {
				c.blockUploadValidations.Dec()
			}
		}()
		if maxConcurrency > 0 && currentValidations > maxConcurrency {
			err := httpError{
				message:    fmt.Sprintf("too many block upload validations in progress, limit is %d", maxConcurrency),
				statusCode: http.StatusTooManyRequests,
			}
			writeBlockUploadError(err, "max concurrency was hit", logger, w, requestID)
			return
		}
		// create validation file to signal that block validation has started
		if err := c.uploadValidation(ctx, blockID, userBkt); err != nil {
			writeBlockUploadError(err, "can't upload validation file", logger, w, requestID)
			return
		}
		decreaseActiveValidationsInDefer = false
		go c.validateAndCompleteBlockUpload(logger, tenantID, userBkt, blockID, m, func(ctx context.Context) error {
			defer c.blockUploadValidations.Dec()
			return c.validateBlock(ctx, logger, blockID, m, userBkt, tenantID)
		})
		level.Info(logger).Log("msg", "validation process started")
	} else {
		if err := c.markBlockComplete(ctx, logger, tenantID, userBkt, blockID, m); err != nil {
			writeBlockUploadError(err, "can't mark block as complete", logger, w, requestID)
			return
		}
		level.Info(logger).Log("msg", "successfully finished block upload")
	}

	w.WriteHeader(http.StatusOK)
}

// parseBlockUploadParameters parses common parameters from the request: block ID, tenant and checks if tenant has uploads enabled.
func (c *MultitenantCompactor) parseBlockUploadParameters(r *http.Request) (ulid.ULID, string, error) {
	blockID, err := ulid.Parse(mux.Vars(r)["block"])
	if err != nil {
		return ulid.ULID{}, "", errors.New("invalid block ID")
	}

	ctx := r.Context()
	tenantID, err := tenant.TenantID(ctx)
	if err != nil {
		return ulid.ULID{}, "", errors.New("invalid tenant ID")
	}

	if !c.cfgProvider.CompactorBlockUploadEnabled(tenantID) {
		return ulid.ULID{}, "", errors.New("block upload is disabled")
	}

	return blockID, tenantID, nil
}

func writeBlockUploadError(err error, msg string, logger log.Logger, w http.ResponseWriter, requestID string) {
	var httpErr httpError
	if errors.As(err, &httpErr) {
		level.Warn(logger).Log("msg", msg, "response", httpErr.message, "status", httpErr.statusCode)
		http.Error(w, httpErr.message, httpErr.statusCode)
		return
	}

	level.Error(logger).Log("msg", msg, "err", err)
	http.Error(w, fmt.Sprintf("internal server error (id %s)", requestID), http.StatusInternalServerError)
}

// hexTimeNano returns a hex-encoded big-endian representation of the current time in nanoseconds, previously converted to uint64 and encoded as big-endian.
func hexTimeNowNano() string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(time.Now().UTC().UnixNano()))
	return hex.EncodeToString(buf[:])
}

func (c *MultitenantCompactor) createBlockUpload(ctx context.Context, meta *block.Meta,
	logger log.Logger, userBkt objstore.Bucket, tenantID string, blockID ulid.ULID,
) error {
	level.Debug(logger).Log("msg", "starting block upload")

	if msg := c.sanitizeMeta(logger, tenantID, blockID, meta); msg != "" {
		return httpError{
			message:    msg,
			statusCode: http.StatusBadRequest,
		}
	}

	// validate data is within the retention period
	retention := c.cfgProvider.CompactorBlocksRetentionPeriod(tenantID)
	if retention > 0 {
		threshold := time.Now().Add(-retention)
		if time.UnixMilli(meta.MaxTime).Before(threshold) {
			maxTimeStr := util.FormatTimeMillis(meta.MaxTime)
			return httpError{
				message:    fmt.Sprintf("block max time (%s) older than retention period", maxTimeStr),
				statusCode: http.StatusUnprocessableEntity,
			}
		}
	}

	return c.uploadMeta(ctx, logger, meta, blockID, uploadingMetaFilename, userBkt)
}

// UploadBlockFile handles requests for uploading block files.
// It takes the mandatory query parameter "path", specifying the file's destination path.
func (c *MultitenantCompactor) UploadBlockFile(w http.ResponseWriter, r *http.Request) {
	blockID, tenantID, err := c.parseBlockUploadParameters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	requestID := hexTimeNowNano()
	logger := log.With(
		util_log.WithContext(ctx, c.logger),
		"feature", "block upload",
		"block", blockID,
		"operation", "block file upload",
		"request", requestID,
	)

	pth := r.URL.Query().Get("path")
	if pth == "" {
		err := httpError{statusCode: http.StatusBadRequest, message: "missing or invalid file path"}
		writeBlockUploadError(err, "failed because file path is empty", logger, w, requestID)
		return
	}

	if path.Base(pth) == block.MetaFilename {
		err := httpError{statusCode: http.StatusBadRequest, message: fmt.Sprintf("%s is not allowed", block.MetaFilename)}
		writeBlockUploadError(err, "failed because block meta is not allowed", logger, w, requestID)
		return
	}

	if !rePath.MatchString(pth) {
		err := httpError{statusCode: http.StatusBadRequest, message: fmt.Sprintf("invalid path: %q", pth)}
		writeBlockUploadError(err, "failed because path is invalid", logger, w, requestID)
		return
	}

	if r.ContentLength == 0 {
		err := httpError{statusCode: http.StatusBadRequest, message: "file cannot be empty"}
		writeBlockUploadError(err, "failed because file is empty", logger, w, requestID)
		return
	}

	userBkt := bucket.NewUserBucketClient(tenantID, c.bucketClient, c.cfgProvider)

	m, _, err := c.checkBlockState(ctx, userBkt, blockID, true)
	if err != nil {
		writeBlockUploadError(err, "can't check block state", logger, w, requestID)
		return
	}

	// This should not happen.
	if m == nil {
		err := httpError{statusCode: http.StatusInternalServerError, message: "internal error"}
		writeBlockUploadError(err, "block meta is nil but err is also nil", logger, w, requestID)
		return
	}

	// Check if file was specified in meta.json, and if it has expected size.
	found := false
	for _, f := range m.Thanos.Files {
		if pth == f.RelPath {
			found = true

			if r.ContentLength >= 0 && r.ContentLength != f.SizeBytes {
				err := httpError{statusCode: http.StatusBadRequest, message: fmt.Sprintf("file size doesn't match %s", block.MetaFilename)}
				writeBlockUploadError(err, "failed because file size didn't match", logger, w, requestID)
				return
			}
		}
	}
	if !found {
		err := httpError{statusCode: http.StatusBadRequest, message: "unexpected file"}
		writeBlockUploadError(err, "failed because file was not found", logger, w, requestID)
		return
	}

	dst := path.Join(blockID.String(), pth)

	level.Debug(logger).Log("msg", "uploading block file to bucket", "destination", dst, "size", r.ContentLength)
	reader := bodyReader{r: r}
	if err := userBkt.Upload(ctx, dst, reader); err != nil {
		// We don't know what caused the error; it could be the client's fault (e.g. killed
		// connection), but internal server error is the safe choice here.
		level.Error(logger).Log("msg", "failed uploading block file to bucket", "destination", dst, "err", err)
		http.Error(w, fmt.Sprintf("internal server error (id %s)", requestID), http.StatusInternalServerError)
		return
	}

	level.Debug(logger).Log("msg", "finished uploading block file to bucket", "path", pth)

	w.WriteHeader(http.StatusOK)
}

func (c *MultitenantCompactor) validateAndCompleteBlockUpload(logger log.Logger, tenantID string, userBkt objstore.Bucket, blockID ulid.ULID, meta *block.Meta, validation func(context.Context) error) {
	level.Debug(logger).Log("msg", "completing block upload", "files", len(meta.Thanos.Files))

	{
		var wg sync.WaitGroup
		ctx, cancel := context.WithCancel(context.Background())

		// start a go routine that updates the validation file's timestamp every heartbeat interval
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.periodicValidationUpdater(ctx, logger, blockID, userBkt, cancel, validationHeartbeatInterval)
		}()

		if err := validation(ctx); err != nil {
			level.Error(logger).Log("msg", "error while validating block", "err", err)
			cancel()
			wg.Wait()
			err := c.uploadValidationWithError(context.Background(), blockID, userBkt, err.Error())
			if err != nil {
				level.Error(logger).Log("msg", "error updating validation file after failed block validation", "err", err)
			}
			return
		}

		cancel()
		wg.Wait() // use waitgroup to ensure validation ts update is complete
	}

	ctx := context.Background()

	if err := c.markBlockComplete(ctx, logger, tenantID, userBkt, blockID, meta); err != nil {
		if err := c.uploadValidationWithError(ctx, blockID, userBkt, err.Error()); err != nil {
			level.Error(logger).Log("msg", "error updating validation file after upload of metadata file failed", "err", err)
		}
		return
	}

	if err := userBkt.Delete(ctx, path.Join(blockID.String(), validationFilename)); err != nil {
		level.Warn(logger).Log("msg", fmt.Sprintf(
			"failed to delete %s from block in object storage", validationFilename), "err", err)
		return
	}

	level.Info(logger).Log("msg", "successfully completed block upload")
}

func (c *MultitenantCompactor) markBlockComplete(ctx context.Context, logger log.Logger, tenantID string, userBkt objstore.Bucket, blockID ulid.ULID, meta *block.Meta) error {
	if err := c.uploadMeta(ctx, logger, meta, blockID, block.MetaFilename, userBkt); err != nil {
		level.Error(logger).Log("msg", "error uploading block metadata file", "err", err)
		return err
	}

	if err := userBkt.Delete(ctx, path.Join(blockID.String(), uploadingMetaFilename)); err != nil {
		// Not returning an error since the temporary meta file persisting is a harmless side effect
		level.Warn(logger).Log("msg", fmt.Sprintf("failed to delete %s from block in object storage", uploadingMetaFilename), "err", err)
	}

	// Increment metrics on successful block upload
	c.blockUploadBlocks.WithLabelValues(tenantID).Inc()
	c.blockUploadBytes.WithLabelValues(tenantID).Add(float64(meta.BlockBytes()))
	c.blockUploadFiles.WithLabelValues(tenantID).Add(float64(len(meta.Thanos.Files)))

	return nil
}

// sanitizeMeta sanitizes and validates a metadata.Meta object. If a validation error occurs, an error
// message gets returned, otherwise an empty string.
func (c *MultitenantCompactor) sanitizeMeta(logger log.Logger, userID string, blockID ulid.ULID, meta *block.Meta) string {
	if meta == nil {
		return "missing block metadata"
	}

	// check that the blocks doesn't contain down-sampled data
	if meta.Thanos.Downsample.Resolution > 0 {
		return "block contains downsampled data"
	}

	meta.ULID = blockID
	for l, v := range meta.Thanos.Labels {
		switch l {
		// Preserve this label
		case mimir_tsdb.CompactorShardIDExternalLabel:
			if v == "" {
				level.Debug(logger).Log("msg", "removing empty external label",
					"label", l)
				delete(meta.Thanos.Labels, l)
				continue
			}

			if _, _, err := sharding.ParseShardIDLabelValue(v); err != nil {
				return fmt.Sprintf("invalid %s external label: %q",
					mimir_tsdb.CompactorShardIDExternalLabel, v)
			}
		// Remove unused labels
		case mimir_tsdb.DeprecatedTenantIDExternalLabel, mimir_tsdb.DeprecatedIngesterIDExternalLabel, mimir_tsdb.DeprecatedShardIDExternalLabel:
			level.Debug(logger).Log("msg", "removing unused external label",
				"label", l, "value", v)
			delete(meta.Thanos.Labels, l)
		default:
			return fmt.Sprintf("unsupported external label: %s", l)
		}
	}

	meta.Compaction.Parents = nil
	meta.Compaction.Sources = []ulid.ULID{blockID}

	for _, f := range meta.Thanos.Files {
		if f.RelPath == block.MetaFilename {
			continue
		}

		if !rePath.MatchString(f.RelPath) {
			return fmt.Sprintf("file with invalid path: %s", f.RelPath)
		}

		if f.SizeBytes <= 0 {
			return fmt.Sprintf("file with invalid size: %s", f.RelPath)
		}
	}

	if err := c.validateMaximumBlockSize(logger, meta.Thanos.Files, userID); err != nil {
		return err.Error()
	}

	if meta.Version != block.TSDBVersion1 {
		return fmt.Sprintf("version must be %d", block.TSDBVersion1)
	}

	// validate minTime/maxTime
	// basic sanity check
	if meta.MinTime < 0 || meta.MaxTime < 0 || meta.MaxTime < meta.MinTime {
		return fmt.Sprintf("invalid minTime/maxTime: minTime=%d, maxTime=%d",
			meta.MinTime, meta.MaxTime)
	}
	// validate that times are in the past
	now := time.Now()
	if meta.MinTime > now.UnixMilli() || meta.MaxTime > now.UnixMilli() {
		return fmt.Sprintf("block time(s) greater than the present: minTime=%d, maxTime=%d",
			meta.MinTime, meta.MaxTime)
	}

	// Mark block source
	meta.Thanos.Source = "upload"

	return ""
}

func (c *MultitenantCompactor) uploadMeta(ctx context.Context, logger log.Logger, meta *block.Meta, blockID ulid.ULID, name string, userBkt objstore.Bucket) error {
	if meta == nil {
		return errors.New("missing block metadata")
	}
	dst := path.Join(blockID.String(), name)
	level.Debug(logger).Log("msg", fmt.Sprintf("uploading %s to bucket", name), "dst", dst)
	buf := bytes.NewBuffer(nil)
	if err := json.NewEncoder(buf).Encode(meta); err != nil {
		return errors.Wrap(err, "failed to encode block metadata")
	}
	if err := userBkt.Upload(ctx, dst, buf); err != nil {
		return errors.Wrapf(err, "failed uploading %s to bucket", name)
	}

	return nil
}

func (c *MultitenantCompactor) createTemporaryBlockDirectory() (dir string, err error) {
	blockDir, err := os.MkdirTemp(c.compactorCfg.DataDir, "upload")
	if err != nil {
		level.Error(c.logger).Log("msg", "failed to create temporary block directory", "err", err)
		return "", errors.New("failed to create temporary block directory")
	}

	level.Debug(c.logger).Log("msg", "created temporary block directory", "dir", blockDir)
	return blockDir, nil
}

func (c *MultitenantCompactor) removeTemporaryBlockDirectory(blockDir string) {
	level.Debug(c.logger).Log("msg", "removing temporary block directory", "dir", blockDir)
	if err := os.RemoveAll(blockDir); err != nil {
		level.Warn(c.logger).Log("msg", "failed to remove temporary block directory", "path", blockDir, "err", err)
	}
}

func (c *MultitenantCompactor) prepareBlockForValidation(ctx context.Context, userBkt objstore.Bucket, blockID ulid.ULID) (string, error) {
	blockDir, err := c.createTemporaryBlockDirectory()
	if err != nil {
		return "", err
	}

	// download the block to local storage
	level.Debug(c.logger).Log("msg", "downloading block from bucket", "block", blockID.String())
	err = objstore.DownloadDir(ctx, c.logger, userBkt, blockID.String(), blockID.String(), blockDir)
	if err != nil {
		c.removeTemporaryBlockDirectory(blockDir)
		return "", errors.Wrap(err, "failed to download block")
	}

	// rename the temporary meta file name to the expected one locally so that the block can be inspected
	err = os.Rename(filepath.Join(blockDir, uploadingMetaFilename), filepath.Join(blockDir, block.MetaFilename))
	if err != nil {
		level.Warn(c.logger).Log("msg", "could not rename temporary metadata file", "block", blockID.String(), "err", err)
		c.removeTemporaryBlockDirectory(blockDir)
		return "", errors.New("failed renaming while preparing block for validation")
	}

	return blockDir, nil
}

func (c *MultitenantCompactor) validateBlock(ctx context.Context, logger log.Logger, blockID ulid.ULID, blockMetadata *block.Meta, userBkt objstore.Bucket, userID string) error {
	if err := c.validateMaximumBlockSize(logger, blockMetadata.Thanos.Files, userID); err != nil {
		return err
	}

	blockDir, err := c.prepareBlockForValidation(ctx, userBkt, blockID)
	if err != nil {
		return err
	}
	defer c.removeTemporaryBlockDirectory(blockDir)

	// check that all files listed in the metadata are present and the correct size
	for _, f := range blockMetadata.Thanos.Files {
		fi, err := os.Stat(filepath.Join(blockDir, filepath.FromSlash(f.RelPath)))
		if err != nil {
			return errors.Wrapf(err, "failed to stat %s", f.RelPath)
		}

		if !fi.Mode().IsRegular() {
			return errors.Errorf("not a file: %s", f.RelPath)
		}

		if f.RelPath != block.MetaFilename && fi.Size() != f.SizeBytes {
			return errors.Errorf("file size mismatch for %s", f.RelPath)
		}
	}

	// validate block
	checkChunks := c.cfgProvider.CompactorBlockUploadVerifyChunks(userID)
	err = block.VerifyBlock(c.logger, blockDir, blockMetadata.MinTime, blockMetadata.MaxTime, checkChunks)
	if err != nil {
		return errors.Wrap(err, "error validating block")
	}

	return nil
}

func (c *MultitenantCompactor) validateMaximumBlockSize(logger log.Logger, files []block.File, userID string) error {
	maxBlockSizeBytes := c.cfgProvider.CompactorBlockUploadMaxBlockSizeBytes(userID)
	if maxBlockSizeBytes <= 0 {
		return nil
	}

	blockSizeBytes := int64(0)
	for _, f := range files {
		if f.SizeBytes < 0 {
			return errors.New("invalid negative file size in block metadata")
		}
		blockSizeBytes += f.SizeBytes
		if blockSizeBytes < 0 {
			// overflow
			break
		}
	}

	if blockSizeBytes > maxBlockSizeBytes || blockSizeBytes < 0 {
		level.Error(logger).Log("msg", "rejecting block upload for exceeding maximum size", "limit", maxBlockSizeBytes, "size", blockSizeBytes)
		return fmt.Errorf(maxBlockUploadSizeBytesFormat, maxBlockSizeBytes)
	}
	return nil
}

type httpError struct {
	message    string
	statusCode int
}

func (e httpError) Error() string {
	return e.message
}

type bodyReader struct {
	r *http.Request
}

// ObjectSize implements thanos.ObjectSizer.
func (r bodyReader) ObjectSize() (int64, error) {
	if r.r.ContentLength < 0 {
		return 0, fmt.Errorf("unknown size")
	}

	return r.r.ContentLength, nil
}

// Read implements io.Reader.
func (r bodyReader) Read(b []byte) (int, error) {
	return r.r.Body.Read(b)
}

type validationFile struct {
	LastUpdate int64  // UnixMillis of last update time.
	Error      string // Error message if validation failed.
}

type blockUploadStateResult struct {
	State string `json:"result"`
	Error string `json:"error,omitempty"`
}

type blockUploadState int

const (
	blockStateUnknown         blockUploadState = iota // unknown, default value
	blockIsComplete                                   // meta.json file exists
	blockUploadNotStarted                             // meta.json doesn't exist, uploading-meta.json doesn't exist
	blockUploadInProgress                             // meta.json doesn't exist, but uploading-meta.json does
	blockValidationInProgress                         // meta.json doesn't exist, uploading-meta.json exists, validation.json exists and is recent
	blockValidationFailed
	blockValidationStale
)

func (c *MultitenantCompactor) GetBlockUploadStateHandler(w http.ResponseWriter, r *http.Request) {
	blockID, tenantID, err := c.parseBlockUploadParameters(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	requestID := hexTimeNowNano()
	logger := log.With(
		util_log.WithContext(r.Context(), c.logger),
		"feature", "block upload",
		"block", blockID,
		"operation", "get block state",
		"request_id", requestID,
	)

	userBkt := bucket.NewUserBucketClient(tenantID, c.bucketClient, c.cfgProvider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s, _, v, err := c.getBlockUploadState(r.Context(), userBkt, blockID)
	if err != nil {
		writeBlockUploadError(err, "can't get upload state", logger, w, requestID)
		return
	}

	res := blockUploadStateResult{}

	switch s {
	case blockIsComplete:
		res.State = "complete"
	case blockUploadNotStarted:
		http.Error(w, "block doesn't exist", http.StatusNotFound)
		return
	case blockValidationStale:
		fallthrough
	case blockUploadInProgress:
		res.State = "uploading"
	case blockValidationInProgress:
		res.State = "validating"
	case blockValidationFailed:
		res.State = "failed"
		res.Error = v.Error
	}

	util.WriteJSONResponse(w, res)
}

// checkBlockState checks blocks state and returns various HTTP status codes for individual states if block
// upload cannot start, finish or file cannot be uploaded to the block.
func (c *MultitenantCompactor) checkBlockState(ctx context.Context, userBkt objstore.Bucket, blockID ulid.ULID, requireUploadInProgress bool) (*block.Meta, *validationFile, error) {
	s, m, v, err := c.getBlockUploadState(ctx, userBkt, blockID)
	if err != nil {
		return m, v, err
	}

	switch s {
	case blockIsComplete:
		return m, v, httpError{message: "block already exists", statusCode: http.StatusConflict}
	case blockValidationInProgress:
		return m, v, httpError{message: "block validation in progress", statusCode: http.StatusBadRequest}
	case blockUploadNotStarted:
		if requireUploadInProgress {
			return m, v, httpError{message: "block upload not started", statusCode: http.StatusNotFound}
		}
		return m, v, nil
	case blockValidationStale:
		// if validation is stale, we treat block as being in "upload in progress" state, and validation can start again.
		fallthrough
	case blockUploadInProgress:
		return m, v, nil
	case blockValidationFailed:
		return m, v, httpError{message: "block validation failed", statusCode: http.StatusBadRequest}
	}

	return m, v, httpError{message: "unknown block upload state", statusCode: http.StatusInternalServerError}
}

// getBlockUploadState returns state of the block upload, and meta and validation objects, if they exist.
func (c *MultitenantCompactor) getBlockUploadState(ctx context.Context, userBkt objstore.Bucket, blockID ulid.ULID) (blockUploadState, *block.Meta, *validationFile, error) {
	exists, err := userBkt.Exists(ctx, path.Join(blockID.String(), block.MetaFilename))
	if err != nil {
		return blockStateUnknown, nil, nil, err
	}
	if exists {
		return blockIsComplete, nil, nil, nil
	}

	meta, err := c.loadUploadingMeta(ctx, userBkt, blockID)
	if err != nil {
		return blockStateUnknown, nil, nil, err
	}
	// If neither meta.json nor uploading-meta.json file exist, we say that the block doesn't exist.
	if meta == nil {
		return blockUploadNotStarted, nil, nil, err
	}

	v, err := c.loadValidation(ctx, userBkt, blockID)
	if err != nil {
		return blockStateUnknown, meta, nil, err
	}
	if v == nil {
		return blockUploadInProgress, meta, nil, err
	}
	if v.Error != "" {
		return blockValidationFailed, meta, v, err
	}
	if time.Since(time.UnixMilli(v.LastUpdate)) < validationHeartbeatTimeout {
		return blockValidationInProgress, meta, v, nil
	}
	return blockValidationStale, meta, v, nil
}

func (c *MultitenantCompactor) loadUploadingMeta(ctx context.Context, userBkt objstore.Bucket, blockID ulid.ULID) (*block.Meta, error) {
	r, err := userBkt.Get(ctx, path.Join(blockID.String(), uploadingMetaFilename))
	if err != nil {
		if userBkt.IsObjNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = r.Close() }()

	v := &block.Meta{}
	err = json.NewDecoder(r).Decode(v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (c *MultitenantCompactor) loadValidation(ctx context.Context, userBkt objstore.Bucket, blockID ulid.ULID) (*validationFile, error) {
	r, err := userBkt.Get(ctx, path.Join(blockID.String(), validationFilename))
	if err != nil {
		if userBkt.IsObjNotFoundErr(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = r.Close() }()

	v := &validationFile{}
	err = json.NewDecoder(r).Decode(v)
	if err != nil {
		return nil, err
	}

	return v, nil
}

func (c *MultitenantCompactor) uploadValidationWithError(ctx context.Context, blockID ulid.ULID,
	userBkt objstore.Bucket, errorStr string,
) error {
	val := validationFile{
		LastUpdate: time.Now().UnixMilli(),
		Error:      errorStr,
	}
	dst := path.Join(blockID.String(), validationFilename)
	if err := marshalAndUploadToBucket(ctx, userBkt, dst, val); err != nil {
		return errors.Wrapf(err, "failed uploading %s to bucket", validationFilename)
	}
	return nil
}

func (c *MultitenantCompactor) uploadValidation(ctx context.Context, blockID ulid.ULID, userBkt objstore.Bucket) error {
	return c.uploadValidationWithError(ctx, blockID, userBkt, "")
}

func (c *MultitenantCompactor) periodicValidationUpdater(ctx context.Context, logger log.Logger, blockID ulid.ULID, userBkt objstore.Bucket, cancelFn func(), interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.uploadValidation(ctx, blockID, userBkt); err != nil {
				level.Warn(logger).Log("msg", "error during periodic update of validation file", "err", err)
				cancelFn()
				return
			}
		}
	}
}

func marshalAndUploadToBucket(ctx context.Context, bkt objstore.Bucket, pth string, val interface{}) error {
	buf, err := json.Marshal(val)
	if err != nil {
		return err
	}
	if err := bkt.Upload(ctx, pth, bytes.NewReader(buf)); err != nil {
		return err
	}
	return nil
}
