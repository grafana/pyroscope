// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/util/yolo.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package util

import "unsafe"

func YoloBuf(s string) []byte {
	return *((*[]byte)(unsafe.Pointer(&s)))
}
