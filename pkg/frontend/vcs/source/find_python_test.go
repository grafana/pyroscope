package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isPythonStdlibPath(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		expectedPath    string
		expectedVersion string
		expectedOk      bool
	}{
		{
			name:            "UV Python install",
			path:            "/Users/user/.local/share/uv/python/cpython-3.12.12-macos-aarch64-none/lib/python3.12/difflib.py",
			expectedPath:    "difflib.py",
			expectedVersion: "3.12",
			expectedOk:      true,
		},
		{
			name:            "pyenv install",
			path:            "/home/user/.pyenv/versions/3.11.4/lib/python3.11/json/decoder.py",
			expectedPath:    "json/decoder.py",
			expectedVersion: "3.11",
			expectedOk:      true,
		},
		{
			name:            "System Python",
			path:            "/usr/lib/python3.10/collections/__init__.py",
			expectedPath:    "collections/__init__.py",
			expectedVersion: "3.10",
			expectedOk:      true,
		},
		{
			name:            "Python 2.7",
			path:            "/usr/lib/python2.7/os.py",
			expectedPath:    "os.py",
			expectedVersion: "2.7",
			expectedOk:      true,
		},
		{
			name:            "Two-digit minor version",
			path:            "/lib/python3.99/test.py",
			expectedPath:    "test.py",
			expectedVersion: "3.99",
			expectedOk:      true,
		},
		{
			name:            "Not stdlib - user project",
			path:            "/app/myproject/main.py",
			expectedPath:    "",
			expectedVersion: "",
			expectedOk:      false,
		},
		{
			name:            "Not stdlib - no python prefix",
			path:            "/usr/lib/site-packages/requests/api.py",
			expectedPath:    "",
			expectedVersion: "",
			expectedOk:      false,
		},
		{
			name:            "Empty path",
			path:            "",
			expectedPath:    "",
			expectedVersion: "",
			expectedOk:      false,
		},
		{
			name:            "Ends at python version - no trailing slash",
			path:            "/lib/python3.12",
			expectedPath:    "",
			expectedVersion: "",
			expectedOk:      false,
		},
		{
			name:            "Ends at python version - with trailing slash only",
			path:            "/lib/python3.12/",
			expectedPath:    "",
			expectedVersion: "",
			expectedOk:      false,
		},
		{
			name:            "Nested stdlib path",
			path:            "/opt/python/lib/python3.9/email/mime/text.py",
			expectedPath:    "email/mime/text.py",
			expectedVersion: "3.9",
			expectedOk:      true,
		},
		{
			name:            "Windows-style path with forward slashes",
			path:            "C:/Python312/Lib/python3.12/asyncio/base_events.py",
			expectedPath:    "asyncio/base_events.py",
			expectedVersion: "3.12",
			expectedOk:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, version, ok := isPythonStdlibPath(tt.path)
			assert.Equal(t, tt.expectedPath, path)
			assert.Equal(t, tt.expectedVersion, version)
			assert.Equal(t, tt.expectedOk, ok)
		})
	}
}

