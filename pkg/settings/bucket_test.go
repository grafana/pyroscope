package settings

import (
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"

	settingsv1 "github.com/grafana/pyroscope/api/gen/proto/go/settings/v1"
)

func TestMemoryBucket_Get(t *testing.T) {
	ctx := context.Background()
	tenantID := "[anonymous]"

	t.Run("get settings are sorted", func(t *testing.T) {
		mem, err := NewMemoryStore()
		assert.NoError(t, err)

		settings := []*settingsv1.Setting{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		}
		for _, s := range settings {
			_, err = mem.Set(ctx, tenantID, s)
			assert.NoError(t, err)
		}
		got, err := mem.Get(ctx, tenantID)
		assert.NoError(t, err)
		assert.Equal(t, settings, got)
		assert.True(t, sort.SliceIsSorted(got, func(i, j int) bool {
			return got[i].Name < got[j].Name
		}))
	})

	t.Run("don't get settings from another tenant", func(t *testing.T) {
		mem, err := NewMemoryStore()
		assert.NoError(t, err)

		otherTenantID := "other"

		tenant1Settings := []*settingsv1.Setting{
			{Name: "t1_key1", Value: "val1"},
			{Name: "t1_key2", Value: "val2"},
		}
		for _, s := range tenant1Settings {
			_, err = mem.Set(ctx, tenantID, s)
			assert.NoError(t, err)
		}

		tenant2Settings := []*settingsv1.Setting{
			{Name: "t2_key1", Value: "val1"},
			{Name: "t2_key2", Value: "val2"},
		}
		for _, s := range tenant2Settings {
			_, err = mem.Set(ctx, otherTenantID, s)
			assert.NoError(t, err)
		}

		got, err := mem.Get(ctx, otherTenantID)
		assert.NoError(t, err)
		assert.Equal(t, tenant2Settings, got)
	})
}

func TestMemoryBucket_Set(t *testing.T) {
	ctx := context.Background()
	tenantID := "[anonymous]"

	t.Run("set a new key", func(t *testing.T) {
		mem, err := NewMemoryStore()
		assert.NoError(t, err)

		setting := &settingsv1.Setting{
			Name:  "key1",
			Value: "val1",
		}
		got, err := mem.Set(ctx, tenantID, setting)
		assert.NoError(t, err)
		assert.Equal(t, setting, got)
	})

	t.Run("update a key", func(t *testing.T) {
		mem, err := NewMemoryStore()
		assert.NoError(t, err)

		setting := &settingsv1.Setting{
			Name:  "key1",
			Value: "val1",
		}
		got, err := mem.Set(ctx, tenantID, setting)
		assert.NoError(t, err)
		assert.Equal(t, setting, got)

		newSetting := &settingsv1.Setting{
			Name:  "key1",
			Value: "val2",
		}
		got, err = mem.Set(ctx, tenantID, newSetting)
		assert.NoError(t, err)
		assert.Equal(t, newSetting, got)
	})

	t.Run("don't update a key that's too old", func(t *testing.T) {
		mem, err := NewMemoryStore()
		assert.NoError(t, err)

		setting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1",
			ModifiedAt: 10,
		}
		got, err := mem.Set(ctx, tenantID, setting)
		assert.NoError(t, err)
		assert.Equal(t, setting, got)

		newSetting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val2",
			ModifiedAt: 5,
		}
		_, err = mem.Set(ctx, tenantID, newSetting)
		assert.EqualError(t, err, "failed to update key1: newer update already written")
	})
}
