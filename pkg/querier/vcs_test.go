package querier

import (
	"context"
	"encoding/base64"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

func TestResolveSymbolPath(t *testing.T) {
	// Loki
	// $GOROOT/src/unicode/utf8/utf8.go
	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go
	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/builder.go
	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/string.go
	// $GOROOT/src/vendor/golang.org/x/net/dns/dnsmessage/message.go
	// $GOROOT/src/vendor/golang.org/x/net/http/httpguts/httplex.go
	// $GOROOT/src/vendor/golang.org/x/net/http/httpproxy/proxy.go
	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/encode.go
	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/hpack.go
	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/huffman.go
	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/tables.go
	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/codec.go
	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/grpc.go
	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/middleware.go
	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/propagation.go
	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/request_chunk_filterer.go
	// /src/enterprise-logs/vendor/github.com/DmitriyVTitov/size/size.go

	// Mimir
	// $GOROOT/src/vendor/golang.org/x/crypto/hkdf/hkdf.go
	// $GOROOT/src/vendor/golang.org/x/net/dns/dnsmessage/message.go
	// $GOROOT/src/vendor/golang.org/x/net/http/httpguts/httplex.go
	// /drone/src/pkg/enterprise/admin/usage/quota.go
	// /drone/src/pkg/enterprise/mimir/instrumentation/node/diskstats.go
	// /drone/src/pkg/enterprise/mimir/instrumentation/node/filesystem.go
	// /drone/src/pkg/enterprise/mimir/instrumentation/node/node.go
	// /drone/src/pkg/enterprise/mimir/instrumentation/node/vmstat.go
	// /drone/src/pkg/enterprise/mimir/version/version.go
	// /drone/src/vendor/cloud.google.com/go/internal/retry.go
	// /drone/src/vendor/cloud.google.com/go/internal/trace/trace.go
	// /drone/src/vendor/cloud.google.com/go/storage/acl.go
	// /drone/src/vendor/cloud.google.com/go/storage/bucket.go

	// Pyroscope prod
	// fmt/errors.go
	// fmt/format.go
	// fmt/print.go
	// fmt/scan.go
	// github.com/armon/go-metrics@v0.4.1/metrics.go
	// github.com/armon/go-metrics@v0.4.1/prometheus/prometheus.go
	// github.com/armon/go-metrics@v0.4.1/start.go
	// github.com/grafana/pyroscope-go/godeltaprof@v0.1.6/internal/pprof/protobuf.go
	// github.com/grafana/pyroscope/api/gen/proto/go/google/v1/profile.pb.go
	// github.com/grafana/pyroscope/api/gen/proto/go/google/v1/profile_vtproto.pb.go

	// Tempo
	// /drone/src/cmd/tempo/app/app.go
	// /drone/src/cmd/tempo/app/config.go
	// /drone/src/modules/distributor/distributor.go
	// /drone/src/modules/distributor/forwarder.go
	// /drone/src/tempodb/backend/gcs/gcs.go
	// /drone/src/tempodb/backend/instrumentation/backend_transports.go
	// /drone/src/vendor/cloud.google.com/go/internal/retry.go
	// /drone/src/vendor/github.com/go-logfmt/logfmt/jsonstring.go
	// /drone/src/vendor/github.com/gogo/protobuf/proto/clone.go
	// /drone/src/vendor/google.golang.org/protobuf/proto/merge.go
	// /usr/local/go/src/bufio/bufio.go
	// /usr/local/go/src/bufio/scan.go
	// /usr/local/go/src/bytes/buffer.go

	// kine
	// /go/pkg/mod/github.com/cespare/xxhash/v2@v2.2.0/xxhash.go
	// /go/pkg/mod/github.com/cespare/xxhash/v2@v2.2.0/xxhash_unsafe.go
	// /go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/proto.go
	// /go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/wire.go
	// /go/src/github.com/k3s-io/kine/pkg/broadcaster/broadcaster.go
	// /go/src/github.com/k3s-io/kine/pkg/drivers/cockroach/cockroach.go
	// /go/src/github.com/k3s-io/kine/pkg/drivers/generic/generic.go
	// /usr/local/go/src/bufio/bufio.go
	// /usr/local/go/src/bufio/scan.go
	// /usr/local/go/src/bytes/buffer.go
	// /usr/local/go/src/bytes/bytes.go

	// grafana
	// /drone/src/pkg/web/router.go
	// /drone/src/pkg/web/tree.go
	// /opt/drone/gomodcache/github.com/!f!zambia/eagle@v0.0.2/eagle.go
	// /opt/drone/gomodcache/github.com/centrifugal/centrifuge@v0.29.1/broker_memory.go
	// /opt/drone/gomodcache/github.com/centrifugal/centrifuge@v0.29.1/hub.go
	// /usr/local/go/src/crypto/internal/edwards25519/field/fe_amd64.s
	// /usr/local/go/src/crypto/rsa/pss.go
	// /usr/local/go/src/crypto/rsa/rsa.go
	// vendor/golang.org/x/crypto/cryptobyte/asn1.go
	// vendor/golang.org/x/crypto/cryptobyte/builder.go
	// vendor/golang.org/x/crypto/cryptobyte/string.go
	// vendor/golang.org/x/crypto/hkdf/hkdf.go
	// vendor/golang.org/x/net/dns/dnsmessage/message.go
	// vendor/golang.org/x/net/http/httpguts/httplex.go
	// vendor/golang.org/x/net/http2/hpack/encode.go
	// vendor/golang.org/x/net/http2/hpack/hpack.go
	// vendor/golang.org/x/net/http2/hpack/huffman.go
	// vendor/golang.org/x/net/http2/hpack/tables.go

	for _, tt := range []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "github",
			input:    "github.com/grafana/grafana/pkg/querier/vcs.go",
			expected: "github.com/grafana/grafana/pkg/querier/vcs.go",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// actual := resolveSymbolPath(tt.input)
			// if actual != tt.expected {
			// 	t.Errorf("expected %s, got %s", tt.expected, actual)
			// }
		})
	}
}

func Test_FindGoStdFile(t *testing.T) {
	// /usr/local/go/src/bufio/bufio.go
	// $GOROOT/src/unicode/utf8/utf8.go
	// fmt/scan.go
	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go
	// /usr/local/go/src/vendor/golang.org/x/net/http2/hpack/tables.go

	f, err := findGoStdFile(context.Background(), fileFinder{
		path: "/usr/local/go/src/bufio/bufio.go",
	})
	require.NoError(t, err)
	require.NotNil(t, f)
}

func Test_FindGoDependencyFile(t *testing.T) {
	// github.com/armon/go-metrics@v0.4.1/metrics.go
	// github.com/grafana/pyroscope-go/godeltaprof@v0.1.6/internal/pprof/protobuf.go
	// /go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/wire.go
	// /drone/src/vendor/cloud.google.com/go/storage/bucket.go
	// /src/enterprise-logs/vendor/github.com/DmitriyVTitov/size/size.go
	// /drone/src/vendor/google.golang.org/protobuf/proto/merge.go

	f, err := findGoDependencyFile(context.Background(), fileFinder{
		path: "/drone/src/vendor/cloud.google.com/go/storage/bucket.go",
	})
	require.NoError(t, err)
	require.NotNil(t, f)
}

func Test_ParseModulePath(t *testing.T) {
	for _, tt := range []struct {
		input      string
		expectedOk bool
		expected   goModuleFile
	}{
		{
			"github.com/armon/go-metrics@v0.4.1/metrics.go",
			true,
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/armon/go-metrics",
					Version: "v0.4.1",
				},
				filePath: "metrics.go",
			},
		},
		{
			"/go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/wire.go",
			true,
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/golang/protobuf",
					Version: "v1.5.3",
				},
				filePath: "proto/wire.go",
			},
		},
		{
			"/go/pkg/mod/golang.org/x/net@v1.5.3/http2/hpack/tables.go",
			true,
			goModuleFile{
				Version: module.Version{
					Path:    "golang.org/x/net",
					Version: "v1.5.3",
				},
				filePath: "http2/hpack/tables.go",
			},
		},
	} {
		t.Run(tt.input, func(t *testing.T) {
			mod, ok := parseGoModuleFilePath(tt.input)
			require.Equal(t, tt.expectedOk, ok)
			require.Equal(t, tt.expected, mod)
		})
	}
}

func Test_ApplyGoModule(t *testing.T) {
	modf, err := modfile.Parse(
		"go.mod",
		[]byte(`
module github.com/grafana/pyroscope

go 1.16

require (
	github.com/grafana/grafana-plugin-sdk-go v0.3.0
	connectrpc.com/connect v1.14.0
	connectrpc.com/grpchealth v1.3.0
)

require (
	github.com/!azure/go-autorest v0.11.16-0.20210324193631-8d5b6a9c4f9e // indirect
	golang.org/x/crypto v0.0.0-20210322153248-8c942d7d6b5c // indirect
	golang.org/x/term v0.15.0 // indirect
	golang.org/x/tools v0.15.0 // indirect
	google.golang.org/api v0.152.0 // indirect
	k8s.io/utils v0.0.0-20230711102312-30195339c3c7 // indirect
)

replace (
	github.com/thanos-io/objstore => github.com/grafana/objstore v0.0.0-20231121154247-84f91ea90e72
	gopkg.in/yaml.v3 => github.com/colega/go-yaml-yaml v0.0.0-20220720105220-255a8d16d094
	github.com/grafana/pyroscope/api => ./api
)
		`), nil)
	require.NoError(t, err)
	for _, tt := range []struct {
		in       goModuleFile
		expected goModuleFile
	}{
		{
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go",
					Version: "v0.4.0",
				},
				filePath: "plugin.go",
			},
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go",
					Version: "v0.3.0",
				},
				filePath: "plugin.go",
			},
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/thanos-io/objstore",
					Version: "v0.4.0",
				},
				filePath: "client.go",
			},
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/objstore",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/pyroscope/api",
					Version: "v0.4.0",
				},
				filePath: "foo_gen.go",
			},
			goModuleFile{
				Version: module.Version{
					Path: "./api",
					// todo the version should be set to the current version of the module
				},
				filePath: "foo_gen.go",
			},
		},
	} {
		t.Run(tt.in.String(), func(t *testing.T) {
			actual := applyGoModule(modf, tt.in)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_ParseGithubRepo(t *testing.T) {
	for _, tt := range []struct {
		input       goModuleFile
		expected    GitHubFile
		expectedErr bool
	}{
		{
			goModuleFile{},
			GitHubFile{},
			true,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path: "github.com/grafana/grafana-plugin-sdk-go",
				},
			},
			GitHubFile{
				Owner: "grafana",
				Repo:  "grafana-plugin-sdk-go",
				Ref:   "main",
			},
			false,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go",
					Version: "v0.3.0",
				},
				filePath: "plugin.go",
			},
			GitHubFile{
				Owner: "grafana",
				Repo:  "grafana-plugin-sdk-go",
				Ref:   "v0.3.0",
				Path:  "plugin.go",
			},
			false,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go/bar",
					Version: "v0.3.0-rc1",
				},
				filePath: "plugin.go",
			},
			GitHubFile{
				Owner: "grafana",
				Repo:  "grafana-plugin-sdk-go",
				Ref:   "v0.3.0-rc1",
				Path:  "bar/plugin.go",
			},
			false,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go/bar",
					Version: "v0.14.1-0.20230710114240-c316eb95ae5b",
				},
				filePath: "plugin.go",
			},
			GitHubFile{
				Owner: "grafana",
				Repo:  "grafana-plugin-sdk-go",
				Ref:   "c316eb95ae5b",
				Path:  "bar/plugin.go",
			},
			false,
		},
	} {
		t.Run(tt.input.String(), func(t *testing.T) {
			actual, err := parseGithubRepo(tt.input)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_refFromVersion(t *testing.T) {
	for _, tt := range []struct {
		input    string
		expected string
	}{
		{
			"v0.3.0",
			"v0.3.0",
		},
		{
			"v0.3.0-rc1",
			"v0.3.0-rc1",
		},
		{
			"v0.3.0-0.20230710114240-c316eb95ae5b",
			"c316eb95ae5b",
		},
		{
			"v2.2.6+incompatible",
			"v2.2.6",
		},
	} {
		t.Run(tt.input, func(t *testing.T) {
			actual, err := refFromVersion(tt.input)
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_ResolveVanityGoModule(t *testing.T) {
	for _, tt := range []struct {
		in          goModuleFile
		expected    goModuleFile
		expectedErr error
	}{
		{
			goModuleFile{
				Version: module.Version{
					Path: "gopkg.in/yaml.v3",
				},
			},
			goModuleFile{
				Version: module.Version{
					Path: "github.com/go-yaml/yaml",
				},
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path: "gopkg.in/alecthomas/kingpin.v2",
				},
			},
			goModuleFile{
				Version: module.Version{
					Path: "github.com/alecthomas/kingpin",
				},
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path: "golang.org/x/oauth2",
				},
			},
			goModuleFile{
				Version: module.Version{
					Path: "go.googlesource.com/oauth2",
				},
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "k8s.io/utils",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/kubernetes/utils",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "cloud.google.com/go/storage",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/googleapis/google-cloud-go/storage",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path: "github.com/oklog/ulid",
				},
				filePath: "foo.go",
			},
			goModuleFile{
				Version: module.Version{
					Path: "github.com/oklog/ulid",
				},
				filePath: "foo.go",
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path: "google.golang.org/genproto/googleapis/api",
				},
				filePath: "foo.go",
			},
			goModuleFile{
				Version: module.Version{
					Path: "github.com/googleapis/go-genproto/googleapis/api",
				},
				filePath: "foo.go",
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path:    "google.golang.org/protobuf",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
			goModuleFile{
				Version: module.Version{
					Path:    "github.com/protocolbuffers/protobuf-go",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				filePath: "client.go",
			},
			nil,
		},
		{
			goModuleFile{
				Version: module.Version{
					Path: "connectrpc.com/grpchealth",
				},
				filePath: "client.go",
			},
			goModuleFile{
				Version: module.Version{
					Path: "github.com/connectrpc/grpchealth-go",
				},
				filePath: "client.go",
			},
			nil,
		},
	} {
		tt := tt
		t.Run(tt.in.String(), func(t *testing.T) {
			t.Parallel()
			actual, err := resolveVanityGoModule(context.Background(), tt.in, http.DefaultClient)
			require.Equal(t, tt.expectedErr, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_FetchGoogleSourceDependencyFile(t *testing.T) {
	content, err := fetchGoogleSourceDependencyFile(context.Background(), goModuleFile{
		Version: module.Version{
			Path:    "go.googlesource.com/oauth2",
			Version: "v0.16.0",
		},
		filePath: "amazon/amazon.go",
	}, http.DefaultClient)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(content.Content)
	require.NoError(t, err)
	require.Contains(t, string(decoded), "package amazon")
	require.Equal(t, "https://go.googlesource.com/oauth2/+/v0.16.0/amazon/amazon.go?format=TEXT", content.URL)
}

func Test_FetchGoStd(t *testing.T) {
	file, err := fetchGoStd(context.Background(), "bufio/bufio.go", "master")
	require.NoError(t, err)
	fileContent, err := base64.StdEncoding.DecodeString(file.Content)
	require.NoError(t, err)

	// todo mock
	require.Contains(t, string(fileContent), "// Copyright 2009 The Go Authors.")
	require.Equal(t, `https://raw.githubusercontent.com/golang/go/master/src/bufio/bufio.go`, file.URL)
}
