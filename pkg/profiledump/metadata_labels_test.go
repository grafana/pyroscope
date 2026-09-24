package profiledump

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateLabelsBounds(t *testing.T) {
	require.NoError(t, ValidateLabels(nil))
	require.NoError(t, ValidateLabels(map[string]string{strings.Repeat("n", MaxLabelName): strings.Repeat("v", MaxLabelValue)}))
	labels := make(map[string]string, MaxLabels)
	for i := 0; i < MaxLabels; i++ {
		labels[fmt.Sprintf("label%d", i)] = "value"
	}
	require.NoError(t, ValidateLabels(labels))
	labels["extra"] = "value"
	require.ErrorContains(t, ValidateLabels(labels), "too many")
	for _, values := range []map[string]string{
		{strings.Repeat("n", MaxLabelName+1): "value"},
		{"name": strings.Repeat("v", MaxLabelValue+1)},
		{"name": "bad\nvalue"}, {"name": "\xff"},
	} {
		require.Error(t, ValidateLabels(values))
	}
	labels = make(map[string]string)
	// Eight bounded pairs total exactly 8192 bytes.
	for i := 0; i < 8; i++ {
		labels[fmt.Sprint(i)] = strings.Repeat("v", MaxLabelValue-1)
	}
	require.NoError(t, ValidateLabels(labels))
	labels["0"] += "v"
	require.ErrorContains(t, ValidateLabels(labels), "byte limit")
}
