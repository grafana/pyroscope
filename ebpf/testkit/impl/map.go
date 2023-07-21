package impl

import "github.com/cilium/ebpf"

type MapImpl struct {
	m *ebpf.Map
}

func (m MapImpl) Update(k, v any, flags ebpf.MapUpdateFlags) error {
	return m.m.Update(k, v, flags)
}
