// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/client_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package client

import (
	"bytes"
	"context"
	"os"
	"path"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/flagext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/grafana/pyroscope/pkg/objstore/providers/filesystem"
)

const (
	configWithS3Backend = `
backend: s3
s3:
  endpoint:          localhost
  bucket_name:       test
  access_key_id:     xxx
  secret_access_key: yyy
  insecure:          true
`

	configWithGCSBackend = `
backend: gcs
gcs:
  bucket_name:     test
  service_account: |-
    {
      "type": "service_account",
      "project_id": "id",
      "private_key_id": "id",
      "private_key": "-----BEGIN PRIVATE KEY-----\nSOMETHING\n-----END PRIVATE KEY-----\n",
      "client_email": "test@test.com",
      "client_id": "12345",
      "auth_uri": "https://accounts.google.com/o/oauth2/auth",
      "token_uri": "https://oauth2.googleapis.com/token",
      "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
      "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/test%40test.com"
    }
`

	configWithUnknownBackend = `
backend: unknown
`
)

func TestNewClient(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		config      string
		expectedErr error
	}{
		"should create an S3 bucket": {
			config:      configWithS3Backend,
			expectedErr: nil,
		},
		"should create a GCS bucket": {
			config:      configWithGCSBackend,
			expectedErr: nil,
		},
		"should return error on unknown backend": {
			config:      configWithUnknownBackend,
			expectedErr: ErrUnsupportedStorageBackend,
		},
	}

	for testName, testData := range tests {
		testData := testData

		t.Run(testName, func(t *testing.T) {
			// Load config
			cfg := Config{}
			flagext.DefaultValues(&cfg)

			err := yaml.Unmarshal([]byte(testData.config), &cfg)
			require.NoError(t, err)

			// Instance a new bucket client from the config
			bucketClient, err := NewBucket(context.Background(), cfg, "test")
			require.Equal(t, testData.expectedErr, err)

			if testData.expectedErr == nil {
				require.NotNil(t, bucketClient)
				bucketClient.Close()
			} else {
				assert.Equal(t, nil, bucketClient)
			}
		})
	}
}

func TestClient_ConfigValidation(t *testing.T) {
	testCases := []struct {
		name          string
		cfg           Config
		expectedError error
		expectedLog   string
	}{
		{
			name: "prefix/valid",
			cfg:  Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "helloWORLD123"},
		},
		{
			name: "prefix/valid-subdir",
			cfg:  Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "hello/world/env"},
		},
		{
			name: "prefix/valid-subdir-trailing-slash",
			cfg:  Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "hello/world/env/"},
		},
		{
			name:          "prefix/invalid-directory-up",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: ".."},
			expectedError: ErrStoragePrefixInvalidCharacters,
		},
		{
			name:          "prefix/invalid-directory",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "."},
			expectedError: ErrStoragePrefixInvalidCharacters,
		},
		{
			name:          "prefix/invalid-absolute-path",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "/hello/world"},
			expectedError: ErrStoragePrefixStartsWithSlash,
		},
		{
			name:          "prefix/invalid-..-in-a-path-segement",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "hello/../test"},
			expectedError: ErrStoragePrefixInvalidCharacters,
		},
		{
			name:          "prefix/invalid-empty-path-segement",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "hello//test"},
			expectedError: ErrStoragePrefixEmptyPathSegment,
		},
		{
			name:          "prefix/invalid-emoji",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "👋"},
			expectedError: ErrStoragePrefixInvalidCharacters,
		},
		{
			name:          "prefix/invalid-emoji",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, Prefix: "hello!world"},
			expectedError: ErrStoragePrefixInvalidCharacters,
		},
		{
			name:          "unsupported backend",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: "flash drive"}},
			expectedError: ErrUnsupportedStorageBackend,
		},
		{
			name:        "prefix/valid-legacy-subdir-trailing-slash",
			cfg:         Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, DeprecatedStoragePrefix: "hello/world/env/"},
			expectedLog: "config has a deprecated storage.storage-prefix flag set",
		},
		{
			name:          "prefix/invalid-emoji",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, DeprecatedStoragePrefix: "hello!world"},
			expectedError: ErrStoragePrefixInvalidCharacters,
			expectedLog:   "config has a deprecated storage.storage-prefix flag set",
		},
		{
			name:          "prefix/invalid-both-configs",
			cfg:           Config{StorageBackendConfig: StorageBackendConfig{Backend: Filesystem}, DeprecatedStoragePrefix: "hello-world1", Prefix: "hello-world2"},
			expectedError: ErrStoragePrefixBothFlagsSet,
		},
	}

	logBuf := new(bytes.Buffer)
	logger := log.NewLogfmtLogger(logBuf)

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Reset log buffer
			logBuf.Reset()

			actualErr := tc.cfg.Validate(logger)
			if tc.expectedError != nil {
				assert.Equal(t, actualErr, tc.expectedError)
			} else {
				assert.NoError(t, actualErr)
			}
			if tc.expectedLog != "" {
				assert.Contains(t, logBuf.String(), tc.expectedLog)
			} else {
				assert.Empty(t, logBuf.String())
			}
		})
	}
}

func TestNewPrefixedBucketClient(t *testing.T) {
	t.Run("with prefix", func(t *testing.T) {
		ctx := context.Background()
		tempDir := t.TempDir()
		cfg := Config{
			StorageBackendConfig: StorageBackendConfig{
				Backend: Filesystem,
				Filesystem: filesystem.Config{
					Directory: tempDir,
				},
			},
			Prefix: "prefix",
		}

		client, err := NewBucket(ctx, cfg, "test")
		require.NoError(t, err)

		err = client.Upload(ctx, "file", bytes.NewBufferString("content"))
		assert.NoError(t, err)

		_, err = client.Get(ctx, "file")
		assert.NoError(t, err)

		filePath := path.Join(tempDir, "prefix", "file")
		assert.FileExists(t, filePath)

		b, err := os.ReadFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "content", string(b))
	})

	t.Run("without prefix", func(t *testing.T) {
		ctx := context.Background()
		tempDir := t.TempDir()
		cfg := Config{
			StorageBackendConfig: StorageBackendConfig{
				Backend: Filesystem,
				Filesystem: filesystem.Config{
					Directory: tempDir,
				},
			},
		}

		client, err := NewBucket(ctx, cfg, "test")
		require.NoError(t, err)
		err = client.Upload(ctx, "file", bytes.NewBufferString("content"))
		require.NoError(t, err)

		_, err = client.Get(ctx, "file")
		assert.NoError(t, err)

		filePath := path.Join(tempDir, "file")
		assert.FileExists(t, filePath)

		b, err := os.ReadFile(filePath)
		assert.NoError(t, err)
		assert.Equal(t, "content", string(b))
	})
}
