package objstore

import (
	"errors"

	"github.com/thanos-io/objstore"
)

func IsNotExist(b objstore.BucketReader, err error) bool {
	// objstore relies on the Causer interface
	// and does not understand wrapped errors.
	return b.IsObjNotFoundErr(UnwrapErr(err))
}

func UnwrapErr(err error) error {
	for {
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			return err
		}
		err = unwrapped
	}
}
