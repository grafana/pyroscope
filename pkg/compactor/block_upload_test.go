// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/grafana/mimir/blob/main/pkg/compactor/block_upload_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package compactor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gorilla/mux"
	"github.com/grafana/dskit/test"
	"github.com/grafana/dskit/user"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"

	"github.com/grafana/mimir/pkg/storage/bucket"
	mimir_tsdb "github.com/grafana/mimir/pkg/storage/tsdb"
	"github.com/grafana/mimir/pkg/storage/tsdb/block"
)

func verifyUploadedMeta(t *testing.T, bkt *bucket.ClientMock, expMeta block.Meta) {
	var call mock.Call
	for _, c := range bkt.Calls {
		if c.Method == "Upload" {
			call = c
			break
		}
	}

	rdr := call.Arguments[2].(io.Reader)
	var gotMeta block.Meta
	require.NoError(t, json.NewDecoder(rdr).Decode(&gotMeta))
	assert.Equal(t, expMeta, gotMeta)
}

// Test MultitenantCompactor.StartBlockUpload
func TestMultitenantCompactor_StartBlockUpload(t *testing.T) {
	const tenantID = "test"
	const blockID = "01G3FZ0JWJYJC0ZM6Y9778P6KD"
	bULID := ulid.MustParse(blockID)
	now := time.Now().UnixMilli()
	validMeta := block.Meta{
		BlockMeta: tsdb.BlockMeta{
			ULID:    bULID,
			Version: block.TSDBVersion1,
			MinTime: now - 1000,
			MaxTime: now,
		},
		Thanos: block.ThanosMeta{
			Labels: map[string]string{
				mimir_tsdb.CompactorShardIDExternalLabel: "1_of_3",
			},
			Files: []block.File{
				{
					RelPath: block.MetaFilename,
				},
				{
					RelPath:   "index",
					SizeBytes: 1,
				},
				{
					RelPath:   "chunks/000001",
					SizeBytes: 1024,
				},
			},
		},
	}

	metaPath := path.Join(tenantID, blockID, block.MetaFilename)
	uploadingMetaPath := path.Join(tenantID, blockID, fmt.Sprintf("uploading-%s", block.MetaFilename))

	setUpPartialBlock := func(bkt *bucket.ClientMock) {
		bkt.MockExists(path.Join(tenantID, blockID, block.MetaFilename), false, nil)
		setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), nil, bucket.ErrObjectDoesNotExist)
	}
	setUpUpload := func(bkt *bucket.ClientMock) {
		setUpPartialBlock(bkt)
		bkt.MockUpload(uploadingMetaPath, nil)
	}

	verifyUpload := func(t *testing.T, bkt *bucket.ClientMock, labels map[string]string) {
		t.Helper()

		expMeta := validMeta
		expMeta.Compaction.Parents = nil
		expMeta.Compaction.Sources = []ulid.ULID{expMeta.ULID}
		expMeta.Thanos.Source = "upload"
		expMeta.Thanos.Labels = labels
		verifyUploadedMeta(t, bkt, expMeta)
	}

	testCases := []struct {
		name                    string
		tenantID                string
		blockID                 string
		body                    string
		meta                    *block.Meta
		retention               time.Duration
		disableBlockUpload      bool
		expBadRequest           string
		expConflict             string
		expUnprocessableEntity  string
		expEntityTooLarge       string
		expInternalServerError  bool
		setUpBucketMock         func(bkt *bucket.ClientMock)
		verifyUpload            func(*testing.T, *bucket.ClientMock)
		maxBlockUploadSizeBytes int64
	}{
		{
			name:          "missing tenant ID",
			tenantID:      "",
			blockID:       blockID,
			expBadRequest: "invalid tenant ID",
		},
		{
			name:          "missing block ID",
			tenantID:      tenantID,
			blockID:       "",
			expBadRequest: "invalid block ID",
		},
		{
			name:          "invalid block ID",
			tenantID:      tenantID,
			blockID:       "1234",
			expBadRequest: "invalid block ID",
		},
		{
			name:            "missing body",
			tenantID:        tenantID,
			blockID:         blockID,
			expBadRequest:   "malformed request body",
			setUpBucketMock: setUpPartialBlock,
		},
		{
			name:            "malformed body",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			body:            "{",
			expBadRequest:   "malformed request body",
		},
		{
			name:            "invalid file path",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				Thanos: block.ThanosMeta{
					Files: []block.File{
						{
							RelPath:   "chunks/invalid-file",
							SizeBytes: 1024,
						},
					},
				},
			},
			expBadRequest: "file with invalid path: chunks/invalid-file",
		},
		{
			name:            "contains downsampled data",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				Thanos: block.ThanosMeta{
					Downsample: block.ThanosDownsample{
						Resolution: 1000,
					},
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath: "chunks/000001",
						},
					},
				},
			},
			expBadRequest: "block contains downsampled data",
		},
		{
			name:            "missing file size",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				Thanos: block.ThanosMeta{
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath: "chunks/000001",
						},
					},
				},
			},
			expBadRequest: "file with invalid size: chunks/000001",
		},
		{
			name:            "invalid minTime",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: -1,
					MaxTime: 0,
				},
			},
			expBadRequest: "invalid minTime/maxTime: minTime=-1, maxTime=0",
		},
		{
			name:            "invalid maxTime",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: 0,
					MaxTime: -1,
				},
			},
			expBadRequest: "invalid minTime/maxTime: minTime=0, maxTime=-1",
		},
		{
			name:            "maxTime before minTime",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: 1,
					MaxTime: 0,
				},
			},
			expBadRequest: "invalid minTime/maxTime: minTime=1, maxTime=0",
		},
		{
			name:            "block before retention period",
			tenantID:        tenantID,
			blockID:         blockID,
			retention:       10 * time.Second,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: 0,
					MaxTime: 1000,
				},
			},
			expUnprocessableEntity: "block max time (1970-01-01 00:00:01 +0000 UTC) older than retention period",
		},
		{
			name:            "invalid version",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: 0,
				},
			},
			expBadRequest: fmt.Sprintf("version must be %d", block.TSDBVersion1),
		},
		{
			name:            "ignore retention period if == 0",
			tenantID:        tenantID,
			blockID:         blockID,
			retention:       0,
			setUpBucketMock: setUpUpload,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: 0,
					MaxTime: 1000,
				},
				Thanos: block.ThanosMeta{
					Labels: map[string]string{
						mimir_tsdb.CompactorShardIDExternalLabel: "1_of_3",
					},
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath:   "chunks/000001",
							SizeBytes: 1024,
						},
					},
				},
			},
		},
		{
			name:            "ignore retention period if < 0",
			tenantID:        tenantID,
			blockID:         blockID,
			retention:       -1,
			setUpBucketMock: setUpUpload,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: 0,
					MaxTime: 1000,
				},
				Thanos: block.ThanosMeta{
					Labels: map[string]string{
						mimir_tsdb.CompactorShardIDExternalLabel: "1_of_3",
					},
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath:   "chunks/000001",
							SizeBytes: 1024,
						},
					},
				},
			},
		},
		{
			name:            "invalid compactor shard ID label",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpPartialBlock,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
				},
				Thanos: block.ThanosMeta{
					Labels: map[string]string{
						mimir_tsdb.CompactorShardIDExternalLabel: "test",
					},
				},
			},
			expBadRequest: fmt.Sprintf(`invalid %s external label: "test"`, mimir_tsdb.CompactorShardIDExternalLabel),
		},
		{
			name:     "failure checking for complete block",
			tenantID: tenantID,
			blockID:  blockID,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(path.Join(tenantID, blockID, block.MetaFilename), false, fmt.Errorf("test"))
			},
			expInternalServerError: true,
		},
		{
			name:     "complete block already exists",
			tenantID: tenantID,
			blockID:  blockID,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(path.Join(tenantID, blockID, block.MetaFilename), true, nil)
			},
			expConflict: "block already exists",
		},
		{
			name:     "failure uploading meta file",
			tenantID: tenantID,
			blockID:  blockID,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				setUpPartialBlock(bkt)
				bkt.MockUpload(uploadingMetaPath, fmt.Errorf("test"))
			},
			meta:                   &validMeta,
			expInternalServerError: true,
		},
		{
			name:              "too large of a request body",
			tenantID:          tenantID,
			blockID:           blockID,
			setUpBucketMock:   setUpPartialBlock,
			body:              strings.Repeat("A", maximumMetaSizeBytes+1),
			expEntityTooLarge: fmt.Sprintf("The block metadata was too large (maximum size allowed is %d bytes)", maximumMetaSizeBytes),
		},
		{
			name:               "block upload disabled",
			tenantID:           tenantID,
			blockID:            blockID,
			disableBlockUpload: true,
			expBadRequest:      "block upload is disabled",
		},
		{
			name:                    "max block size exceeded",
			tenantID:                tenantID,
			blockID:                 blockID,
			setUpBucketMock:         setUpPartialBlock,
			meta:                    &validMeta,
			maxBlockUploadSizeBytes: 1,
			expBadRequest:           fmt.Sprintf(maxBlockUploadSizeBytesFormat, 1),
		},
		{
			name:            "valid request",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpUpload,
			meta:            &validMeta,
			verifyUpload: func(t *testing.T, bkt *bucket.ClientMock) {
				verifyUpload(t, bkt, map[string]string{
					mimir_tsdb.CompactorShardIDExternalLabel: "1_of_3",
				})
			},
		},
		{
			name:            "valid request with empty compactor shard ID label",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpUpload,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: now - 1000,
					MaxTime: now,
				},
				Thanos: block.ThanosMeta{
					Labels: map[string]string{
						mimir_tsdb.CompactorShardIDExternalLabel: "",
					},
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath:   "chunks/000001",
							SizeBytes: 1024,
						},
					},
				},
			},
			verifyUpload: func(t *testing.T, bkt *bucket.ClientMock) {
				verifyUpload(t, bkt, map[string]string{})
			},
		},
		{
			name:            "valid request without compactor shard ID label",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpUpload,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    bULID,
					Version: block.TSDBVersion1,
					MinTime: now - 1000,
					MaxTime: now,
				},
				Thanos: block.ThanosMeta{
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath:   "chunks/000001",
							SizeBytes: 1024,
						},
					},
				},
			},
			verifyUpload: func(t *testing.T, bkt *bucket.ClientMock) {
				verifyUpload(t, bkt, nil)
			},
		},
		{
			name:            "valid request with different block ID in meta file",
			tenantID:        tenantID,
			blockID:         blockID,
			setUpBucketMock: setUpUpload,
			meta: &block.Meta{
				BlockMeta: tsdb.BlockMeta{
					ULID:    ulid.MustParse("11A2FZ0JWJYJC0ZM6Y9778P6KD"),
					Version: block.TSDBVersion1,
					MinTime: now - 1000,
					MaxTime: now,
				},
				Thanos: block.ThanosMeta{
					Files: []block.File{
						{
							RelPath: block.MetaFilename,
						},
						{
							RelPath:   "index",
							SizeBytes: 1,
						},
						{
							RelPath:   "chunks/000001",
							SizeBytes: 1024,
						},
					},
				},
			},
			verifyUpload: func(t *testing.T, bkt *bucket.ClientMock) {
				verifyUpload(t, bkt, nil)
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bkt bucket.ClientMock
			if tc.setUpBucketMock != nil {
				tc.setUpBucketMock(&bkt)
			}

			cfgProvider := newMockConfigProvider()
			cfgProvider.userRetentionPeriods[tenantID] = tc.retention
			cfgProvider.blockUploadEnabled[tenantID] = !tc.disableBlockUpload
			cfgProvider.blockUploadMaxBlockSizeBytes[tenantID] = tc.maxBlockUploadSizeBytes
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: &bkt,
				cfgProvider:  cfgProvider,
			}
			var rdr io.Reader
			if tc.body != "" {
				rdr = strings.NewReader(tc.body)
			} else if tc.meta != nil {
				buf := bytes.NewBuffer(nil)
				require.NoError(t, json.NewEncoder(buf).Encode(tc.meta))
				rdr = buf
			}
			r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/upload/block/%s/start", tc.blockID), rdr)
			if tc.tenantID != "" {
				r = r.WithContext(user.InjectOrgID(r.Context(), tc.tenantID))
			}
			if tc.blockID != "" {
				r = mux.SetURLVars(r, map[string]string{"block": tc.blockID})
			}
			w := httptest.NewRecorder()
			c.StartBlockUpload(w, r)

			resp := w.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			switch {
			case tc.expInternalServerError:
				assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
				assert.Regexp(t, "internal server error \\(id [0-9a-f]{16}\\)\n", string(body))
			case tc.expBadRequest != "":
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expBadRequest), string(body))
			case tc.expConflict != "":
				assert.Equal(t, http.StatusConflict, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expConflict), string(body))
			case tc.expUnprocessableEntity != "":
				assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expUnprocessableEntity), string(body))
			case tc.expEntityTooLarge != "":
				assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expEntityTooLarge), string(body))
			default:
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Empty(t, string(body))
			}

			bkt.AssertExpectations(t)

			if tc.verifyUpload != nil {
				tc.verifyUpload(t, &bkt)
			}
		})
	}

	downloadMeta := func(t *testing.T, bkt *objstore.InMemBucket, pth string) block.Meta {
		t.Helper()

		ctx := context.Background()
		rdr, err := bkt.Get(ctx, pth)
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = rdr.Close()
		})
		var gotMeta block.Meta
		require.NoError(t, json.NewDecoder(rdr).Decode(&gotMeta))
		return gotMeta
	}

	// Additional test cases using an in-memory bucket for state testing
	extraCases := []struct {
		name          string
		setUp         func(*testing.T, *objstore.InMemBucket) block.Meta
		verifyBucket  func(*testing.T, *objstore.InMemBucket)
		expBadRequest string
		expConflict   string
	}{
		{
			name: "valid request when both in-flight meta file and complete meta file exist in object storage",
			setUp: func(t *testing.T, bkt *objstore.InMemBucket) block.Meta {
				marshalAndUploadJSON(t, bkt, uploadingMetaPath, validMeta)
				marshalAndUploadJSON(t, bkt, metaPath, validMeta)
				return validMeta
			},
			verifyBucket: func(t *testing.T, bkt *objstore.InMemBucket) {
				assert.Equal(t, validMeta, downloadMeta(t, bkt, uploadingMetaPath))
				assert.Equal(t, validMeta, downloadMeta(t, bkt, metaPath))
			},
			expConflict: "block already exists",
		},
		{
			name: "invalid request when in-flight meta file exists in object storage",
			setUp: func(t *testing.T, bkt *objstore.InMemBucket) block.Meta {
				marshalAndUploadJSON(t, bkt, uploadingMetaPath, validMeta)

				meta := validMeta
				// Invalid version
				meta.Version = 0
				return meta
			},
			verifyBucket: func(t *testing.T, bkt *objstore.InMemBucket) {
				assert.Equal(t, validMeta, downloadMeta(t, bkt, uploadingMetaPath))
			},
			expBadRequest: fmt.Sprintf("version must be %d", block.TSDBVersion1),
		},
		{
			name: "valid request when same in-flight meta file exists in object storage",
			setUp: func(t *testing.T, bkt *objstore.InMemBucket) block.Meta {
				marshalAndUploadJSON(t, bkt, uploadingMetaPath, validMeta)
				return validMeta
			},
			verifyBucket: func(t *testing.T, bkt *objstore.InMemBucket) {
				expMeta := validMeta
				expMeta.Compaction.Sources = []ulid.ULID{expMeta.ULID}
				expMeta.Thanos.Source = "upload"
				assert.Equal(t, expMeta, downloadMeta(t, bkt, uploadingMetaPath))
			},
		},
		{
			name: "valid request when different in-flight meta file exists in object storage",
			setUp: func(t *testing.T, bkt *objstore.InMemBucket) block.Meta {
				meta := validMeta
				meta.MinTime -= 1000
				meta.MaxTime -= 1000
				marshalAndUploadJSON(t, bkt, uploadingMetaPath, meta)

				// Return meta file that differs from the one in bucket
				return validMeta
			},
			verifyBucket: func(t *testing.T, bkt *objstore.InMemBucket) {
				expMeta := validMeta
				expMeta.Compaction.Sources = []ulid.ULID{expMeta.ULID}
				expMeta.Thanos.Source = "upload"
				assert.Equal(t, expMeta, downloadMeta(t, bkt, uploadingMetaPath))
			},
		},
	}
	for _, tc := range extraCases {
		t.Run(tc.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			meta := tc.setUp(t, bkt)
			metaJSON, err := json.Marshal(meta)
			require.NoError(t, err)

			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadEnabled[tenantID] = true
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: bkt,
				cfgProvider:  cfgProvider,
			}
			r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/upload/block/%s/start", blockID), bytes.NewReader(metaJSON))
			r = r.WithContext(user.InjectOrgID(r.Context(), tenantID))
			r = mux.SetURLVars(r, map[string]string{"block": blockID})
			w := httptest.NewRecorder()
			c.StartBlockUpload(w, r)

			resp := w.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			switch {
			case tc.expBadRequest != "":
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expBadRequest), string(body))
			case tc.expConflict != "":
				assert.Equal(t, http.StatusConflict, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expConflict), string(body))
			default:
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Empty(t, string(body))
			}
		})
	}
}

// Test MultitenantCompactor.UploadBlockFile
func TestMultitenantCompactor_UploadBlockFile(t *testing.T) {
	const tenantID = "test"
	const blockID = "01G3FZ0JWJYJC0ZM6Y9778P6KD"
	uploadingMetaFilename := fmt.Sprintf("uploading-%s", block.MetaFilename)
	uploadingMetaPath := path.Join(tenantID, blockID, uploadingMetaFilename)
	metaPath := path.Join(tenantID, blockID, block.MetaFilename)

	chunkBodyContent := "content"
	validMeta := block.Meta{
		BlockMeta: tsdb.BlockMeta{
			ULID: ulid.MustParse(blockID),
		},
		Thanos: block.ThanosMeta{
			Labels: map[string]string{
				mimir_tsdb.CompactorShardIDExternalLabel: "1_of_3",
			},
			Files: []block.File{
				{
					RelPath:   "index",
					SizeBytes: 1,
				},
				{
					RelPath:   "chunks/000001",
					SizeBytes: int64(len(chunkBodyContent)),
				},
			},
		},
	}

	setupFnForValidRequest := func(bkt *bucket.ClientMock) {
		bkt.MockExists(metaPath, false, nil)

		b, err := json.Marshal(validMeta)
		setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), b, err)
		setUpGet(bkt, path.Join(tenantID, blockID, validationFilename), nil, bucket.ErrObjectDoesNotExist)

		bkt.MockUpload(path.Join(tenantID, blockID, "chunks/000001"), nil)
	}

	verifyFuncForValidRequest := func(t *testing.T, bkt *bucket.ClientMock, expContent string) {
		var call mock.Call
		for _, c := range bkt.Calls {
			if c.Method == "Upload" {
				call = c
				break
			}
		}

		rdr := call.Arguments[2].(io.Reader)
		got, err := io.ReadAll(rdr)
		require.NoError(t, err)
		assert.Equal(t, []byte(expContent), got)
	}

	testCases := []struct {
		name                   string
		tenantID               string
		blockID                string
		path                   string
		body                   string
		unknownContentLength   bool
		disableBlockUpload     bool
		expBadRequest          string
		expConflict            string
		expNotFound            string
		expInternalServerError bool
		setUpBucketMock        func(bkt *bucket.ClientMock)
		verifyUpload           func(*testing.T, *bucket.ClientMock, string)
	}{
		{
			name:          "without tenant ID",
			blockID:       blockID,
			path:          "chunks/000001",
			expBadRequest: "invalid tenant ID",
		},
		{
			name:          "without block ID",
			tenantID:      tenantID,
			path:          "chunks/000001",
			expBadRequest: "invalid block ID",
		},
		{
			name:          "invalid block ID",
			tenantID:      tenantID,
			blockID:       "1234",
			path:          "chunks/000001",
			expBadRequest: "invalid block ID",
		},
		{
			name:          "without path",
			tenantID:      tenantID,
			blockID:       blockID,
			expBadRequest: "missing or invalid file path",
		},
		{
			name:          "invalid path",
			tenantID:      tenantID,
			blockID:       blockID,
			path:          "../chunks/000001",
			expBadRequest: `invalid path: "../chunks/000001"`,
		},
		{
			name:          "empty file",
			tenantID:      tenantID,
			blockID:       blockID,
			path:          "chunks/000001",
			expBadRequest: "file cannot be empty",
		},
		{
			name:          "attempt block metadata file",
			tenantID:      tenantID,
			blockID:       blockID,
			path:          block.MetaFilename,
			body:          "content",
			expBadRequest: fmt.Sprintf("%s is not allowed", block.MetaFilename),
		},
		{
			name:          "attempt in-flight block metadata file",
			tenantID:      tenantID,
			blockID:       blockID,
			path:          uploadingMetaFilename,
			body:          "content",
			expBadRequest: fmt.Sprintf("invalid path: %q", uploadingMetaFilename),
		},
		{
			name:               "block upload disabled",
			tenantID:           tenantID,
			blockID:            blockID,
			disableBlockUpload: true,
			path:               "chunks/000001",
			expBadRequest:      "block upload is disabled",
		},
		{
			name:     "complete block already exists",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/000001",
			body:     "content",
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, true, nil)
			},
			expConflict: "block already exists",
		},
		{
			name:     "failure checking for complete block",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/000001",
			body:     chunkBodyContent,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, false, fmt.Errorf("test"))
			},
			expInternalServerError: true,
		},
		{
			name:     "failure checking for in-flight meta file",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/000001",
			body:     chunkBodyContent,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, false, nil)
				setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), nil, fmt.Errorf("test"))
			},
			expInternalServerError: true,
		},
		{
			name:     "missing in-flight meta file",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/000001",
			body:     chunkBodyContent,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, false, nil)
				setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), nil, bucket.ErrObjectDoesNotExist)
			},
			expNotFound: "block upload not started",
		},
		{
			name:     "file upload fails",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/000001",
			body:     chunkBodyContent,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, false, nil)

				b, err := json.Marshal(validMeta)
				setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), b, err)
				setUpGet(bkt, path.Join(tenantID, blockID, validationFilename), nil, bucket.ErrObjectDoesNotExist)

				bkt.MockUpload(path.Join(tenantID, blockID, "chunks/000001"), fmt.Errorf("test"))
			},
			expInternalServerError: true,
		},
		{
			name:     "invalid file size",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/000001",
			body:     chunkBodyContent + chunkBodyContent,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, false, nil)

				b, err := json.Marshal(validMeta)
				setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), b, err)
				setUpGet(bkt, path.Join(tenantID, blockID, validationFilename), nil, bucket.ErrObjectDoesNotExist)
			},
			expBadRequest: "file size doesn't match meta.json",
		},
		{
			name:     "unexpected file",
			tenantID: tenantID,
			blockID:  blockID,
			path:     "chunks/111111",
			body:     chunkBodyContent,
			setUpBucketMock: func(bkt *bucket.ClientMock) {
				bkt.MockExists(metaPath, false, nil)

				b, err := json.Marshal(validMeta)
				setUpGet(bkt, path.Join(tenantID, blockID, uploadingMetaFilename), b, err)
				setUpGet(bkt, path.Join(tenantID, blockID, validationFilename), nil, bucket.ErrObjectDoesNotExist)
			},
			expBadRequest: "unexpected file",
		},
		{
			name:            "valid request",
			tenantID:        tenantID,
			blockID:         blockID,
			path:            "chunks/000001",
			body:            chunkBodyContent,
			setUpBucketMock: setupFnForValidRequest,
			verifyUpload:    verifyFuncForValidRequest,
		},
		{
			name:                 "valid request, with unknown content-length",
			tenantID:             tenantID,
			blockID:              blockID,
			path:                 "chunks/000001",
			body:                 chunkBodyContent,
			unknownContentLength: true,
			setUpBucketMock:      setupFnForValidRequest,
			verifyUpload:         verifyFuncForValidRequest,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var bkt bucket.ClientMock
			if tc.setUpBucketMock != nil {
				tc.setUpBucketMock(&bkt)
			}

			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadEnabled[tc.tenantID] = !tc.disableBlockUpload
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: &bkt,
				cfgProvider:  cfgProvider,
			}
			var rdr io.Reader
			if tc.body != "" {
				rdr = strings.NewReader(tc.body)
			}
			r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/upload/block/%s/files?path=%s", blockID, url.QueryEscape(tc.path)), rdr)
			if tc.tenantID != "" {
				r = r.WithContext(user.InjectOrgID(r.Context(), tenantID))
			}
			if tc.blockID != "" {
				r = mux.SetURLVars(r, map[string]string{"block": tc.blockID})
			}
			if tc.body != "" {
				r.ContentLength = int64(len(tc.body))
				if tc.unknownContentLength {
					r.ContentLength = -1
				}
			}
			w := httptest.NewRecorder()
			c.UploadBlockFile(w, r)

			resp := w.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			switch {
			case tc.expBadRequest != "":
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expBadRequest), string(body))
			case tc.expConflict != "":
				assert.Equal(t, http.StatusConflict, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expConflict), string(body))
			case tc.expNotFound != "":
				assert.Equal(t, http.StatusNotFound, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expNotFound), string(body))
			case tc.expInternalServerError:
				assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
				assert.Regexp(t, "internal server error \\(id [0-9a-f]{16}\\)\n", string(body))
			default:
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Empty(t, string(body))
			}

			bkt.AssertExpectations(t)

			if tc.verifyUpload != nil {
				tc.verifyUpload(t, &bkt, tc.body)
			}
		})
	}

	type file struct {
		path    string
		content string
	}

	// Additional test cases using an in-memory bucket for state testing
	extraCases := []struct {
		name         string
		files        []file
		setUpBucket  func(*testing.T, *objstore.InMemBucket)
		verifyBucket func(*testing.T, *objstore.InMemBucket, []file)
	}{
		{
			name: "multiple sequential uploads of same file",
			files: []file{
				{
					path:    "chunks/000001",
					content: strings.Repeat("a", len(chunkBodyContent)),
				},
				{
					path:    "chunks/000001",
					content: strings.Repeat("b", len(chunkBodyContent)),
				},
			},
			setUpBucket: func(t *testing.T, bkt *objstore.InMemBucket) {
				marshalAndUploadJSON(t, bkt, uploadingMetaPath, validMeta)
			},
			verifyBucket: func(t *testing.T, bkt *objstore.InMemBucket, files []file) {
				t.Helper()

				ctx := context.Background()
				rdr, err := bkt.Get(ctx, path.Join(tenantID, blockID, files[1].path))
				require.NoError(t, err)
				t.Cleanup(func() {
					_ = rdr.Close()
				})

				content, err := io.ReadAll(rdr)
				require.NoError(t, err)
				assert.Equal(t, files[1].content, string(content))
			},
		},
	}
	for _, tc := range extraCases {
		t.Run(tc.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			tc.setUpBucket(t, bkt)
			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadEnabled[tenantID] = true
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: bkt,
				cfgProvider:  cfgProvider,
			}

			for _, f := range tc.files {
				rdr := strings.NewReader(f.content)
				r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/upload/block/%s/files?path=%s", blockID, url.QueryEscape(f.path)), rdr)
				urlVars := map[string]string{
					"block": blockID,
				}
				r = mux.SetURLVars(r, urlVars)
				r = r.WithContext(user.InjectOrgID(r.Context(), tenantID))
				w := httptest.NewRecorder()
				c.UploadBlockFile(w, r)

				resp := w.Result()
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Empty(t, body)
			}

			tc.verifyBucket(t, bkt, tc.files)
		})
	}
}

func setUpGet(bkt *bucket.ClientMock, pth string, content []byte, err error) {
	bkt.On("Get", mock.Anything, pth).Return(func(_ context.Context, _ string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(content)), err
	})
}

// Test MultitenantCompactor.FinishBlockUpload
func TestMultitenantCompactor_FinishBlockUpload(t *testing.T) {
	const tenantID = "test"
	const blockID = "01G3FZ0JWJYJC0ZM6Y9778P6KD"
	uploadingMetaPath := path.Join(tenantID, blockID, uploadingMetaFilename)
	metaPath := path.Join(tenantID, blockID, block.MetaFilename)
	injectedError := fmt.Errorf("injected error")
	validMeta := block.Meta{
		BlockMeta: tsdb.BlockMeta{
			Version: block.TSDBVersion1,
			ULID:    ulid.MustParse(blockID),
		},
		Thanos: block.ThanosMeta{
			Labels: map[string]string{
				mimir_tsdb.CompactorShardIDExternalLabel: "1_of_3",
			},
			Files: []block.File{
				{
					RelPath:   "index",
					SizeBytes: 1,
				},
				{
					RelPath:   "chunks/000001",
					SizeBytes: 2,
				},
			},
		},
	}

	validSetup := func(t *testing.T, bkt objstore.Bucket) {
		err := marshalAndUploadToBucket(context.Background(), bkt, uploadingMetaPath, validMeta)
		require.NoError(t, err)
		for _, file := range validMeta.Thanos.Files {
			content := bytes.NewReader(make([]byte, file.SizeBytes))
			err = bkt.Upload(context.Background(), path.Join(tenantID, blockID, file.RelPath), content)
			require.NoError(t, err)
		}
	}

	testCases := []struct {
		name                   string
		tenantID               string
		blockID                string
		setUpBucket            func(*testing.T, objstore.Bucket)
		errorInjector          func(op bucket.Operation, name string) error
		disableBlockUpload     bool
		enableValidation       bool // should only be set to true for tests that fail before validation is started
		maxConcurrency         int
		setConcurrency         int64
		expBadRequest          string
		expConflict            string
		expNotFound            string
		expTooManyRequests     bool
		expInternalServerError bool
	}{
		{
			name:          "without tenant ID",
			blockID:       blockID,
			expBadRequest: "invalid tenant ID",
		},
		{
			name:          "without block ID",
			tenantID:      tenantID,
			expBadRequest: "invalid block ID",
		},
		{
			name:          "invalid block ID",
			tenantID:      tenantID,
			blockID:       "1234",
			expBadRequest: "invalid block ID",
		},
		{
			name:               "block upload disabled",
			tenantID:           tenantID,
			blockID:            blockID,
			disableBlockUpload: true,
			expBadRequest:      "block upload is disabled",
		},
		{
			name:     "complete block already exists",
			tenantID: tenantID,
			blockID:  blockID,
			setUpBucket: func(t *testing.T, bkt objstore.Bucket) {
				err := marshalAndUploadToBucket(context.Background(), bkt, metaPath, validMeta)
				require.NoError(t, err)
			},
			expConflict: "block already exists",
		},
		{
			name:                   "checking for complete block fails",
			tenantID:               tenantID,
			blockID:                blockID,
			errorInjector:          bucket.InjectErrorOn(bucket.OpExists, metaPath, injectedError),
			expInternalServerError: true,
		},
		{
			name:        "missing in-flight meta file",
			tenantID:    tenantID,
			blockID:     blockID,
			expNotFound: "block upload not started",
		},
		{
			name:     "downloading in-flight meta file fails",
			tenantID: tenantID,
			blockID:  blockID,
			setUpBucket: func(t *testing.T, bkt objstore.Bucket) {
				err := marshalAndUploadToBucket(context.Background(), bkt, uploadingMetaPath, validMeta)
				require.NoError(t, err)
			},
			errorInjector:          bucket.InjectErrorOn(bucket.OpGet, uploadingMetaPath, injectedError),
			expInternalServerError: true,
		},
		{
			name:     "corrupt in-flight meta file",
			tenantID: tenantID,
			blockID:  blockID,
			setUpBucket: func(t *testing.T, bkt objstore.Bucket) {
				err := bkt.Upload(context.Background(), uploadingMetaPath, bytes.NewReader([]byte("{")))
				require.NoError(t, err)
			},
			expInternalServerError: true,
		},
		{
			name:                   "uploading meta file fails",
			tenantID:               tenantID,
			blockID:                blockID,
			setUpBucket:            validSetup,
			errorInjector:          bucket.InjectErrorOn(bucket.OpUpload, metaPath, injectedError),
			expInternalServerError: true,
		},
		{
			name:               "too many concurrent validations",
			tenantID:           tenantID,
			blockID:            blockID,
			setUpBucket:        validSetup,
			enableValidation:   true,
			maxConcurrency:     2,
			setConcurrency:     2,
			expTooManyRequests: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			injectedBkt := bucket.ErrorInjectedBucketClient{
				Bucket:   bkt,
				Injector: tc.errorInjector,
			}
			if tc.setUpBucket != nil {
				tc.setUpBucket(t, bkt)
			}

			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadEnabled[tc.tenantID] = !tc.disableBlockUpload
			cfgProvider.blockUploadValidationEnabled[tc.tenantID] = tc.enableValidation
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: &injectedBkt,
				cfgProvider:  cfgProvider,
			}
			c.compactorCfg.MaxBlockUploadValidationConcurrency = tc.maxConcurrency
			if tc.setConcurrency > 0 {
				c.blockUploadValidations.Add(tc.setConcurrency)
			}

			c.compactorCfg.DataDir = t.TempDir()

			r := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/upload/block/%s/finish", tc.blockID), nil)
			if tc.tenantID != "" {
				r = r.WithContext(user.InjectOrgID(r.Context(), tenantID))
			}
			if tc.blockID != "" {
				r = mux.SetURLVars(r, map[string]string{"block": tc.blockID})
			}
			w := httptest.NewRecorder()
			c.FinishBlockUpload(w, r)

			resp := w.Result()
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			switch {
			case tc.expBadRequest != "":
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expBadRequest), string(body))
			case tc.expConflict != "":
				assert.Equal(t, http.StatusConflict, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expConflict), string(body))
			case tc.expNotFound != "":
				assert.Equal(t, http.StatusNotFound, resp.StatusCode)
				assert.Equal(t, fmt.Sprintf("%s\n", tc.expNotFound), string(body))
			case tc.expInternalServerError:
				assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
				assert.Regexp(t, "internal server error \\(id [0-9a-f]{16}\\)\n", string(body))
			case tc.expTooManyRequests:
				assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
				assert.Equal(t, "too many block upload validations in progress, limit is 2\n", string(body))
			default:
				assert.Equal(t, http.StatusOK, resp.StatusCode)
				assert.Empty(t, string(body))
				exists, err := bkt.Exists(context.Background(), path.Join(tc.blockID, block.MetaFilename))
				require.NoError(t, err)
				require.True(t, exists)
			}
		})
	}
}

func TestMultitenantCompactor_ValidateAndComplete(t *testing.T) {
	const tenantID = "test"
	const blockID = "01G3FZ0JWJYJC0ZM6Y9778P6KD"
	injectedError := fmt.Errorf("injected error")

	uploadingMetaPath := path.Join(tenantID, blockID, uploadingMetaFilename)
	validationPath := path.Join(tenantID, blockID, validationFilename)
	metaPath := path.Join(tenantID, blockID, block.MetaFilename)

	validationSucceeds := func(_ context.Context) error { return nil }

	testCases := []struct {
		name                        string
		errorInjector               func(op bucket.Operation, name string) error
		validation                  func(context.Context) error
		expectValidationFile        bool
		expectErrorInValidationFile bool
		expectTempUploadingMeta     bool
		expectMeta                  bool
	}{
		{
			name:                        "validation fails",
			validation:                  func(_ context.Context) error { return injectedError },
			expectValidationFile:        true,
			expectErrorInValidationFile: true,
			expectTempUploadingMeta:     true,
			expectMeta:                  false,
		},
		{
			name:                        "validation fails, uploading error fails",
			errorInjector:               bucket.InjectErrorOn(bucket.OpUpload, validationPath, injectedError),
			validation:                  func(_ context.Context) error { return injectedError },
			expectValidationFile:        true,
			expectErrorInValidationFile: false,
			expectTempUploadingMeta:     true,
			expectMeta:                  false,
		},
		{
			name:                        "uploading meta file fails",
			errorInjector:               bucket.InjectErrorOn(bucket.OpUpload, metaPath, injectedError),
			validation:                  validationSucceeds,
			expectValidationFile:        true,
			expectErrorInValidationFile: true,
			expectTempUploadingMeta:     true,
			expectMeta:                  false,
		},
		{
			name: "uploading meta file fails, uploading error fails",
			errorInjector: func(op bucket.Operation, target string) error {
				if op == bucket.OpUpload && (target == metaPath || target == validationPath) {
					return injectedError
				}
				return nil
			},
			validation:                  validationSucceeds,
			expectValidationFile:        true,
			expectErrorInValidationFile: false,
			expectTempUploadingMeta:     true,
			expectMeta:                  false,
		},
		{
			name:                    "removing in-flight meta file fails",
			errorInjector:           bucket.InjectErrorOn(bucket.OpDelete, uploadingMetaPath, injectedError),
			validation:              validationSucceeds,
			expectValidationFile:    false,
			expectTempUploadingMeta: true,
			expectMeta:              true,
		},
		{
			name:                        "removing validation file fails",
			errorInjector:               bucket.InjectErrorOn(bucket.OpDelete, validationPath, injectedError),
			validation:                  validationSucceeds,
			expectValidationFile:        true,
			expectErrorInValidationFile: false,
			expectTempUploadingMeta:     false,
			expectMeta:                  true,
		},
		{
			name:                    "valid request",
			validation:              validationSucceeds,
			expectValidationFile:    false,
			expectTempUploadingMeta: false,
			expectMeta:              true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			var injectedBkt objstore.Bucket = bkt
			if tc.errorInjector != nil {
				injectedBkt = &bucket.ErrorInjectedBucketClient{
					Bucket:   bkt,
					Injector: tc.errorInjector,
				}
			}
			cfgProvider := newMockConfigProvider()
			c := &MultitenantCompactor{
				logger:            log.NewNopLogger(),
				bucketClient:      injectedBkt,
				cfgProvider:       cfgProvider,
				blockUploadBlocks: promauto.With(nil).NewGaugeVec(prometheus.GaugeOpts{}, []string{tenantID}),
				blockUploadBytes:  promauto.With(nil).NewGaugeVec(prometheus.GaugeOpts{}, []string{tenantID}),
				blockUploadFiles:  promauto.With(nil).NewGaugeVec(prometheus.GaugeOpts{}, []string{tenantID}),
			}
			userBkt := bucket.NewUserBucketClient(tenantID, injectedBkt, cfgProvider)

			meta := block.Meta{}
			marshalAndUploadJSON(t, bkt, uploadingMetaPath, meta)
			v := validationFile{}
			marshalAndUploadJSON(t, bkt, validationPath, v)

			c.validateAndCompleteBlockUpload(log.NewNopLogger(), tenantID, userBkt, ulid.MustParse(blockID), &meta, tc.validation)

			tempUploadingMetaExists, err := bkt.Exists(context.Background(), uploadingMetaPath)
			require.NoError(t, err)
			require.Equal(t, tempUploadingMetaExists, tc.expectTempUploadingMeta)

			metaExists, err := bkt.Exists(context.Background(), metaPath)
			require.NoError(t, err)
			require.Equal(t, metaExists, tc.expectMeta)

			if !tc.expectValidationFile {
				exists, err := bkt.Exists(context.Background(), validationPath)
				require.NoError(t, err)
				require.False(t, exists)
				return
			}

			r, err := bkt.Get(context.Background(), validationPath)
			require.NoError(t, err)
			decoder := json.NewDecoder(r)
			err = decoder.Decode(&v)
			require.NoError(t, err)

			if tc.expectErrorInValidationFile {
				require.NotEmpty(t, v.Error)
			} else {
				require.Empty(t, v.Error)
			}
		})
	}
}

func TestMultitenantCompactor_ValidateBlock(t *testing.T) {
	const tenantID = "test"
	ctx := context.Background()
	tmpDir := t.TempDir()
	bkt := objstore.NewInMemBucket()

	type Missing uint8
	const (
		MissingMeta Missing = 1 << iota
		MissingIndex
		MissingChunks
	)

	validLabels := func() []labels.Labels {
		return []labels.Labels{
			labels.FromStrings("a", "1"),
			labels.FromStrings("b", "2"),
			labels.FromStrings("c", "3"),
		}
	}

	testCases := []struct {
		name             string
		lbls             func() []labels.Labels
		metaInject       func(meta *block.Meta)
		indexInject      func(fname string)
		chunkInject      func(fname string)
		populateFileList bool
		maximumBlockSize int64
		verifyChunks     bool
		missing          Missing
		expectError      bool
		expectedMsg      string
	}{
		{
			name:             "valid block",
			lbls:             validLabels,
			verifyChunks:     true,
			expectError:      false,
			populateFileList: true,
		},
		{
			name:             "maximum block size exceeded",
			lbls:             validLabels,
			populateFileList: true,
			maximumBlockSize: 1,
			expectError:      true,
			expectedMsg:      fmt.Sprintf(maxBlockUploadSizeBytesFormat, 1),
		},
		{
			name:        "missing meta file",
			lbls:        validLabels,
			missing:     MissingMeta,
			expectError: true,
			expectedMsg: "failed renaming while preparing block for validation",
		},
		{
			name:        "missing index file",
			lbls:        validLabels,
			missing:     MissingIndex,
			expectError: true,
			expectedMsg: "error validating block: open index file:",
		},
		{
			name:             "missing chunks file",
			lbls:             validLabels,
			populateFileList: true,
			missing:          MissingChunks,
			expectError:      true,
			expectedMsg:      "failed to stat chunks/",
		},
		{
			name: "file size mismatch",
			lbls: validLabels,
			metaInject: func(meta *block.Meta) {
				require.Greater(t, len(meta.Thanos.Files), 0)
				meta.Thanos.Files[0].SizeBytes += 10
			},
			populateFileList: true,
			expectError:      true,
			expectedMsg:      "file size mismatch",
		},
		{
			name: "empty index file",
			lbls: validLabels,
			indexInject: func(fname string) {
				require.NoError(t, os.Truncate(fname, 0))
			},
			expectError: true,
			expectedMsg: "error validating block: open index file: mmap, size 0: invalid argument",
		},
		{
			name: "index file invalid magic number",
			lbls: validLabels,
			indexInject: func(fname string) {
				flipByteAt(t, fname, 0) // guaranteed to be a magic number byte
			},
			expectError: true,
			expectedMsg: "error validating block: open index file: invalid magic number",
		},
		{
			name: "out of order labels",
			lbls: func() []labels.Labels {
				b := labels.NewScratchBuilder(2)
				b.Add("d", "4")
				b.Add("a", "1")
				oooLabels := []labels.Labels{
					b.Labels(), // Haven't called Sort(), so they will be out of order.
					labels.FromStrings("b", "2"),
					labels.FromStrings("c", "3"),
				}
				return oooLabels
			},
			expectError: true,
			expectedMsg: "error validating block: index contains 1 postings with out of order labels",
		},
		{
			name: "segment file invalid magic number",
			lbls: validLabels,
			chunkInject: func(fname string) {
				flipByteAt(t, fname, 0) // guaranteed to be a magic number byte
			},
			verifyChunks: true,
			expectError:  true,
			expectedMsg:  "invalid magic number",
		},
		{
			name: "segment file invalid checksum",
			lbls: validLabels,
			chunkInject: func(fname string) {
				flipByteAt(t, fname, 12) // guaranteed to be a data byte
			},
			populateFileList: true,
			verifyChunks:     true,
			expectError:      true,
			expectedMsg:      "checksum mismatch",
		},
		{
			name: "empty segment file",
			lbls: validLabels,
			chunkInject: func(fname string) {
				require.NoError(t, os.Truncate(fname, 0))
			},
			verifyChunks: true,
			expectError:  true,
			expectedMsg:  "size 0: invalid argument",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// create a test block
			now := time.Now()
			blockID, err := block.CreateBlock(ctx, tmpDir, tc.lbls(), 300, now.Add(-2*time.Hour).UnixMilli(), now.UnixMilli(), labels.EmptyLabels())
			require.NoError(t, err)
			testDir := filepath.Join(tmpDir, blockID.String())
			meta, err := block.ReadMetaFromDir(testDir)
			require.NoError(t, err)
			if tc.populateFileList {
				stats, err := block.GatherFileStats(testDir)
				require.NoError(t, err)
				meta.Thanos.Files = stats
			}

			// create a compactor
			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadValidationEnabled[tenantID] = true
			cfgProvider.verifyChunks[tenantID] = tc.verifyChunks
			cfgProvider.blockUploadMaxBlockSizeBytes[tenantID] = tc.maximumBlockSize
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: bkt,
				cfgProvider:  cfgProvider,
			}

			// upload the block
			require.NoError(t, block.Upload(ctx, log.NewNopLogger(), bkt, testDir, nil))
			// remove meta.json as we will be uploading a new one with the uploading meta name
			require.NoError(t, bkt.Delete(ctx, path.Join(blockID.String(), block.MetaFilename)))

			// handle meta file
			if tc.metaInject != nil {
				tc.metaInject(meta)
			}
			var metaBody bytes.Buffer
			require.NoError(t, meta.Write(&metaBody))

			// replace index file
			if tc.indexInject != nil {
				indexFile := filepath.Join(testDir, block.IndexFilename)
				indexObject := path.Join(blockID.String(), block.IndexFilename)
				require.NoError(t, bkt.Delete(ctx, indexObject))
				tc.indexInject(indexFile)
				uploadLocalFileToBucket(ctx, t, bkt, indexFile, indexObject)
			}

			// replace segment file
			if tc.chunkInject != nil {
				segmentFile := filepath.Join(testDir, block.ChunksDirname, "000001")
				segmentObject := path.Join(blockID.String(), block.ChunksDirname, "000001")
				require.NoError(t, bkt.Delete(ctx, segmentObject))
				tc.chunkInject(segmentFile)
				uploadLocalFileToBucket(ctx, t, bkt, segmentFile, segmentObject)
			}

			// delete any files that should be missing
			if tc.missing&MissingIndex != 0 {
				require.NoError(t, bkt.Delete(ctx, path.Join(blockID.String(), block.IndexFilename)))
			}

			if tc.missing&MissingChunks != 0 {
				chunkDir := path.Join(blockID.String(), block.ChunksDirname)
				err := bkt.Iter(ctx, chunkDir, func(name string) error {
					require.NoError(t, bkt.Delete(ctx, name))
					return nil
				})
				require.NoError(t, err)
			}

			// only upload renamed meta file if it is not meant to be missing
			if tc.missing&MissingMeta == 0 {
				// rename to uploading meta file as that is what validateBlock expects
				require.NoError(t, bkt.Upload(ctx, path.Join(blockID.String(), uploadingMetaFilename), &metaBody))
			}

			// validate the block
			err = c.validateBlock(ctx, c.logger, blockID, meta, bkt, tenantID)
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMultitenantCompactor_PeriodicValidationUpdater(t *testing.T) {
	const tenantID = "test"
	const blockID = "01G3FZ0JWJYJC0ZM6Y9778P6KD"
	injectedError := fmt.Errorf("injected error")
	validationPath := path.Join(tenantID, blockID, validationFilename)

	heartbeatInterval := 50 * time.Millisecond

	validationExists := func(t *testing.T, bkt objstore.Bucket) bool {
		exists, err := bkt.Exists(context.Background(), validationPath)
		require.NoError(t, err)
		return exists
	}

	testCases := []struct {
		name          string
		errorInjector func(op bucket.Operation, name string) error
		cancelContext bool
		assertions    func(t *testing.T, ctx context.Context, bkt objstore.Bucket)
	}{
		{
			name:          "updating validation file fails",
			errorInjector: bucket.InjectErrorOn(bucket.OpUpload, validationPath, injectedError),
			assertions: func(t *testing.T, ctx context.Context, bkt objstore.Bucket) {
				<-ctx.Done()
				require.True(t, errors.Is(context.Canceled, ctx.Err()))
				require.False(t, validationExists(t, bkt))
			},
		},
		{
			name: "updating validation file succeeds",
			assertions: func(t *testing.T, ctx context.Context, bkt objstore.Bucket) {
				test.Poll(t, heartbeatInterval*2, true, func() interface{} {
					return validationExists(t, bkt)
				})

				v := validationFile{}
				r, err := bkt.Get(context.Background(), validationPath)
				require.NoError(t, err)
				decoder := json.NewDecoder(r)
				err = decoder.Decode(&v)
				require.NoError(t, err)
				require.NotEqual(t, 0, v.LastUpdate)
				require.Empty(t, v.Error)
			},
		},
		{
			name:          "context cancelled before update",
			cancelContext: true,
			assertions: func(t *testing.T, ctx context.Context, bkt objstore.Bucket) {
				require.False(t, validationExists(t, bkt))
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			var injectedBkt objstore.Bucket = bkt
			if tc.errorInjector != nil {
				injectedBkt = &bucket.ErrorInjectedBucketClient{
					Bucket:   bkt,
					Injector: tc.errorInjector,
				}
			}

			cfgProvider := newMockConfigProvider()
			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: injectedBkt,
				cfgProvider:  cfgProvider,
			}
			userBkt := bucket.NewUserBucketClient(tenantID, injectedBkt, cfgProvider)
			ctx, cancel := context.WithCancel(context.Background())

			heartbeatInterval := heartbeatInterval
			if tc.cancelContext {
				cancel()
				heartbeatInterval = 1 * time.Hour // to avoid racing a heartbeat
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				c.periodicValidationUpdater(ctx, log.NewNopLogger(), ulid.MustParse(blockID), userBkt, cancel, heartbeatInterval)
			}()

			if !tc.cancelContext {
				time.Sleep(heartbeatInterval)
			}

			tc.assertions(t, ctx, bkt)

			cancel()
			wg.Wait()
		})
	}
}

func TestMultitenantCompactor_GetBlockUploadStateHandler(t *testing.T) {
	const (
		tenantID = "tenant"
		blockID  = "01G8X9GA8R6N8F75FW1J18G83N"
	)

	type testcase struct {
		setupBucket        func(t *testing.T, bkt objstore.Bucket)
		disableBlockUpload bool
		expectedStatusCode int
		expectedBody       string
	}

	for name, tc := range map[string]testcase{
		"block doesn't exist": {
			expectedStatusCode: http.StatusNotFound,
			expectedBody:       "block doesn't exist",
		},

		"complete block": {
			setupBucket: func(t *testing.T, bkt objstore.Bucket) {
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, block.MetaFilename), block.Meta{})
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"result":"complete"}`,
		},

		"upload in progress": {
			setupBucket: func(t *testing.T, bkt objstore.Bucket) {
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, uploadingMetaFilename), block.Meta{})
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"result":"uploading"}`,
		},

		"validating": {
			setupBucket: func(t *testing.T, bkt objstore.Bucket) {
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, uploadingMetaFilename), block.Meta{})
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, validationFilename), validationFile{LastUpdate: time.Now().UnixMilli()})
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"result":"validating"}`,
		},

		"validation failed": {
			setupBucket: func(t *testing.T, bkt objstore.Bucket) {
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, uploadingMetaFilename), block.Meta{})
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, validationFilename), validationFile{LastUpdate: time.Now().UnixMilli(), Error: "error during validation"})
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"result":"failed","error":"error during validation"}`,
		},

		"stale validation file": {
			setupBucket: func(t *testing.T, bkt objstore.Bucket) {
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, uploadingMetaFilename), block.Meta{})
				marshalAndUploadJSON(t, bkt, path.Join(tenantID, blockID, validationFilename), validationFile{LastUpdate: time.Now().Add(-10 * time.Minute).UnixMilli()})
			},
			expectedStatusCode: http.StatusOK,
			expectedBody:       `{"result":"uploading"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			if tc.setupBucket != nil {
				tc.setupBucket(t, bkt)
			}

			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadEnabled[tenantID] = !tc.disableBlockUpload

			c := &MultitenantCompactor{
				logger:       log.NewNopLogger(),
				bucketClient: bkt,
				cfgProvider:  cfgProvider,
			}

			r := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/upload/block/%s/check", blockID), nil)
			urlVars := map[string]string{"block": blockID}
			r = mux.SetURLVars(r, urlVars)
			r = r.WithContext(user.InjectOrgID(r.Context(), tenantID))

			w := httptest.NewRecorder()
			c.GetBlockUploadStateHandler(w, r)
			resp := w.Result()

			body, err := io.ReadAll(resp.Body)

			require.NoError(t, err)
			require.Equal(t, tc.expectedStatusCode, resp.StatusCode)
			require.Equal(t, tc.expectedBody, strings.TrimSpace(string(body)))
		})
	}
}

func TestMultitenantCompactor_ValidateMaximumBlockSize(t *testing.T) {
	const userID = "user"

	type testCase struct {
		maximumBlockSize int64
		fileSizes        []int64
		expectErr        bool
	}

	for name, tc := range map[string]testCase{
		"no limit": {
			maximumBlockSize: 0,
			fileSizes:        []int64{math.MaxInt64},
			expectErr:        false,
		},
		"under limit": {
			maximumBlockSize: 4,
			fileSizes:        []int64{1, 2},
			expectErr:        false,
		},
		"under limit - zero size file included": {
			maximumBlockSize: 2,
			fileSizes:        []int64{1, 0},
			expectErr:        false,
		},
		"under limit - negative size file included": {
			maximumBlockSize: 2,
			fileSizes:        []int64{2, -1},
			expectErr:        true,
		},
		"exact limit": {
			maximumBlockSize: 3,
			fileSizes:        []int64{1, 2},
			expectErr:        false,
		},
		"over limit": {
			maximumBlockSize: 1,
			fileSizes:        []int64{1, 1},
			expectErr:        true,
		},
		"overflow": {
			maximumBlockSize: math.MaxInt64,
			fileSizes:        []int64{math.MaxInt64, math.MaxInt64, math.MaxInt64},
			expectErr:        true,
		},
	} {
		t.Run(name, func(t *testing.T) {
			files := make([]block.File, len(tc.fileSizes))
			for i, size := range tc.fileSizes {
				files[i] = block.File{SizeBytes: size}
			}

			cfgProvider := newMockConfigProvider()
			cfgProvider.blockUploadMaxBlockSizeBytes[userID] = tc.maximumBlockSize
			c := &MultitenantCompactor{
				logger:      log.NewNopLogger(),
				cfgProvider: cfgProvider,
			}

			err := c.validateMaximumBlockSize(c.logger, files, userID)
			if tc.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMultitenantCompactor_MarkBlockComplete(t *testing.T) {
	const tenantID = "test"
	const blockID = "01G3FZ0JWJYJC0ZM6Y9778P6KD"
	injectedError := fmt.Errorf("injected error")

	uploadingMetaPath := path.Join(tenantID, blockID, uploadingMetaFilename)
	metaPath := path.Join(tenantID, blockID, block.MetaFilename)
	testCases := []struct {
		name          string
		errorInjector func(op bucket.Operation, name string) error
		expectSuccess bool
	}{
		{
			name:          "marking block complete succeeds",
			expectSuccess: true,
		},
		{
			name:          "uploading meta file fails",
			errorInjector: bucket.InjectErrorOn(bucket.OpUpload, metaPath, injectedError),
		},
		{
			name:          "deleting uploading meta file fails",
			errorInjector: bucket.InjectErrorOn(bucket.OpDelete, uploadingMetaPath, injectedError),
			expectSuccess: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bkt := objstore.NewInMemBucket()
			var injectedBkt objstore.Bucket = bkt
			if tc.errorInjector != nil {
				injectedBkt = &bucket.ErrorInjectedBucketClient{
					Bucket:   bkt,
					Injector: tc.errorInjector,
				}
			}
			cfgProvider := newMockConfigProvider()
			c := &MultitenantCompactor{
				logger:            log.NewNopLogger(),
				bucketClient:      injectedBkt,
				cfgProvider:       cfgProvider,
				blockUploadBlocks: promauto.With(nil).NewGaugeVec(prometheus.GaugeOpts{}, []string{tenantID}),
				blockUploadBytes:  promauto.With(nil).NewGaugeVec(prometheus.GaugeOpts{}, []string{tenantID}),
				blockUploadFiles:  promauto.With(nil).NewGaugeVec(prometheus.GaugeOpts{}, []string{tenantID}),
			}
			userBkt := bucket.NewUserBucketClient(tenantID, injectedBkt, cfgProvider)

			meta := block.Meta{
				Thanos: block.ThanosMeta{
					Files: []block.File{
						{
							RelPath:   "chunks/000001",
							SizeBytes: 42,
						},
						{
							RelPath:   "index",
							SizeBytes: 17,
						},
						{
							RelPath: "meta.json",
						},
					},
				},
			}
			marshalAndUploadJSON(t, bkt, uploadingMetaPath, meta)

			ctx := context.Background()
			err := c.markBlockComplete(ctx, log.NewNopLogger(), tenantID, userBkt, ulid.MustParse(blockID), &meta)
			if tc.expectSuccess {
				require.NoError(t, err)
				assert.Equal(t, 1.0, promtest.ToFloat64(c.blockUploadBlocks.WithLabelValues(tenantID)))
				assert.Equal(t, 59.0, promtest.ToFloat64(c.blockUploadBytes.WithLabelValues(tenantID)))
				assert.Equal(t, 3.0, promtest.ToFloat64(c.blockUploadFiles.WithLabelValues(tenantID)))
			} else {
				require.Error(t, err)
				assert.Equal(t, 0.0, promtest.ToFloat64(c.blockUploadBlocks.WithLabelValues(tenantID)))
				assert.Equal(t, 0.0, promtest.ToFloat64(c.blockUploadBytes.WithLabelValues(tenantID)))
				assert.Equal(t, 0.0, promtest.ToFloat64(c.blockUploadFiles.WithLabelValues(tenantID)))
			}
		})
	}
}

// marshalAndUploadJSON is a test helper for uploading a meta file to a certain path in a bucket.
func marshalAndUploadJSON(t *testing.T, bkt objstore.Bucket, pth string, val interface{}) {
	t.Helper()
	err := marshalAndUploadToBucket(context.Background(), bkt, pth, val)
	require.NoError(t, err)
}

func uploadLocalFileToBucket(ctx context.Context, t *testing.T, bkt objstore.Bucket, src, dst string) {
	t.Helper()
	fd, err := os.Open(src)
	require.NoError(t, err)
	defer func(fd *os.File) {
		err := fd.Close()
		require.NoError(t, err)
	}(fd)
	require.NoError(t, bkt.Upload(ctx, dst, fd))
}

// flipByteAt flips a byte at a given offset in a file.
func flipByteAt(t *testing.T, fname string, offset int64) {
	fd, err := os.OpenFile(fname, os.O_RDWR, 0o644)
	require.NoError(t, err)
	defer func(fd *os.File) {
		err := fd.Close()
		require.NoError(t, err)
	}(fd)
	var b [1]byte
	_, err = fd.ReadAt(b[:], offset)
	require.NoError(t, err)
	// alter the byte
	b[0] = 0xff - b[0]
	_, err = fd.WriteAt(b[:], offset)
	require.NoError(t, err)
}

func TestHexTimeNowNano(t *testing.T) {
	v := hexTimeNowNano()
	require.Len(t, v, 16, "Should have exactly 16 characters")

	require.NotEqual(t, strings.Repeat("0", 16), v, "Should not be all zeros")
	time.Sleep(time.Nanosecond)
	require.NotEqual(t, v, hexTimeNowNano(), "Should generate a different one.")
}
