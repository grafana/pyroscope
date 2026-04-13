package pyroscope

import (
	"errors"
	"fmt"
)

type StorageMode string

const (
	Legacy  StorageMode = "legacy"
	Dual    StorageMode = "dual"
	Default StorageMode = "default"
)

var ErrInvalidStorageMode = errors.New("invalid storage mode")

var storageModes = []StorageMode{
	Legacy,
	Dual,
	Default,
}

const validStorageModeOptionsString = "valid options: legacy, dual, default"

func (m *StorageMode) Set(text string) error {
	x := StorageMode(text)
	for _, name := range storageModes {
		if x == name {
			*m = x
			return nil
		}
	}
	return fmt.Errorf("%w: %s; %s", ErrInvalidStorageMode, x, validStorageModeOptionsString)
}

func (m *StorageMode) String() string { return string(*m) }
