package validation

import (
	"bytes"
	"flag"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const symbolizerOverrideConfig = `
overrides:
  symbolizer-disabled:
    symbolizer:
      enabled: false
  symbolizer-enabled:
    symbolizer:
      enabled: true
  mixed-config:
    symbolizer:
      enabled: true
    ingestion_rate_mb: 100
`

func Test_SymbolizerEnabled(t *testing.T) {
	rc, err := LoadRuntimeConfig(bytes.NewReader([]byte(symbolizerOverrideConfig)))
	require.NoError(t, err)

	var defaultCfg Limits
	fs := flag.NewFlagSet("test", flag.PanicOnError)
	defaultCfg.RegisterFlags(fs)
	defaultCfg.Symbolizer.RegisterFlags(fs)

	err = fs.Parse([]string{})
	require.NoError(t, err)

	o, err := NewOverrides(defaultCfg, &wrappedRuntimeConfig{rc})
	require.NoError(t, err)

	assert.False(t, o.SymbolizerEnabled("empty-overrides"))
	assert.False(t, o.SymbolizerEnabled("symbolizer-disabled"))
	assert.True(t, o.SymbolizerEnabled("symbolizer-enabled"))
	assert.True(t, o.SymbolizerEnabled("mixed-config"))
}

func Test_SymbolizerMockOverrides(t *testing.T) {
	overrides := MockOverrides(func(defaults *Limits, tenantLimits map[string]*Limits) {
		defaults.Symbolizer.Enabled = false

		l := MockDefaultLimits()
		l.Symbolizer.Enabled = true
		tenantLimits["enabled-tenant"] = l
	})

	assert.False(t, overrides.SymbolizerEnabled("default-tenant"))
	assert.True(t, overrides.SymbolizerEnabled("enabled-tenant"))
}
