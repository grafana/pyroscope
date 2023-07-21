package iface

import "github.com/cilium/ebpf"

type UserMap interface {
	Update(k, v any, flags ebpf.MapUpdateFlags) error
}
