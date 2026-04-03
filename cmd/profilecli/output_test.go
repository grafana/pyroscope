package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	typesv1 "github.com/grafana/pyroscope/api/gen/proto/go/types/v1"
)

// TestOutputSeriesTable_LabelNamesPreserved verifies that label names are
// rendered exactly as received: no uppercasing, no character stripping.
func TestOutputSeriesTable_LabelNamesPreserved(t *testing.T) {
	t.Parallel()

	series := []*typesv1.Labels{
		{Labels: []*typesv1.LabelPair{
			{Name: "service_name", Value: "frontend"},
			{Name: "http.method", Value: "GET"},
			{Name: "camelCase", Value: "val1"},
		}},
		{Labels: []*typesv1.LabelPair{
			{Name: "service_name", Value: "backend"},
			{Name: "http.method", Value: "POST"},
			{Name: "camelCase", Value: "val2"},
		}},
	}

	var buf bytes.Buffer
	ctx := withOutput(context.Background(), &buf)

	err := outputSeriesTable(ctx, series)
	require.NoError(t, err)

	out := buf.String()

	// Header names must appear as-is, not uppercased or modified.
	assert.Contains(t, out, "service_name", "underscore label name must not be transformed")
	assert.Contains(t, out, "http.method", "dot in label name must not be stripped")
	assert.Contains(t, out, "camelCase", "mixed-case label name must not be uppercased")

	// Values must appear as-is.
	assert.Contains(t, out, "frontend")
	assert.Contains(t, out, "backend")
	assert.Contains(t, out, "GET")
	assert.Contains(t, out, "POST")
}

// TestOutputSeriesTable_Empty verifies that an empty result produces no output.
func TestOutputSeriesTable_Empty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	ctx := withOutput(context.Background(), &buf)

	err := outputSeriesTable(ctx, nil)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
