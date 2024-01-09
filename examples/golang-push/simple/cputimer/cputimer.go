package cputimer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
)

type Timer struct {
	pprofLabels pprof.LabelSet
}

func (c *Timer) Start(ctx context.Context) (context.Context, func()) {
	stop := func() {
		pprof.SetGoroutineLabels(ctx)
	}
	ctx = pprof.WithLabels(ctx, c.pprofLabels)
	pprof.SetGoroutineLabels(ctx)
	return ctx, stop
}

func (c *Timer) Do(ctx context.Context, fn func(context.Context)) {
	defer pprof.SetGoroutineLabels(ctx)
	ctx = pprof.WithLabels(ctx, c.pprofLabels)
	pprof.SetGoroutineLabels(ctx)
	fn(ctx)
}

type Opts prometheus.Opts

type TimerVec struct {
	fqName string
	help   string

	labels    labels.Labels
	varLabels []string
	err       error

	m    sync.RWMutex
	dims map[uint64]*Timer
}

func NewCPUTimerVec(opts Opts, labelNames []string) *TimerVec {
	v := &TimerVec{
		fqName:    prometheus.BuildFQName(opts.Namespace, opts.Subsystem, opts.Name),
		varLabels: labelNames,
		labels:    make(labels.Labels, 0, 1+len(labelNames)+len(opts.ConstLabels)),
		dims:      make(map[uint64]*Timer),
	}
	if !model.IsValidMetricName(model.LabelValue(v.fqName)) {
		v.err = fmt.Errorf("invalid metric name %q", v.fqName)
		return v
	}
	for _, ln := range labelNames {
		if err := checkLabelName(ln); err != nil {
			v.err = err
			break
		}
		v.labels = append(v.labels, labels.Label{Name: ln})
	}
	for ln, lv := range opts.ConstLabels {
		if err := checkLabelName(ln); err != nil {
			v.err = err
			break
		}
		if err := checkLabelValue(lv); err != nil {
			v.err = err
			break
		}
		v.labels = append(v.labels, labels.Label{
			Name:  ln,
			Value: lv,
		})
	}
	v.labels = append(v.labels, labels.Label{
		Name:  model.MetricNameLabel,
		Value: v.fqName,
	})
	return v
}

func (v *TimerVec) WithLabelValues(lvs ...string) *Timer {
	c, err := v.GetTimerWithLabelValues(lvs...)
	if err != nil {
		panic(err)
	}
	return c
}

func (v *TimerVec) GetTimerWithLabelValues(lvs ...string) (*Timer, error) {
	return v.getOrCreateTimer(lvs...)
}

func (v *TimerVec) getOrCreateTimer(lvs ...string) (*Timer, error) {
	if err := v.validateLabelValues(lvs...); err != nil {
		return nil, err
	}
	hc := v.hashLabelValues(lvs...)
	v.m.RLock()
	c, ok := v.dims[hc]
	v.m.RUnlock()
	if ok {
		return c, nil
	}
	v.m.Lock()
	defer v.m.Unlock()
	c, ok = v.dims[hc]
	if ok {
		return c, nil
	}
	c = &Timer{v.createPprofLabels(lvs)}
	v.dims[hc] = c
	return c, nil
}

func (v *TimerVec) validateLabelValues(lvs ...string) error {
	if v.err != nil {
		return v.err
	}
	if len(lvs) != len(v.varLabels) {
		return makeInconsistentCardinalityError(v.fqName, v.varLabels, lvs)
	}
	for _, val := range lvs {
		if err := checkLabelValue(val); err != nil {
			return err
		}
	}
	return nil
}

func (v *TimerVec) hashLabelValues(lvs ...string) uint64 {
	h := hashNew()
	for _, lv := range lvs {
		h = hashAdd(h, lv)
		h = hashAddByte(h, model.SeparatorByte)
	}
	return h
}

const pprofMetricLabelPrefix = "__m_"

func (v *TimerVec) createPprofLabels(lvs []string) pprof.LabelSet {
	k := v.labels.Copy()
	if len(lvs) > 0 {
		for i, lv := range lvs {
			k[i].Value = lv
		}
	}
	sort.Sort(k)
	var b bytes.Buffer
	b.WriteString(pprofMetricLabelPrefix)
	b.WriteByte('{')
	for i, l := range k {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		b.WriteString(l.Name)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(l.Value))
	}
	b.WriteByte('}')
	return pprof.Labels(b.String(), "")
}

var errInconsistentCardinality = errors.New("inconsistent label cardinality")

func makeInconsistentCardinalityError(fqName string, labels, labelValues []string) error {
	return fmt.Errorf(
		"%w: %q has %d variable labels named %q but %d values %q were provided",
		errInconsistentCardinality, fqName,
		len(labels), labels,
		len(labelValues), labelValues,
	)
}

func checkLabelName(n string) error {
	if !model.LabelName(n).IsValid() || strings.HasPrefix(n, model.ReservedLabelPrefix) {
		return fmt.Errorf("%q is not a valid label name", n)
	}
	return nil
}

func checkLabelValue(v string) error {
	if !model.LabelValue(v).IsValid() {
		return fmt.Errorf("%q is not a valid label value", v)
	}
	return nil
}
