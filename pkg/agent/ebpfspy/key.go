//go:build ebpfspy
// +build ebpfspy

// Package ebpfspy provides integration with Linux eBPF. It is a rough copy of profile.py from BCC tools:
//   https://github.com/iovisor/bcc/blob/master/tools/profile.py
package ebpfspy

import (
	"bytes"
)

type KeyBytes struct {
	pid             []byte
	kernel_ip       []byte
	kernel_ret_ip   []byte
	user_stack_id   []byte
	kernel_stack_id []byte
	name            []byte
}

func UnpackKeyBytes(b []byte) *KeyBytes {
	g := KeyBytes{}
	g.pid = b[:4]                // 4
	g.kernel_ip = b[8:16]        // 8
	g.kernel_ret_ip = b[16:24]   // 8
	g.user_stack_id = b[24:28]   // 4
	g.kernel_stack_id = b[28:32] // 4
	g.name = b[32:]              // 8
	i := bytes.Index(g.name, []byte{0})
	if i >= 0 {
		g.name = g.name[:i]
	}
	return &g
}
