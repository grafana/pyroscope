package pyroscope

import (
	"errors"
	"fmt"
)

type StorageLayer string

const (
	V1       StorageLayer = "v1"
	V1V2Dual StorageLayer = "v1-v2-dual"
	V2       StorageLayer = "v2"
)

var ErrInvalidStorageLayer = errors.New("invalid storage layer")

var storageLayers = []StorageLayer{
	V1,
	V1V2Dual,
	V2,
}

const validStorageLayerOptionsString = "valid options: v1, v1-v2-dual, v2"

func (m *StorageLayer) Set(text string) error {
	x := StorageLayer(text)
	for _, name := range storageLayers {
		if x == name {
			*m = x
			return nil
		}
	}
	return fmt.Errorf("%w: %s; %s", ErrInvalidStorageLayer, x, validStorageLayerOptionsString)
}

func (m *StorageLayer) String() string { return string(*m) }
