package golang

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
)

var (
	modf       *modfile.File
	mainModule = module.Version{
		Path:    "github.com/grafana/pyroscope",
		Version: "v0.5.0",
	}
)

func init() {
	var err error
	modf, err = modfile.Parse(
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
	if err != nil {
		panic(err)
	}
}

func TestParseModulePath(t *testing.T) {
	for _, tt := range []struct {
		input      string
		expectedOk bool
		expected   Module
	}{
		{
			"github.com/armon/go-metrics@v0.4.1/metrics.go",
			true,
			Module{
				Version: module.Version{
					Path:    "github.com/armon/go-metrics",
					Version: "v0.4.1",
				},
				FilePath: "metrics.go",
			},
		},
		{
			"/go/pkg/mod/github.com/golang/protobuf@v1.5.3/proto/wire.go",
			true,
			Module{
				Version: module.Version{
					Path:    "github.com/golang/protobuf",
					Version: "v1.5.3",
				},
				FilePath: "proto/wire.go",
			},
		},
		{
			"/go/pkg/mod/golang.org/x/net@v1.5.3/http2/hpack/tables.go",
			true,
			Module{
				Version: module.Version{
					Path:    "golang.org/x/net",
					Version: "v1.5.3",
				},
				FilePath: "http2/hpack/tables.go",
			},
		},
		{
			"/go/pkg/mod/golang.org/x/net/http2/hpack/tables.go",
			false,
			Module{},
		},
		{
			"/go/pkg/mod/golang.org/x/net@v1.0tables.go",
			false,
			Module{},
		},
	} {
		t.Run(tt.input, func(t *testing.T) {
			mod, ok := ParseModuleFromPath(tt.input)
			require.Equal(t, tt.expectedOk, ok)
			require.Equal(t, tt.expected, mod)
		})
	}
}

func Test_ApplyGoModule(t *testing.T) {
	for _, tt := range []struct {
		in       Module
		expected Module
	}{
		{
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go",
					Version: "v0.4.0",
				},
				FilePath: "plugin.go",
			},
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go",
					Version: "v0.3.0",
				},
				FilePath: "plugin.go",
			},
		},
		{
			Module{
				Version: module.Version{
					Path:    "github.com/thanos-io/objstore",
					Version: "v0.4.0",
				},
				FilePath: "client.go",
			},
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/objstore",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				FilePath: "client.go",
			},
		},
		{
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/pyroscope/api",
					Version: "v0.4.0",
				},
				FilePath: "foo_gen.go",
			},
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/pyroscope/api",
					Version: "v0.5.0",
				},
				FilePath: "foo_gen.go",
			},
		},
	} {
		t.Run(tt.in.String(), func(t *testing.T) {
			tt.in.applyGoMod(mainModule, modf)
			require.Equal(t, tt.expected, tt.in)
		})
	}
}

type ClientMock struct{}

func (c *ClientMock) Do(req *http.Request) (*http.Response, error) {
	switch req.URL.String() {
	case "https://golang.org/x/oauth2?go-get=1":
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(
				strings.NewReader(`
			<html>
			<head>
			<meta name="go-import" content="golang.org/x/oauth2 git https://go.googlesource.com/oauth2">
			</head>
			</html>`,
				),
			),
		}, nil
	case "https://k8s.io/utils?go-get=1":
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(
				strings.NewReader(`
				<html><head>
				<meta name="go-import"
					  content="k8s.io/utils
							   git https://github.com/kubernetes/utils">
				<meta name="go-source"
					  content="k8s.io/utils
							   https://github.com/kubernetes/utils
							   https://github.com/kubernetes/utils/tree/master{/dir}
							   https://github.com/kubernetes/utils/blob/master{/dir}/{file}#L{line}">
		  </head></html>`,
				),
			),
		}, nil
	case "https://cloud.google.com/go/storage?go-get=1":
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(
				strings.NewReader(`
				<!DOCTYPE html>
				<html>
				<head>

					<meta name="go-import" content="cloud.google.com/go git https://github.com/googleapis/google-cloud-go">
					<meta name="go-source" content="cloud.google.com/go https://github.com/googleapis/google-cloud-go https://github.com/GoogleCloudPlatform/gcloud-golang/tree/master{/dir} https://github.com/GoogleCloudPlatform/gcloud-golang/tree/master{/dir}/{file}#L{line}">
					<meta name="robots" content="noindex">
					<meta http-equiv="refresh" content="0; url=https://pkg.go.dev/cloud.google.com/go/storage">
				</head>
				<body>
					Redirecting to <a href="https://pkg.go.dev/cloud.google.com/go/storage">pkg.go.dev/cloud.google.com/go/storage</a>.
				</body>
				</html>`,
				),
			),
		}, nil
	case "https://google.golang.org/protobuf?go-get=1":
		return &http.Response{
			StatusCode: 200,
			Body: io.NopCloser(
				strings.NewReader(`
				<!DOCTYPE html>
				<html>
				<head>
				<meta name="go-import" content="google.golang.org/protobuf git https://go.googlesource.com/protobuf">

				<meta name="go-source" content="google.golang.org/protobuf https://github.com/protocolbuffers/protobuf-go https://github.com/protocolbuffers/protobuf-go/tree/master{/dir} https://github.com/protocolbuffers/protobuf-go/tree/master{/dir}/{file}#L{line}">

				<meta http-equiv="refresh" content="0; url=https://pkg.go.dev/google.golang.org/protobuf">
				</head>
				<body>
				<a href="https://pkg.go.dev/google.golang.org/protobuf">Redirecting to documentation...</a>
				</body>
				</html>`,
				),
			),
		}, nil
	}
	return &http.Response{
		StatusCode: 404,
		Body:       http.NoBody,
	}, nil
}

func Test_Resolve(t *testing.T) {
	for _, tt := range []struct {
		in      Module
		main    module.Version
		modfile *modfile.File

		expected    Module
		expectedErr bool
	}{
		{
			expectedErr: true,
		},
		{
			in: Module{
				Version: module.Version{
					Path: "gopkg.in/yaml.v3",
				},
			},
			main:    mainModule,
			modfile: modf,
			expected: Module{
				Version: module.Version{
					Path:    "github.com/colega/go-yaml-yaml",
					Version: "v0.0.0-20220720105220-255a8d16d094",
				},
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path: "gopkg.in/yaml.v3",
				},
			},
			main: mainModule,
			expected: Module{
				Version: module.Version{
					Path: "github.com/go-yaml/yaml",
				},
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path: "github.com/go-yaml/yaml/v56",
				},
			},
			main: mainModule,
			expected: Module{
				Version: module.Version{
					Path: "github.com/go-yaml/yaml",
				},
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path: "gopkg.in/alecthomas/kingpin.v2",
				},
			},
			main:    mainModule,
			modfile: modf,
			expected: Module{
				Version: module.Version{
					Path: "github.com/alecthomas/kingpin",
				},
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path: "gopkg.in/kingpin.v2",
				},
			},
			expected: Module{
				Version: module.Version{
					Path: "github.com/go-kingpin/kingpin",
				},
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path: "golang.org/x/oauth2",
				},
			},
			main:    mainModule,
			modfile: modf,
			expected: Module{
				Version: module.Version{
					Path: "go.googlesource.com/oauth2",
				},
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path:    "k8s.io/utils",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				FilePath: "client.go",
			},
			main:    mainModule,
			modfile: modf,
			expected: Module{
				Version: module.Version{
					Path:    "github.com/kubernetes/utils",
					Version: "v0.0.0-20230711102312-30195339c3c7",
				},
				FilePath: "client.go",
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path:    "cloud.google.com/go/storage",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				FilePath: "client.go",
			},
			main:    mainModule,
			modfile: modf,
			expected: Module{
				Version: module.Version{
					Path:    "github.com/googleapis/google-cloud-go/storage",
					Version: "v0.0.0-20231121154247-84f91ea90e72",
				},
				FilePath: "client.go",
			},
		},
		{
			in: Module{
				Version: module.Version{
					Path: "google.golang.org/protobuf",
				},
			},
			main:    mainModule,
			modfile: modf,
			expected: Module{
				Version: module.Version{
					Path: "github.com/protocolbuffers/protobuf-go",
				},
			},
		},
	} {
		t.Run(tt.in.String(), func(t *testing.T) {
			err := tt.in.Resolve(context.Background(), tt.main, tt.modfile, &ClientMock{})
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, tt.in)
		})
	}
}

func Test_ParseGithubRepo(t *testing.T) {
	for _, tt := range []struct {
		input       Module
		expected    GitHubFile
		expectedErr bool
	}{
		{
			Module{},
			GitHubFile{},
			true,
		},
		{
			Module{
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
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go",
					Version: "v0.3.0",
				},
				FilePath: "plugin.go",
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
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go/bar",
					Version: "v0.3.0-rc1",
				},
				FilePath: "plugin.go",
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
			Module{
				Version: module.Version{
					Path:    "github.com/grafana/grafana-plugin-sdk-go/bar",
					Version: "v0.14.1-0.20230710114240-c316eb95ae5b",
				},
				FilePath: "plugin.go",
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
			actual, err := tt.input.GithubFile()
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_GoogleSourceURL(t *testing.T) {
	for _, tt := range []struct {
		in          Module
		expected    string
		expectedErr bool
	}{
		{
			Module{},
			"",
			true,
		},
		{
			Module{
				Version: module.Version{
					Path: "go.googlesource.com/",
				},
			},
			"",
			true,
		},
		{
			Module{
				Version: module.Version{
					Path: "go.googlesource.com/oauth2",
				},
				FilePath: "amazon/amazon.go",
			},
			"https://go.googlesource.com/oauth2/+/master/amazon/amazon.go?format=TEXT",
			false,
		},
		{
			Module{
				Version: module.Version{
					Path: "go.googlesource.com/oauth2/amazon",
				},
				FilePath: "amazon.go",
			},
			"https://go.googlesource.com/oauth2/+/master/amazon/amazon.go?format=TEXT",
			false,
		},
		{
			Module{
				Version: module.Version{
					Path:    "go.googlesource.com/oauth2/amazon",
					Version: "v0.16.0",
				},
				FilePath: "amazon.go",
			},
			"https://go.googlesource.com/oauth2/+/v0.16.0/amazon/amazon.go?format=TEXT",
			false,
		},
		{
			Module{
				Version: module.Version{
					Path:    "go.googlesource.com/oauth2/amazon",
					Version: "v0.16.0-0.20230710114240-c316eb95ae5b",
				},
				FilePath: "amazon.go",
			},
			"https://go.googlesource.com/oauth2/+/c316eb95ae5b/amazon/amazon.go?format=TEXT",
			false,
		},
	} {
		t.Run(tt.in.String(), func(t *testing.T) {
			actual, err := tt.in.GoogleSourceURL()
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
