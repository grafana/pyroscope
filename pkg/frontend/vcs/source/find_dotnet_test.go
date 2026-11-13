package source

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/pyroscope/v2/pkg/frontend/vcs/config"
)

func Test_isDotNetExtension(t *testing.T) {
	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{name: "C#", ext: ".cs", expected: true},
		{name: "F#", ext: ".fs", expected: true},
		{name: "Visual Basic", ext: ".vb", expected: true},
		{name: "assembly name suffix", ext: ".Web", expected: false},
		{name: "DLL", ext: ".dll", expected: false},
		{name: "Empty extension", ext: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isDotNetExtension(tt.ext))
		})
	}
}

func Test_findDotNetFile_noSourcePath(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "assembly name", path: "MyApp.Web"},
		{name: "framework assembly", path: "System.Private.CoreLib"},
		{name: "empty", path: ""},
	}

	mapping := &config.MappingConfig{
		FunctionName: []config.Match{{Prefix: "MyApp"}},
		Language:     string(config.LanguageDotNet),
		Source:       config.Source{Local: &config.LocalMappingConfig{Path: "src"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockVCSClient()
			ff := FileFinder{
				client: mockClient,
				file:   config.FileSpec{FunctionName: "MyApp.Web.Program.Main", Path: tt.path},
				logger: log.NewNopLogger(),
			}

			resp, err := ff.findDotNetFile(context.Background(), mapping)
			require.Nil(t, resp)
			require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
			assert.Empty(t, mockClient.searchedSequence, "no source should be fetched")
		})
	}
}
