package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_filename_extraction(t *testing.T) {
	assert.Equal(t, "app", extractMappingFilename(`app`))
	assert.Equal(t, "app", extractMappingFilename(`./app`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/app`))
	assert.Equal(t, "app", extractMappingFilename(`../../../app`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/app\`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/app\\`))
	assert.Equal(t, "my awesome app", extractMappingFilename(`/usr/bin/my awesome app`))
	assert.Equal(t, "app", extractMappingFilename(`/usr/bin/my\ awesome\ app`))

	assert.Equal(t, "app.exe", extractMappingFilename(`C:\\app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`C:\\./app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`./app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`./../app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`C:\\build\app.exe`))
	assert.Equal(t, "My App.exe", extractMappingFilename(`C:\\build\My App.exe`))
	assert.Equal(t, "Not My App.exe", extractMappingFilename(`C:\\build\Not My App.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`\\app.exe`))
	assert.Equal(t, "app.exe", extractMappingFilename(`\\build\app.exe`))

	assert.Equal(t, "bin", extractMappingFilename(`/usr/bin/`))
	assert.Equal(t, "build", extractMappingFilename(`\\build\`))

	assert.Equal(t, "", extractMappingFilename(""))
	assert.Equal(t, "", extractMappingFilename(`[vdso]`))
	assert.Equal(t, "", extractMappingFilename(`[vsyscall]`))
	assert.Equal(t, "not a path actually", extractMappingFilename(`not a path actually`))
}
