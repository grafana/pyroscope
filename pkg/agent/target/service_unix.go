// +build !windows

package target

import (
	"errors"
)

func getPID(_ string) (int, error) {
	return 0, errors.New("not implemented")
}
