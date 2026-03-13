package debuginfo

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	debuginfov1alpha1 "github.com/grafana/pyroscope/api/gen/proto/go/debuginfo/v1alpha1"
	"github.com/grafana/pyroscope/pkg/objstore/providers/memory"
)

func newTestStore(t *testing.T, cfg Config) (*Store, *memory.InMemBucket) {
	t.Helper()
	bucket := memory.NewInMemBucket()
	s, err := NewStore(log.NewNopLogger(), bucket, cfg)
	require.NoError(t, err)
	return s, bucket
}

func mustValidateGnuBuildID(t *testing.T, id string) *ValidGnuBuildID {
	t.Helper()
	v, err := ValidateGnuBuildID(id)
	require.NoError(t, err)
	return v
}

func TestValidateGnuBuildID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid 2 chars (min boundary)", input: "ab", wantErr: false},
		{name: "valid 40 chars (max boundary)", input: strings.Repeat("ab", 20), wantErr: false},
		{name: "valid mixed case hex", input: "aAbBcCdDeEfF00112233", wantErr: false},
		{name: "valid lowercase hex", input: "deadbeef", wantErr: false},
		{name: "valid uppercase hex", input: "DEADBEEF", wantErr: false},
		{name: "empty string", input: "", wantErr: true},
		{name: "single char below min", input: "a", wantErr: true},
		{name: "41 chars above max", input: strings.Repeat("a", 41), wantErr: true},
		{name: "non-hex letter g", input: "abcg", wantErr: true},
		{name: "contains space", input: "ab cd", wantErr: true},
		{name: "contains dash", input: "ab-cd", wantErr: true},
		{name: "special characters", input: "ab!@#$", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := ValidateGnuBuildID(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, id)
			} else {
				require.NoError(t, err)
				require.NotNil(t, id)
				assert.Equal(t, tt.input, id.gnuBuildID)
			}
		})
	}
}

func TestValidateInit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		init       *debuginfov1alpha1.ShouldInitiateUploadRequest
		wantErr    bool
		errContain string
	}{
		{
			name:       "nil init",
			init:       nil,
			wantErr:    true,
			errContain: "first message expected to be init",
		},
		{
			name:       "nil file",
			init:       &debuginfov1alpha1.ShouldInitiateUploadRequest{File: nil},
			wantErr:    true,
			errContain: "init.FileData == nil",
		},
		{
			name: "valid executable full",
			init: &debuginfov1alpha1.ShouldInitiateUploadRequest{
				File: &debuginfov1alpha1.FileMetadata{
					GNU:  "aabbccdd",
					Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_FULL,
				},
			},
			wantErr: false,
		},
		{
			name: "valid executable no text",
			init: &debuginfov1alpha1.ShouldInitiateUploadRequest{
				File: &debuginfov1alpha1.FileMetadata{
					GNU:  "aabbccdd",
					Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_NO_TEXT,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid type",
			init: &debuginfov1alpha1.ShouldInitiateUploadRequest{
				File: &debuginfov1alpha1.FileMetadata{
					GNU:  "aabbccdd",
					Type: debuginfov1alpha1.FileMetadata_DebuginfoType(99),
				},
			},
			wantErr:    true,
			errContain: "is not valid",
		},
		{
			name: "valid type invalid build id",
			init: &debuginfov1alpha1.ShouldInitiateUploadRequest{
				File: &debuginfov1alpha1.FileMetadata{
					GNU:  "xyz",
					Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_FULL,
				},
			},
			wantErr:    true,
			errContain: "invalid gnuBuildID",
		},
		{
			name: "valid type empty build id",
			init: &debuginfov1alpha1.ShouldInitiateUploadRequest{
				File: &debuginfov1alpha1.FileMetadata{
					GNU:  "",
					Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_FULL,
				},
			},
			wantErr:    true,
			errContain: "invalid gnuBuildID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			id, err := validateInit(tt.init)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, id)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, id)
				assert.Equal(t, tt.init.File.GNU, id.gnuBuildID)
			}
		})
	}
}

func TestNewStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        Config
		bucket     func() *memory.InMemBucket
		wantErr    bool
		errContain string
	}{
		{
			name:   "enabled with bucket",
			cfg:    Config{Enabled: true},
			bucket: func() *memory.InMemBucket { return memory.NewInMemBucket() },
		},
		{
			name:       "enabled without bucket",
			cfg:        Config{Enabled: true},
			bucket:     func() *memory.InMemBucket { return nil },
			wantErr:    true,
			errContain: "enabled debug info requires a bucket",
		},
		{
			name:   "disabled without bucket",
			cfg:    Config{Enabled: false},
			bucket: func() *memory.InMemBucket { return nil },
		},
		{
			name:   "disabled with bucket",
			cfg:    Config{Enabled: false},
			bucket: func() *memory.InMemBucket { return memory.NewInMemBucket() },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := tt.bucket()
			var s *Store
			var err error
			if b != nil {
				s, err = NewStore(log.NewNopLogger(), b, tt.cfg)
			} else {
				s, err = NewStore(log.NewNopLogger(), nil, tt.cfg)
			}
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, s)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, s)
			}
		})
	}
}

func TestObjectPath(t *testing.T) {
	t.Parallel()

	id := mustValidateGnuBuildID(t, "aabbccdd")
	assert.Equal(t, "debug-info/tenant-1/aabbccdd/exe", ObjectPath("tenant-1", id))
	assert.Equal(t, "debug-info/org-42/aabbccdd/exe", ObjectPath("org-42", id))
}

func TestMetadataObjectPath(t *testing.T) {
	t.Parallel()

	id := mustValidateGnuBuildID(t, "aabbccdd")
	assert.Equal(t, "debug-info/tenant-1/aabbccdd/metadata", MetadataObjectPath("tenant-1", id))
	assert.Equal(t, "debug-info/org-42/aabbccdd/metadata", MetadataObjectPath("org-42", id))
}

func TestShouldInitiateUpload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		metadata   *debuginfov1alpha1.ObjectMetadata
		cfg        Config
		wantUpload bool
		wantReason string
		wantErr    bool
	}{
		{
			name:       "nil metadata first time seen",
			metadata:   nil,
			cfg:        Config{Enabled: true, MaxUploadDuration: time.Minute},
			wantUpload: true,
			wantReason: ReasonFirstTimeSeen,
		},
		{
			name: "uploading state stale",
			metadata: &debuginfov1alpha1.ObjectMetadata{
				State:     debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING,
				StartedAt: timestamppb.New(time.Now().Add(-1 * time.Hour)),
			},
			cfg:        Config{Enabled: true, MaxUploadDuration: time.Minute},
			wantUpload: true,
			wantReason: ReasonUploadStale,
		},
		{
			name: "uploading state not stale",
			metadata: &debuginfov1alpha1.ObjectMetadata{
				State:     debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING,
				StartedAt: timestamppb.New(time.Now()),
			},
			cfg:        Config{Enabled: true, MaxUploadDuration: time.Minute},
			wantUpload: false,
			wantReason: ReasonUploadInProgress,
		},
		{
			name: "uploaded state",
			metadata: &debuginfov1alpha1.ObjectMetadata{
				State: debuginfov1alpha1.ObjectMetadata_STATE_UPLOADED,
			},
			cfg:        Config{Enabled: true, MaxUploadDuration: time.Minute},
			wantUpload: false,
			wantReason: ReasonDebuginfoAlreadyExists,
		},
		{
			name: "unknown state",
			metadata: &debuginfov1alpha1.ObjectMetadata{
				State: debuginfov1alpha1.ObjectMetadata_State(99),
			},
			cfg:     Config{Enabled: true, MaxUploadDuration: time.Minute},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newTestStore(t, tt.cfg)
			resp, err := s.shouldInitiateUpload(tt.metadata)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.wantUpload, resp.ShouldInitiateUpload)
				assert.Equal(t, tt.wantReason, resp.Reason)
			}
		})
	}
}

func TestUploadIsStale(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		startedAt         time.Time
		maxUploadDuration time.Duration
		wantStale         bool
	}{
		{
			name:              "stale started long ago",
			startedAt:         time.Now().Add(-1 * time.Hour),
			maxUploadDuration: time.Minute,
			wantStale:         true,
		},
		{
			name:              "not stale started now",
			startedAt:         time.Now(),
			maxUploadDuration: time.Minute,
			wantStale:         false,
		},
		{
			name:              "not stale within grace period",
			startedAt:         time.Now().Add(-(time.Minute + time.Minute)),
			maxUploadDuration: time.Minute,
			wantStale:         false,
		},
		{
			name:              "stale just past threshold",
			startedAt:         time.Now().Add(-(time.Minute + 2*time.Minute + time.Second)),
			maxUploadDuration: time.Minute,
			wantStale:         true,
		},
		{
			name:              "not stale with long max duration",
			startedAt:         time.Now().Add(-5 * time.Minute),
			maxUploadDuration: 10 * time.Minute,
			wantStale:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, _ := newTestStore(t, Config{Enabled: true, MaxUploadDuration: tt.maxUploadDuration})
			md := &debuginfov1alpha1.ObjectMetadata{
				StartedAt: timestamppb.New(tt.startedAt),
			}
			assert.Equal(t, tt.wantStale, s.uploadIsStale(md))
		})
	}
}

func TestFetchMetadata(t *testing.T) {
	t.Parallel()

	const tenantID = "test-tenant"
	buildID := "aabbccdd"

	t.Run("not found returns nil", func(t *testing.T) {
		t.Parallel()
		s, _ := newTestStore(t, Config{Enabled: true})
		id := mustValidateGnuBuildID(t, buildID)

		md, err := s.fetchMetadata(context.Background(), tenantID, id)
		require.NoError(t, err)
		assert.Nil(t, md)
	})

	t.Run("valid metadata", func(t *testing.T) {
		t.Parallel()
		s, bucket := newTestStore(t, Config{Enabled: true})
		id := mustValidateGnuBuildID(t, buildID)

		original := &debuginfov1alpha1.ObjectMetadata{
			File: &debuginfov1alpha1.FileMetadata{
				GNU:  buildID,
				Name: "test-binary",
				Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_FULL,
			},
			State:     debuginfov1alpha1.ObjectMetadata_STATE_UPLOADED,
			StartedAt: timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
		}
		data, err := protojson.Marshal(original)
		require.NoError(t, err)
		bucket.Set(MetadataObjectPath(tenantID, id), data)

		md, err := s.fetchMetadata(context.Background(), tenantID, id)
		require.NoError(t, err)
		require.NotNil(t, md)
		assert.Equal(t, debuginfov1alpha1.ObjectMetadata_STATE_UPLOADED, md.State)
		assert.Equal(t, buildID, md.File.GNU)
		assert.Equal(t, "test-binary", md.File.Name)
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		t.Parallel()
		s, bucket := newTestStore(t, Config{Enabled: true})
		id := mustValidateGnuBuildID(t, buildID)

		bucket.Set(MetadataObjectPath(tenantID, id), []byte("not valid json"))

		md, err := s.fetchMetadata(context.Background(), tenantID, id)
		require.Error(t, err)
		assert.Nil(t, md)
		assert.Contains(t, err.Error(), "unmarshal")
	})
}

func TestWriteMetadata(t *testing.T) {
	t.Parallel()

	const tenantID = "test-tenant"
	buildID := "aabbccdd"

	t.Run("write and verify bucket contents", func(t *testing.T) {
		t.Parallel()
		s, bucket := newTestStore(t, Config{Enabled: true})
		id := mustValidateGnuBuildID(t, buildID)

		md := &debuginfov1alpha1.ObjectMetadata{
			File: &debuginfov1alpha1.FileMetadata{
				GNU:  buildID,
				Name: "my-binary",
				Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_FULL,
			},
			State:     debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING,
			StartedAt: timestamppb.New(time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC)),
		}

		err := s.writeMetadata(context.Background(), tenantID, id, md)
		require.NoError(t, err)

		objects := bucket.Objects()
		raw, ok := objects[MetadataObjectPath(tenantID, id)]
		require.True(t, ok, "metadata object should exist in bucket")

		var stored debuginfov1alpha1.ObjectMetadata
		require.NoError(t, protojson.Unmarshal(raw, &stored))
		assert.Equal(t, debuginfov1alpha1.ObjectMetadata_STATE_UPLOADING, stored.State)
		assert.Equal(t, buildID, stored.File.GNU)
	})

	t.Run("write then fetch roundtrip", func(t *testing.T) {
		t.Parallel()
		s, _ := newTestStore(t, Config{Enabled: true})
		id := mustValidateGnuBuildID(t, buildID)

		original := &debuginfov1alpha1.ObjectMetadata{
			File: &debuginfov1alpha1.FileMetadata{
				GNU:  buildID,
				Name: "roundtrip-binary",
				Type: debuginfov1alpha1.FileMetadata_DEBUGINFO_TYPE_EXECUTABLE_NO_TEXT,
			},
			State:      debuginfov1alpha1.ObjectMetadata_STATE_UPLOADED,
			StartedAt:  timestamppb.New(time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)),
			FinishedAt: timestamppb.New(time.Date(2025, 3, 1, 10, 1, 0, 0, time.UTC)),
		}

		err := s.writeMetadata(context.Background(), tenantID, id, original)
		require.NoError(t, err)

		fetched, err := s.fetchMetadata(context.Background(), tenantID, id)
		require.NoError(t, err)
		require.NotNil(t, fetched)
		assert.Equal(t, original.State, fetched.State)
		assert.Equal(t, original.File.GNU, fetched.File.GNU)
		assert.Equal(t, original.File.Name, fetched.File.Name)
		assert.Equal(t, original.File.Type, fetched.File.Type)
		assert.Equal(t, original.StartedAt.AsTime(), fetched.StartedAt.AsTime())
		assert.Equal(t, original.FinishedAt.AsTime(), fetched.FinishedAt.AsTime())
	})
}
