// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/s3/config_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package s3

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"

	"github.com/grafana/dskit/flagext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		setup    func() *SSEConfig
		expected error
	}{
		"should pass with default config": {
			setup: func() *SSEConfig {
				cfg := &SSEConfig{}
				flagext.DefaultValues(cfg)

				return cfg
			},
		},
		"should fail on invalid SSE type": {
			setup: func() *SSEConfig {
				return &SSEConfig{
					Type: "unknown",
				}
			},
			expected: errUnsupportedSSEType,
		},
		"should fail on invalid SSE KMS encryption context": {
			setup: func() *SSEConfig {
				return &SSEConfig{
					Type:                 SSEKMS,
					KMSEncryptionContext: "!{}!",
				}
			},
			expected: errInvalidSSEContext,
		},
		"should pass on valid SSE KMS encryption context": {
			setup: func() *SSEConfig {
				return &SSEConfig{
					Type:                 SSEKMS,
					KMSEncryptionContext: `{"department": "10103.0"}`,
				}
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testData.expected, testData.setup().Validate())
		})
	}
}

func TestSSEConfig_BuildMinioConfig(t *testing.T) {
	tests := map[string]struct {
		cfg             *SSEConfig
		expectedType    string
		expectedKeyID   string
		expectedContext string
	}{
		"SSE KMS without encryption context": {
			cfg: &SSEConfig{
				Type:     SSEKMS,
				KMSKeyID: "test-key",
			},
			expectedType:    "aws:kms",
			expectedKeyID:   "test-key",
			expectedContext: "",
		},
		"SSE KMS with encryption context": {
			cfg: &SSEConfig{
				Type:                 SSEKMS,
				KMSKeyID:             "test-key",
				KMSEncryptionContext: "{\"department\":\"10103.0\"}",
			},
			expectedType:    "aws:kms",
			expectedKeyID:   "test-key",
			expectedContext: "{\"department\":\"10103.0\"}",
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			sse, err := testData.cfg.BuildMinioConfig()
			require.NoError(t, err)

			headers := http.Header{}
			sse.Marshal(headers)

			assert.Equal(t, testData.expectedType, headers.Get("x-amz-server-side-encryption"))
			assert.Equal(t, testData.expectedKeyID, headers.Get("x-amz-server-side-encryption-aws-kms-key-id"))
			assert.Equal(t, base64.StdEncoding.EncodeToString([]byte(testData.expectedContext)), headers.Get("x-amz-server-side-encryption-context"))
		})
	}
}

func TestParseKMSEncryptionContext(t *testing.T) {
	actual, err := parseKMSEncryptionContext("")
	assert.NoError(t, err)
	assert.Equal(t, map[string]string(nil), actual)

	expected := map[string]string{
		"department": "10103.0",
	}
	actual, err = parseKMSEncryptionContext(`{"department": "10103.0"}`)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)
}

func TestConfig_Validate(t *testing.T) {
	tests := map[string]struct {
		setup    func() *Config
		expected error
	}{
		"should pass with default config": {
			setup: func() *Config {
				cfg := &Config{}
				flagext.DefaultValues(cfg)

				return cfg
			},
		},
		"should fail on invalid bucket lookup style": {
			setup: func() *Config {
				cfg := &Config{}
				flagext.DefaultValues(cfg)
				cfg.BucketLookupType = "invalid"
				return cfg
			},
			expected: errUnsupportedBucketLookupType,
		},
		"should fail if force-path-style conflicts with bucket-lookup-type": {
			setup: func() *Config {
				cfg := &Config{}
				flagext.DefaultValues(cfg)
				cfg.ForcePathStyle = true
				cfg.BucketLookupType = VirtualHostedStyleLookup
				return cfg
			},
			expected: errBucketLookupConfigConflict,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			assert.Equal(t, testData.expected, testData.setup().Validate())
		})
	}
}

type testRoundTripper struct {
	roundTrip func(r *http.Request) (*http.Response, error)
}

func (t *testRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return t.roundTrip(r)
}

func handleSTSRequest(t *testing.T, r *http.Request, w http.ResponseWriter) {
	body, err := io.ReadAll(r.Body)
	require.NoError(t, err)

	require.Contains(t, string(body), "RoleArn=arn%3Ahello-world")
	require.Contains(t, string(body), "WebIdentityToken=my-web-token")
	require.Contains(t, string(body), "Action=AssumeRoleWithWebIdentity")

	w.WriteHeader(200)
	_, err = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
				<AssumeRoleWithWebIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
				  <AssumeRoleWithWebIdentityResult>
				    <Credentials>
				      <AccessKeyId>test-key</AccessKeyId>
				      <SecretAccessKey>test-secret</SecretAccessKey>
				      <SessionToken>test-token</SessionToken>
				      <Expiration>` + time.Now().Add(time.Hour).Format(time.RFC3339) + `</Expiration>
				    </Credentials>
				  </AssumeRoleWithWebIdentityResult>
				  <ResponseMetadata>
				    <RequestId>test-request-id</RequestId>
				  </ResponseMetadata>
				</AssumeRoleWithWebIdentityResponse>`))
	require.NoError(t, err)

}

func overrideEnv(t testing.TB, kv ...string) {
	old := make([]string, len(kv))
	for i := 0; i < len(kv); i += 2 {
		k := kv[i]
		v := kv[i+1]
		old[i] = k
		old[i+1] = os.Getenv(k)
		os.Setenv(k, v)
	}
	t.Cleanup(func() {
		for i := 0; i < len(old); i += 2 {
			os.Setenv(old[i], old[i+1])
		}
	})
}

func TestAWSSTSWebIdentity(t *testing.T) {
	logger := log.NewNopLogger()
	tmpDir := t.TempDir()

	// override env variables, will be cleaned up by t.Cleanup
	overrideEnv(t,
		"AWS_WEB_IDENTITY_TOKEN_FILE", tmpDir+"/token",
		"AWS_ROLE_ARN", "arn:hello-world",
		"AWS_DEFAULT_REGION", "eu-central-1",
		"AWS_CONFIG_FILE", "/dev/null", // dont accidentally use real config
		"AWS_ACCESS_KEY_ID", "", // dont use real credentials
		"AWS_SECRET_ACCESS_KEY", "", // dont use real credentials
	)

	rt := &testRoundTripper{
		roundTrip: func(r *http.Request) (*http.Response, error) {
			w := httptest.NewRecorder()
			if r.Body != nil {
				defer r.Body.Close()
			}
			switch r.URL.String() {
			case "https://sts.amazonaws.com":
				handleSTSRequest(t, r, w)
			case "https://eu-central-1.amazonaws.com/pyroscope-test-bucket/test":
				assert.Equal(t, "GET", r.Method)
				assert.Contains(t, r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=test-key")
				w.Header().Set("Last-Modified", time.Now().Format("Mon, 2 Jan 2006 15:04:05 GMT"))
				w.WriteHeader(200)
				_, err := w.Write([]byte("test"))
				require.NoError(t, err)
			default:
				w.WriteHeader(404)
				_, err := w.Write([]byte("unexpected"))
				require.NoError(t, err)
				t.Errorf("unexpected request: %s", r.URL.Host)
				t.FailNow()
			}
			return w.Result(), nil
		},
	}
	oldDefaultTransport := http.DefaultTransport
	oldDefaultClient := http.DefaultClient
	http.DefaultTransport = rt
	http.DefaultClient = &http.Client{
		Transport: rt,
	}
	// restore default transport and client
	t.Cleanup(func() {
		http.DefaultTransport = oldDefaultTransport
		http.DefaultClient = oldDefaultClient
	})

	// mock a web token
	err := os.WriteFile(tmpDir+"/token", []byte("my-web-token"), 0644)
	require.NoError(t, err)

	cfg := Config{
		SignatureVersion: SignatureVersionV4,
		BucketName:       "pyroscope-test-bucket",
		Region:           "eu-central-1",
		Endpoint:         "eu-central-1.amazonaws.com",
		BucketLookupType: AutoLookup,
	}

	cfg.HTTP.Transport = rt
	r, err := NewBucketClient(cfg, "test", logger)
	require.NoError(t, err)

	_, err = r.Get(context.Background(), "test")
	require.NoError(t, err)

}
