package storage

import "github.com/petethepig/pyroscope/pkg/storage/dict"

type persistableDict struct {
	dict.Dict
	path string
}

func newPersistableDict() {

}
