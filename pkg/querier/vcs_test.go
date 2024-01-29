package querier

// import (
// 	"context"
// 	"encoding/base64"
// 	"net/http"
// 	"testing"

// 	"github.com/stretchr/testify/require"
// 	"golang.org/x/mod/modfile"
// 	"golang.org/x/mod/module"
// )

// func TestResolveSymbolPath(t *testing.T) {
// 	// Loki
// 	// $GOROOT/src/unicode/utf8/utf8.go
// 	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go
// 	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/builder.go
// 	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/string.go
// 	// $GOROOT/src/vendor/golang.org/x/net/dns/dnsmessage/message.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http/httpguts/httplex.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http/httpproxy/proxy.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/encode.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/hpack.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/huffman.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http2/hpack/tables.go
// 	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/codec.go
// 	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/grpc.go
// 	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/middleware.go
// 	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/propagation.go
// 	// /src/enterprise-logs/pkg/enterprise/loki/labelaccess/request_chunk_filterer.go
// 	// /src/enterprise-logs/vendor/github.com/DmitriyVTitov/size/size.go

// 	// Mimir
// 	// $GOROOT/src/vendor/golang.org/x/crypto/hkdf/hkdf.go
// 	// $GOROOT/src/vendor/golang.org/x/net/dns/dnsmessage/message.go
// 	// $GOROOT/src/vendor/golang.org/x/net/http/httpguts/httplex.go
// 	// /drone/src/pkg/enterprise/admin/usage/quota.go
// 	// /drone/src/pkg/enterprise/mimir/instrumentation/node/diskstats.go
// 	// /drone/src/pkg/enterprise/mimir/instrumentation/node/filesystem.go
// 	// /drone/src/pkg/enterprise/mimir/instrumentation/node/node.go
// 	// /drone/src/pkg/enterprise/mimir/instrumentation/node/vmstat.go
// 	// /drone/src/pkg/enterprise/mimir/version/version.go
// 	// /drone/src/vendor/cloud.google.com/go/internal/retry.go
// 	// /drone/src/vendor/cloud.google.com/go/internal/trace/trace.go
// 	// /drone/src/vendor/cloud.google.com/go/storage/acl.go
// 	// /drone/src/vendor/cloud.google.com/go/storage/bucket.go

// 	// Pyroscope prod
// 	// fmt/errors.go
// 	// fmt/format.go
// 	// fmt/print.go
// 	// fmt/scan.go
// 	// github.com/armon/go-metrics@v0.4.1/metrics.go
// 	// github.com/armon/go-metrics@v0.4.1/prometheus/prometheus.go
// 	// github.com/armon/go-metrics@v0.4.1/start.go
// 	// github.com/grafana/pyroscope-go/godeltaprof@v0.1.6/internal/pprof/protobuf.go
// 	// github.com/grafana/pyroscope/api/gen/proto/go/google/v1/profile.pb.go
// 	// github.com/grafana/pyroscope/api/gen/proto/go/google/v1/profile_vtproto.pb.go

// 	// Tempo
// 	// /drone/src/cmd/tempo/app/app.go
// 	// /drone/src/cmd/tempo/app/config.go
// 	// /drone/src/modules/distributor/distributor.go
// 	// /drone/src/modules/distributor/forwarder.go
// 	// /drone/src/tempodb/backend/gcs/gcs.go
// 	// /drone/src/tempodb/backend/instrumentation/backend_transports.go
// 	// /drone/src/vendor/cloud.google.com/go/internal/retry.go
// 	// /drone/src/vendor/github.com/go-logfmt/logfmt/jsonstring.go
// 	// /drone/src/vendor/github.com/gogo/protobuf/proto/clone.go
// 	// /drone/src/vendor/google.golang.org/protobuf/proto/merge.go
// 	// /usr/local/go/src/bufio/bufio.go
// 	// /usr/local/go/src/bufio/scan.go
// 	// /usr/local/go/src/bytes/buffer.go

// 	// kine
// 	// /go/pkg/mod/github.com/cespare/xxhash/v2@v2.2.0/xxhash.go
// 	// /go/pkg/mod/github.com/cespare/xxhash/v2@v2.2.0/xxhash_unsafe.go
// 	// /go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/proto.go
// 	// /go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/wire.go
// 	// /go/src/github.com/k3s-io/kine/pkg/broadcaster/broadcaster.go
// 	// /go/src/github.com/k3s-io/kine/pkg/drivers/cockroach/cockroach.go
// 	// /go/src/github.com/k3s-io/kine/pkg/drivers/generic/generic.go
// 	// /usr/local/go/src/bufio/bufio.go
// 	// /usr/local/go/src/bufio/scan.go
// 	// /usr/local/go/src/bytes/buffer.go
// 	// /usr/local/go/src/bytes/bytes.go

// 	// grafana
// 	// /drone/src/pkg/web/router.go
// 	// /drone/src/pkg/web/tree.go
// 	// /opt/drone/gomodcache/github.com/!f!zambia/eagle@v0.0.2/eagle.go
// 	// /opt/drone/gomodcache/github.com/centrifugal/centrifuge@v0.29.1/broker_memory.go
// 	// /opt/drone/gomodcache/github.com/centrifugal/centrifuge@v0.29.1/hub.go
// 	// /usr/local/go/src/crypto/internal/edwards25519/field/fe_amd64.s
// 	// /usr/local/go/src/crypto/rsa/pss.go
// 	// /usr/local/go/src/crypto/rsa/rsa.go
// 	// vendor/golang.org/x/crypto/cryptobyte/asn1.go
// 	// vendor/golang.org/x/crypto/cryptobyte/builder.go
// 	// vendor/golang.org/x/crypto/cryptobyte/string.go
// 	// vendor/golang.org/x/crypto/hkdf/hkdf.go
// 	// vendor/golang.org/x/net/dns/dnsmessage/message.go
// 	// vendor/golang.org/x/net/http/httpguts/httplex.go
// 	// vendor/golang.org/x/net/http2/hpack/encode.go
// 	// vendor/golang.org/x/net/http2/hpack/hpack.go
// 	// vendor/golang.org/x/net/http2/hpack/huffman.go
// 	// vendor/golang.org/x/net/http2/hpack/tables.go

// 	for _, tt := range []struct {
// 		name     string
// 		input    string
// 		expected string
// 	}{
// 		{
// 			name:     "github",
// 			input:    "github.com/grafana/grafana/pkg/querier/vcs.go",
// 			expected: "github.com/grafana/grafana/pkg/querier/vcs.go",
// 		},
// 	} {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// actual := resolveSymbolPath(tt.input)
// 			// if actual != tt.expected {
// 			// 	t.Errorf("expected %s, got %s", tt.expected, actual)
// 			// }
// 		})
// 	}
// }

// func Test_FindGoStdFile(t *testing.T) {
// 	// /usr/local/go/src/bufio/bufio.go
// 	// $GOROOT/src/unicode/utf8/utf8.go
// 	// fmt/scan.go
// 	// $GOROOT/src/vendor/golang.org/x/crypto/cryptobyte/asn1.go
// 	// /usr/local/go/src/vendor/golang.org/x/net/http2/hpack/tables.go

// 	f, err := findGoStdFile(context.Background(), fileFinder{
// 		path: "/usr/local/go/src/bufio/bufio.go",
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, f)
// }

// func Test_FindGoDependencyFile(t *testing.T) {
// 	// github.com/armon/go-metrics@v0.4.1/metrics.go
// 	// github.com/grafana/pyroscope-go/godeltaprof@v0.1.6/internal/pprof/protobuf.go
// 	// /go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/wire.go
// 	// /drone/src/vendor/cloud.google.com/go/storage/bucket.go
// 	// /src/enterprise-logs/vendor/github.com/DmitriyVTitov/size/size.go
// 	// /drone/src/vendor/google.golang.org/protobuf/proto/merge.go

// 	f, err := findGoDependencyFile(context.Background(), fileFinder{
// 		path: "/drone/src/vendor/cloud.google.com/go/storage/bucket.go",
// 	})
// 	require.NoError(t, err)
// 	require.NotNil(t, f)
// }

// func Test_FetchGoogleSourceDependencyFile(t *testing.T) {
// 	content, err := fetchGoogleSourceDependencyFile(context.Background(), goModuleFile{
// 		Version: module.Version{
// 			Path:    "go.googlesource.com/oauth2",
// 			Version: "v0.16.0",
// 		},
// 		filePath: "amazon/amazon.go",
// 	}, http.DefaultClient)
// 	require.NoError(t, err)
// 	decoded, err := base64.StdEncoding.DecodeString(content.Content)
// 	require.NoError(t, err)
// 	require.Contains(t, string(decoded), "package amazon")
// 	require.Equal(t, "https://go.googlesource.com/oauth2/+/v0.16.0/amazon/amazon.go?format=TEXT", content.URL)
// }

// func Test_FetchGoStd(t *testing.T) {
// 	file, err := fetchGoStd(context.Background(), "bufio/bufio.go", "master")
// 	require.NoError(t, err)
// 	fileContent, err := base64.StdEncoding.DecodeString(file.Content)
// 	require.NoError(t, err)

// 	// todo mock
// 	require.Contains(t, string(fileContent), "// Copyright 2009 The Go Authors.")
// 	require.Equal(t, `https://raw.githubusercontent.com/golang/go/master/src/bufio/bufio.go`, file.URL)
// }
