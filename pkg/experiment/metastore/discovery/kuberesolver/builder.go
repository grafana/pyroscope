package kuberesolver

import (
	"context"
	"fmt"
	"github.com/go-kit/log"
	"io"
	"sync"
	"time"
)

const (
	defaultFreq = time.Minute * 30
)

type TargetInfo struct {
	ServiceName      string
	ServiceNamespace string
}

func Build(l log.Logger, k8sClient K8sClient, upd ResolveUpdates, target TargetInfo) (*KResolver, error) {
	if k8sClient == nil {
		return nil, fmt.Errorf("k8sClient is nil")
	}
	ti := target
	if ti.ServiceNamespace == "" {
		ti.ServiceNamespace = getCurrentNamespaceOrDefault()
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &KResolver{
		target:    ti,
		upd:       upd,
		l:         l,
		ctx:       ctx,
		cancel:    cancel,
		k8sClient: k8sClient,
		t:         time.NewTimer(defaultFreq),
		freq:      defaultFreq,
	}
	go until(func() {
		r.wg.Add(1)
		err := r.watch()
		if err != nil && err != io.EOF {
			l.Log("msg", "watching ended with error, will reconnect again", "err", err)
		}
	}, time.Second, time.Second*30, ctx.Done())
	return r, nil
}

type ResolveUpdates interface {
	Resolved(e Endpoints)
}

type ResolveUpdatesFunc func(e Endpoints)

func (f ResolveUpdatesFunc) Resolved(e Endpoints) {
	f(e)
}

type KResolver struct {
	target    TargetInfo
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient K8sClient
	// wg is used to enforce Close() to return after the watcher() goroutine has finished.
	wg   sync.WaitGroup
	t    *time.Timer
	freq time.Duration
	upd  ResolveUpdates
	l    log.Logger
}

// Close closes the resolver.
func (k *KResolver) Close() {
	k.cancel()
	k.wg.Wait()
}

func (k *KResolver) handle(e Endpoints) {
	k.upd.Resolved(e)
}

func (k *KResolver) resolve() {
	e, err := getEndpoints(k.k8sClient, k.target.ServiceNamespace, k.target.ServiceName)
	if err == nil {
		k.handle(e)
	} else {
		k.l.Log("msg", "lookup endpoints failed", "err", err)
	}
	// Next lookup should happen after an interval defined by k.freq.
	k.t.Reset(k.freq)
}

func (k *KResolver) watch() error {
	defer k.wg.Done()
	// watch endpoints lists existing endpoints at start
	sw, err := watchEndpoints(k.ctx, k.k8sClient, k.target.ServiceNamespace, k.target.ServiceName)
	if err != nil {
		return err
	}
	for {
		select {
		case <-k.ctx.Done():
			return nil
		case <-k.t.C:
			k.resolve()
		case up, hasMore := <-sw.ResultChan():
			if hasMore {
				k.handle(up.Object)
			} else {
				return nil
			}
		}
	}
}
