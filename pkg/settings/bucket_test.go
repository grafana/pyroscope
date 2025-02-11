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
	const tenantID = "[anonymous]"

	t.Run("get settings are sorted", func(t *testing.T) {
		mem := newMemoryStore()

		settings := []*settingsv1.Setting{
			{Name: "key1", Value: "val1"},
			{Name: "key2", Value: "val2"},
		}
		for _, s := range settings {
			_, err := mem.Set(ctx, tenantID, s)
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
		var err error
		mem := newMemoryStore()

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
	const tenantID = "[anonymous]"

	t.Run("set a new key", func(t *testing.T) {
		mem := newMemoryStore()

		setting := &settingsv1.Setting{
			Name:  "key1",
			Value: "val1",
		}
		got, err := mem.Set(ctx, tenantID, setting)
		assert.NoError(t, err)
		assert.Equal(t, setting, got)
	})

	t.Run("update a key", func(t *testing.T) {
		mem := newMemoryStore()

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
		mem := newMemoryStore()

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

func TestMemoryBucket_Delete(t *testing.T) {
	ctx := context.Background()
	const tenantID = "[anonymous]"

	t.Run("delete a key", func(t *testing.T) {
		mem := newMemoryStore()

		setting := &settingsv1.Setting{
			Name:  "key1",
			Value: "val1",
		}
		_, err := mem.Set(ctx, tenantID, setting)
		assert.NoError(t, err)

		err = mem.Delete(ctx, tenantID, setting.Name, setting.ModifiedAt)
		assert.NoError(t, err)

		got, err := mem.Get(ctx, tenantID)
		assert.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("delete a non-existent key", func(t *testing.T) {
		mem := newMemoryStore()

		err := mem.Delete(ctx, tenantID, "key1", 0)
		assert.NoError(t, err)
	})

	t.Run("don't delete a key that's too old", func(t *testing.T) {
		mem := newMemoryStore()

		setting := &settingsv1.Setting{
			Name:       "key1",
			Value:      "val1",
			ModifiedAt: 10,
		}
		_, err := mem.Set(ctx, tenantID, setting)
		assert.NoError(t, err)

		err = mem.Delete(ctx, tenantID, setting.Name, 5)
		assert.EqualError(t, err, "failed to delete key1: newer update already written")
	})
}
