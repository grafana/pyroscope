package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isJavaScriptExtension(t *testing.T) {
	tests := []struct {
		name     string
		ext      string
		expected bool
	}{
		{
			name:     "JavaScript",
			ext:      ".js",
			expected: true,
		},
		{
			name:     "TypeScript",
			ext:      ".ts",
			expected: true,
		},
		{
			name:     "ES Module",
			ext:      ".mjs",
			expected: true,
		},
		{
			name:     "CommonJS",
			ext:      ".cjs",
			expected: true,
		},
		{
			name:     "JSX",
			ext:      ".jsx",
			expected: true,
		},
		{
			name:     "TSX",
			ext:      ".tsx",
			expected: true,
		},
		{
			name:     "Go file",
			ext:      ".go",
			expected: false,
		},
		{
			name:     "Python file",
			ext:      ".py",
			expected: false,
		},
		{
			name:     "Empty extension",
			ext:      "",
			expected: false,
		},
		{
			name:     "JSON file",
			ext:      ".json",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isJavaScriptExtension(tt.ext)
			assert.Equal(t, tt.expected, result)
		})
	}
}
