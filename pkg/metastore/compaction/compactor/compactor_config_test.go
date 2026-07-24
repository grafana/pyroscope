package compactor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfig_maxBytes(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		level    uint32
		expected uint64
	}{
		{"level 0 is always exempt", Config{Levels: []LevelConfig{{}, {}}, MaxJobBytes: 500}, 0, 0},
		{"level 1 uses configured MaxJobBytes", Config{Levels: []LevelConfig{{}, {}}, MaxJobBytes: 500}, 1, 500},
		{"MaxJobBytes 0 disables the limit everywhere", Config{Levels: []LevelConfig{{}, {}}, MaxJobBytes: 0}, 1, 0},
		{"level beyond configured Levels returns 0", Config{Levels: []LevelConfig{{}}, MaxJobBytes: 500}, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, tt.config.maxBytes(tt.level))
		})
	}
}
