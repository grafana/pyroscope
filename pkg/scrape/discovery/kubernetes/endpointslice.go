// Copyright 2020 The Prometheus Authors
// Copyright 2021 The Pyroscope Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	disv1beta1 "k8s.io/api/discovery/v1beta1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/model"
)

var (
	epslAddCount    = eventCount.WithLabelValues("endpointslice", "add")
	epslUpdateCount = eventCount.WithLabelValues("endpointslice", "update")
	epslDeleteCount = eventCount.WithLabelValues("endpointslice", "delete")
)

// revive:disable:cognitive-complexity preserve original implementation

// EndpointSlice discovers new endpoint targets.
type EndpointSlice struct {
	logger logrus.FieldLogger

	endpointSliceInf cache.SharedInformer
	serviceInf       cache.SharedInformer
	podInf           cache.SharedInformer

	podStore           cache.Store
	endpointSliceStore cache.Store
	serviceStore       cache.Store

	queue *workqueue.Type
}

// NewEndpointSlice returns a new endpointslice discovery.
func NewEndpointSlice(l logrus.FieldLogger, svc, eps, pod cache.SharedInformer) *EndpointSlice {
	e := &EndpointSlice{
		logger:             l,
		endpointSliceInf:   eps,
		endpointSliceStore: eps.GetStore(),
		serviceInf:         svc,
		serviceStore:       svc.GetStore(),
		podInf:             pod,
		podStore:           pod.GetStore(),
		queue:              workqueue.NewNamed("endpointSlice"),
	}

	e.endpointSliceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			epslAddCount.Inc()
			e.enqueue(o)
		},
		UpdateFunc: func(_, o interface{}) {
			epslUpdateCount.Inc()
			e.enqueue(o)
		},
		DeleteFunc: func(o interface{}) {
			epslDeleteCount.Inc()
			e.enqueue(o)
		},
	})

	serviceUpdate := func(o interface{}) {
		svc, err := convertToService(o)
		if err != nil {
			e.logger.WithError(err).Error("converting to Service object")
			return
		}

		// TODO(brancz): use cache.Indexer to index endpoints by
		// disv1beta1.LabelServiceName so this operation doesn't have to
		// iterate over all endpoint objects.
		for _, obj := range e.endpointSliceStore.List() {
			ep := obj.(*disv1beta1.EndpointSlice)
			if lv, exists := ep.Labels[disv1beta1.LabelServiceName]; exists && lv == svc.Name {
				e.enqueue(ep)
			}
		}
	}
	e.serviceInf.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(o interface{}) {
			svcAddCount.Inc()
			serviceUpdate(o)
		},
		UpdateFunc: func(_, o interface{}) {
			svcUpdateCount.Inc()
			serviceUpdate(o)
		},
		DeleteFunc: func(o interface{}) {
			svcDeleteCount.Inc()
			serviceUpdate(o)
		},
	})

	return e
}

func (e *EndpointSlice) enqueue(obj interface{}) {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		return
	}

	e.queue.Add(key)
}

// Run implements the Discoverer interface.
func (e *EndpointSlice) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	defer e.queue.ShutDown()

	if !cache.WaitForCacheSync(ctx.Done(), e.endpointSliceInf.HasSynced, e.serviceInf.HasSynced, e.podInf.HasSynced) {
		if ctx.Err() != context.Canceled {
			e.logger.Error("endpointslice informer unable to sync cache")
		}
		return
	}

	go func() {
		for {
			if !e.process(ctx, ch) {
				return
			}
		}
	}()

	// Block until the target provider is explicitly canceled.
	<-ctx.Done()
}

func (e *EndpointSlice) process(ctx context.Context, ch chan<- []*targetgroup.Group) bool {
	keyObj, quit := e.queue.Get()
	if quit {
		return false
	}
	defer e.queue.Done(keyObj)
	key := keyObj.(string)
	logger := e.logger.WithField("key", key)

	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		logger.WithError(err).Error("splitting key")
		return true
	}

	o, exists, err := e.endpointSliceStore.GetByKey(key)
	if err != nil {
		logger.WithError(err).Error("getting object from store")
		return true
	}
	if !exists {
		send(ctx, ch, &targetgroup.Group{Source: endpointSliceSourceFromNamespaceAndName(namespace, name)})
		return true
	}
	eps, err := convertToEndpointSlice(o)
	if err != nil {
		logger.WithError(err).Error("converting to EndpointSlice object")
		return true
	}
	send(ctx, ch, e.buildEndpointSlice(eps))
	return true
}

func convertToEndpointSlice(o interface{}) (*disv1beta1.EndpointSlice, error) {
	endpoints, ok := o.(*disv1beta1.EndpointSlice)
	if ok {
		return endpoints, nil
	}

	return nil, fmt.Errorf("received unexpected object: %v", o)
}

func endpointSliceSource(ep *disv1beta1.EndpointSlice) string {
	return endpointSliceSourceFromNamespaceAndName(ep.Namespace, ep.Name)
}

func endpointSliceSourceFromNamespaceAndName(namespace, name string) string {
	return "endpointslice/" + namespace + "/" + name
}

const (
	endpointSliceNameLabel                          = metaLabelPrefix + "endpointslice_name"
	endpointSliceAddressTypeLabel                   = metaLabelPrefix + "endpointslice_address_type"
	endpointSlicePortNameLabel                      = metaLabelPrefix + "endpointslice_port_name"
	endpointSlicePortProtocolLabel                  = metaLabelPrefix + "endpointslice_port_protocol"
	endpointSlicePortLabel                          = metaLabelPrefix + "endpointslice_port"
	endpointSlicePortAppProtocol                    = metaLabelPrefix + "endpointslice_port_app_protocol"
	endpointSliceEndpointConditionsReadyLabel       = metaLabelPrefix + "endpointslice_endpoint_conditions_ready"
	endpointSliceEndpointHostnameLabel              = metaLabelPrefix + "endpointslice_endpoint_hostname"
	endpointSliceAddressTargetKindLabel             = metaLabelPrefix + "endpointslice_address_target_kind"
	endpointSliceAddressTargetNameLabel             = metaLabelPrefix + "endpointslice_address_target_name"
	endpointSliceEndpointTopologyLabelPrefix        = metaLabelPrefix + "endpointslice_endpoint_topology_"
	endpointSliceEndpointTopologyLabelPresentPrefix = metaLabelPrefix + "endpointslice_endpoint_topology_present_"
)

func (e *EndpointSlice) buildEndpointSlice(eps *disv1beta1.EndpointSlice) *targetgroup.Group {
	tg := &targetgroup.Group{
		Source: endpointSliceSource(eps),
	}
	tg.Labels = model.LabelSet{
		namespaceLabel:                lv(eps.Namespace),
		endpointSliceNameLabel:        lv(eps.Name),
		endpointSliceAddressTypeLabel: lv(string(eps.AddressType)),
	}
	e.addServiceLabels(eps, tg)

	type podEntry struct {
		pod          *apiv1.Pod
		servicePorts []disv1beta1.EndpointPort
	}
	seenPods := map[string]*podEntry{}

	add := func(addr string, ep disv1beta1.Endpoint, port disv1beta1.EndpointPort) {
		a := addr
		if port.Port != nil {
			a = net.JoinHostPort(addr, strconv.FormatUint(uint64(*port.Port), 10))
		}

		target := model.LabelSet{
			model.AddressLabel: lv(a),
		}

		if port.Name != nil {
			target[endpointSlicePortNameLabel] = lv(*port.Name)
		}

		if port.Protocol != nil {
			target[endpointSlicePortProtocolLabel] = lv(string(*port.Protocol))
		}

		if port.Port != nil {
			target[endpointSlicePortLabel] = lv(strconv.FormatUint(uint64(*port.Port), 10))
		}

		if port.AppProtocol != nil {
			target[endpointSlicePortAppProtocol] = lv(*port.AppProtocol)
		}

		if ep.Conditions.Ready != nil {
			target[endpointSliceEndpointConditionsReadyLabel] = lv(strconv.FormatBool(*ep.Conditions.Ready))
		}

		if ep.Hostname != nil {
			target[endpointSliceEndpointHostnameLabel] = lv(*ep.Hostname)
		}

		if ep.TargetRef != nil {
			target[model.LabelName(endpointSliceAddressTargetKindLabel)] = lv(ep.TargetRef.Kind)
			target[model.LabelName(endpointSliceAddressTargetNameLabel)] = lv(ep.TargetRef.Name)
		}

		for k, v := range ep.Topology {
			ln := sanitizeLabelName(k)
			target[model.LabelName(endpointSliceEndpointTopologyLabelPrefix+ln)] = lv(v)
			target[model.LabelName(endpointSliceEndpointTopologyLabelPresentPrefix+ln)] = presentValue
		}

		pod := e.resolvePodRef(ep.TargetRef)
		if pod == nil {
			// This target is not a Pod, so don't continue with Pod specific logic.
			tg.Targets = append(tg.Targets, target)
			return
		}
		s := pod.Namespace + "/" + pod.Name

		sp, ok := seenPods[s]
		if !ok {
			sp = &podEntry{pod: pod}
			seenPods[s] = sp
		}

		// Attach standard pod labels.
		target = target.Merge(podLabels(pod))

		// Attach potential container port labels matching the endpoint port.
		for _, c := range pod.Spec.Containers {
			for _, cport := range c.Ports {
				if port.Port == nil {
					continue
				}
				if *port.Port == cport.ContainerPort {
					ports := strconv.FormatUint(uint64(*port.Port), 10)

					target[podContainerNameLabel] = lv(c.Name)
					target[podContainerPortNameLabel] = lv(cport.Name)
					target[podContainerPortNumberLabel] = lv(ports)
					target[podContainerPortProtocolLabel] = lv(string(cport.Protocol))
					break
				}
			}
		}

		// Add service port so we know that we have already generated a target
		// for it.
		sp.servicePorts = append(sp.servicePorts, port)
		tg.Targets = append(tg.Targets, target)
	}

	for _, ep := range eps.Endpoints {
		for _, port := range eps.Ports {
			for _, addr := range ep.Addresses {
				add(addr, ep, port)
			}
		}
	}

	// For all seen pods, check all container ports. If they were not covered
	// by one of the service endpoints, generate targets for them.
	for _, pe := range seenPods {
		for _, c := range pe.pod.Spec.Containers {
			for _, cport := range c.Ports {
				hasSeenPort := func() bool {
					for _, eport := range pe.servicePorts {
						if eport.Port == nil {
							continue
						}
						if cport.ContainerPort == *eport.Port {
							return true
						}
					}
					return false
				}
				if hasSeenPort() {
					continue
				}

				a := net.JoinHostPort(pe.pod.Status.PodIP, strconv.FormatUint(uint64(cport.ContainerPort), 10))
				ports := strconv.FormatUint(uint64(cport.ContainerPort), 10)

				target := model.LabelSet{
					model.AddressLabel:            lv(a),
					podContainerNameLabel:         lv(c.Name),
					podContainerPortNameLabel:     lv(cport.Name),
					podContainerPortNumberLabel:   lv(ports),
					podContainerPortProtocolLabel: lv(string(cport.Protocol)),
				}
				tg.Targets = append(tg.Targets, target.Merge(podLabels(pe.pod)))
			}
		}
	}

	return tg
}

func (e *EndpointSlice) resolvePodRef(ref *apiv1.ObjectReference) *apiv1.Pod {
	if ref == nil || ref.Kind != "Pod" {
		return nil
	}
	p := &apiv1.Pod{}
	p.Namespace = ref.Namespace
	p.Name = ref.Name

	obj, exists, err := e.podStore.Get(p)
	if err != nil {
		e.logger.WithError(err).Error("resolving pod ref")
		return nil
	}
	if !exists {
		return nil
	}
	return obj.(*apiv1.Pod)
}

func (e *EndpointSlice) addServiceLabels(eps *disv1beta1.EndpointSlice, tg *targetgroup.Group) {
	var (
		svc   = &apiv1.Service{}
		found bool
	)
	svc.Namespace = eps.Namespace

	// Every EndpointSlice object has the Service they belong to in the
	// kubernetes.io/service-name label.
	svc.Name, found = eps.Labels[disv1beta1.LabelServiceName]
	if !found {
		return
	}

	obj, exists, err := e.serviceStore.Get(svc)
	if err != nil {
		e.logger.WithError(err).Error("retrieving service")
		return
	}
	if !exists {
		return
	}
	svc = obj.(*apiv1.Service)

	tg.Labels = tg.Labels.Merge(serviceLabels(svc))
}
