// Copyright 2016 The Prometheus Authors
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
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"
	disv1beta1 "k8s.io/api/discovery/v1beta1"
	networkv1 "k8s.io/api/networking/v1"
	"k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilversion "k8s.io/apimachinery/pkg/util/version"
	"k8s.io/apimachinery/pkg/watch"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/pyroscope-io/pyroscope/pkg/build"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/config"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery"
	"github.com/pyroscope-io/pyroscope/pkg/scrape/discovery/targetgroup"
)

// revive:disable:max-public-structs complex domain
// revive:disable:cognitive-complexity preserve original implementation

const (
	// kubernetesMetaLabelPrefix is the meta prefix used for all meta labels.
	// in this discovery.
	metaLabelPrefix  = model.MetaLabelPrefix + "kubernetes_"
	namespaceLabel   = metaLabelPrefix + "namespace"
	metricsNamespace = "prometheus_sd_kubernetes"
	presentValue     = model.LabelValue("true")
)

var (
	// Http header
	userAgent = fmt.Sprintf("Pyroscope/%s", build.Version)
	// Custom events metric
	eventCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricsNamespace,
			Name:      "events_total",
			Help:      "The number of Kubernetes events handled.",
		},
		[]string{"role", "event"},
	)
	// DefaultSDConfig is the default Kubernetes SD configuration
	DefaultSDConfig = SDConfig{
		HTTPClientConfig: config.DefaultHTTPClientConfig,
	}
)

func init() {
	discovery.RegisterConfig(&SDConfig{})
	prometheus.MustRegister(eventCount)
	// Initialize metric vectors.
	for _, role := range []string{"endpointslice", "endpoints", "node", "pod", "service", "ingress"} {
		for _, evt := range []string{"add", "delete", "update"} {
			eventCount.WithLabelValues(role, evt)
		}
	}
	(&clientGoRequestMetricAdapter{}).Register(prometheus.DefaultRegisterer)
	(&clientGoWorkqueueMetricsProvider{}).Register(prometheus.DefaultRegisterer)
}

// Role is role of the service in Kubernetes.
type Role string

// The valid options for Role.
const (
	RoleNode          Role = "node"
	RolePod           Role = "pod"
	RoleService       Role = "service"
	RoleEndpoint      Role = "endpoints"
	RoleEndpointSlice Role = "endpointslice"
	RoleIngress       Role = "ingress"
)

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Role) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if err := unmarshal((*string)(c)); err != nil {
		return err
	}
	switch *c {
	case RoleNode, RolePod, RoleService, RoleEndpoint, RoleEndpointSlice, RoleIngress:
		return nil
	default:
		return fmt.Errorf("unknown Kubernetes SD role %q", *c)
	}
}

// SDConfig is the configuration for Kubernetes service discovery.
type SDConfig struct {
	APIServer          config.URL              `yaml:"api_server,omitempty"`
	Role               Role                    `yaml:"role"`
	KubeConfig         string                  `yaml:"kubeconfig_file"`
	HTTPClientConfig   config.HTTPClientConfig `yaml:",inline"`
	NamespaceDiscovery NamespaceDiscovery      `yaml:"namespaces,omitempty"`
	Selectors          []SelectorConfig        `yaml:"selectors,omitempty"`
}

// Name returns the name of the Config.
func (*SDConfig) Name() string { return "kubernetes" }

// NewDiscoverer returns a Discoverer for the Config.
func (c *SDConfig) NewDiscoverer(opts discovery.DiscovererOptions) (discovery.Discoverer, error) {
	return New(opts.Logger, c)
}

// SetDirectory joins any relative file paths with dir.
func (c *SDConfig) SetDirectory(dir string) {
	c.HTTPClientConfig.SetDirectory(dir)
	c.KubeConfig = config.JoinDir(dir, c.KubeConfig)
}

type roleSelector struct {
	node          resourceSelector
	pod           resourceSelector
	service       resourceSelector
	endpoints     resourceSelector
	endpointslice resourceSelector
	ingress       resourceSelector
}

type SelectorConfig struct {
	Role  Role   `yaml:"role,omitempty"`
	Label string `yaml:"label,omitempty"`
	Field string `yaml:"field,omitempty"`
}

type resourceSelector struct {
	label string
	field string
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SDConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSDConfig
	type plain SDConfig
	err := unmarshal((*plain)(c))
	if err != nil {
		return err
	}
	if c.Role == "" {
		return fmt.Errorf("role missing (one of: pod, service, endpoints, endpointslice, node, ingress)")
	}
	err = c.HTTPClientConfig.Validate()
	if err != nil {
		return err
	}
	if c.APIServer.URL != nil && c.KubeConfig != "" {
		// Api-server and kubeconfig_file are mutually exclusive
		return fmt.Errorf("cannot use 'kubeconfig_file' and 'api_server' simultaneously")
	}
	if c.KubeConfig != "" && !reflect.DeepEqual(c.HTTPClientConfig, config.DefaultHTTPClientConfig) {
		// Kubeconfig_file and custom http config are mutually exclusive
		return fmt.Errorf("cannot use a custom HTTP client configuration together with 'kubeconfig_file'")
	}
	if c.APIServer.URL == nil && !reflect.DeepEqual(c.HTTPClientConfig, config.DefaultHTTPClientConfig) {
		return fmt.Errorf("to use custom HTTP client configuration please provide the 'api_server' URL explicitly")
	}

	foundSelectorRoles := make(map[Role]struct{})
	allowedSelectors := map[Role][]string{
		RolePod:           {string(RolePod)},
		RoleService:       {string(RoleService)},
		RoleEndpointSlice: {string(RolePod), string(RoleService), string(RoleEndpointSlice)},
		RoleEndpoint:      {string(RolePod), string(RoleService), string(RoleEndpoint)},
		RoleNode:          {string(RoleNode)},
		RoleIngress:       {string(RoleIngress)},
	}

	for _, selector := range c.Selectors {
		if _, ok := foundSelectorRoles[selector.Role]; ok {
			return fmt.Errorf("duplicated selector role: %s", selector.Role)
		}
		foundSelectorRoles[selector.Role] = struct{}{}

		if _, ok := allowedSelectors[c.Role]; !ok {
			return fmt.Errorf("invalid role: %q, expecting one of: pod, service, endpoints, endpointslice, node or ingress", c.Role)
		}
		var allowed bool
		for _, role := range allowedSelectors[c.Role] {
			if role == string(selector.Role) {
				allowed = true
				break
			}
		}

		if !allowed {
			return fmt.Errorf("%s role supports only %s selectors", c.Role, strings.Join(allowedSelectors[c.Role], ", "))
		}

		_, err := fields.ParseSelector(selector.Field)
		if err != nil {
			return err
		}
		_, err = labels.Parse(selector.Label)
		if err != nil {
			return err
		}
	}
	return nil
}

// NamespaceDiscovery is the configuration for discovering
// Kubernetes namespaces.
type NamespaceDiscovery struct {
	Names []string `yaml:"names"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NamespaceDiscovery) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = NamespaceDiscovery{}
	type plain NamespaceDiscovery
	return unmarshal((*plain)(c))
}

// Discovery implements the discoverer interface for discovering
// targets from Kubernetes.
type Discovery struct {
	sync.RWMutex
	client             k8s.Interface
	role               Role
	logger             logrus.FieldLogger
	namespaceDiscovery *NamespaceDiscovery
	discoverers        []discovery.Discoverer
	selectors          roleSelector
}

func (d *Discovery) getNamespaces() []string {
	namespaces := d.namespaceDiscovery.Names
	if len(namespaces) == 0 {
		namespaces = []string{apiv1.NamespaceAll}
	}
	return namespaces
}

// New creates a new Kubernetes discovery for the given role.
func New(l logrus.FieldLogger, conf *SDConfig) (*Discovery, error) {
	var (
		kcfg *rest.Config
		err  error
	)
	if conf.KubeConfig != "" {
		kcfg, err = clientcmd.BuildConfigFromFlags("", conf.KubeConfig)
		if err != nil {
			return nil, err
		}
	} else if conf.APIServer.URL == nil {
		// Use the Kubernetes provided pod service account
		// as described in https://kubernetes.io/docs/admin/service-accounts-admin/
		kcfg, err = rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		l.Debug("using pod service account via in-cluster config")
	} else {
		rt, err := config.NewRoundTripperFromConfig(conf.HTTPClientConfig, "kubernetes_sd")
		if err != nil {
			return nil, err
		}
		kcfg = &rest.Config{
			Host:      conf.APIServer.String(),
			Transport: rt,
		}
	}

	kcfg.UserAgent = userAgent

	c, err := k8s.NewForConfig(kcfg)
	if err != nil {
		return nil, err
	}
	return &Discovery{
		client:             c,
		logger:             l,
		role:               conf.Role,
		namespaceDiscovery: &conf.NamespaceDiscovery,
		discoverers:        make([]discovery.Discoverer, 0),
		selectors:          mapSelector(conf.Selectors),
	}, nil
}

func mapSelector(rawSelector []SelectorConfig) roleSelector {
	rs := roleSelector{}
	for _, resourceSelectorRaw := range rawSelector {
		switch resourceSelectorRaw.Role {
		case RoleEndpointSlice:
			rs.endpointslice.field = resourceSelectorRaw.Field
			rs.endpointslice.label = resourceSelectorRaw.Label
		case RoleEndpoint:
			rs.endpoints.field = resourceSelectorRaw.Field
			rs.endpoints.label = resourceSelectorRaw.Label
		case RoleIngress:
			rs.ingress.field = resourceSelectorRaw.Field
			rs.ingress.label = resourceSelectorRaw.Label
		case RoleNode:
			rs.node.field = resourceSelectorRaw.Field
			rs.node.label = resourceSelectorRaw.Label
		case RolePod:
			rs.pod.field = resourceSelectorRaw.Field
			rs.pod.label = resourceSelectorRaw.Label
		case RoleService:
			rs.service.field = resourceSelectorRaw.Field
			rs.service.label = resourceSelectorRaw.Label
		}
	}
	return rs
}

const resyncPeriod = 10 * time.Minute

// Run implements the discoverer interface.
func (d *Discovery) Run(ctx context.Context, ch chan<- []*targetgroup.Group) {
	d.Lock()
	namespaces := d.getNamespaces()

	switch d.role {
	case RoleEndpointSlice:
		for _, namespace := range namespaces {
			e := d.client.DiscoveryV1beta1().EndpointSlices(namespace)
			elw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.endpointslice.field
					options.LabelSelector = d.selectors.endpointslice.label
					return e.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.endpointslice.field
					options.LabelSelector = d.selectors.endpointslice.label
					return e.Watch(ctx, options)
				},
			}
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.Watch(ctx, options)
				},
			}
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.Watch(ctx, options)
				},
			}
			eps := NewEndpointSlice(
				d.logger.WithField("role", "endpointslice"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
				cache.NewSharedInformer(elw, &disv1beta1.EndpointSlice{}, resyncPeriod),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, eps)
			go eps.endpointSliceInf.Run(ctx.Done())
			go eps.serviceInf.Run(ctx.Done())
			go eps.podInf.Run(ctx.Done())
		}
	case RoleEndpoint:
		for _, namespace := range namespaces {
			e := d.client.CoreV1().Endpoints(namespace)
			elw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.endpoints.field
					options.LabelSelector = d.selectors.endpoints.label
					return e.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.endpoints.field
					options.LabelSelector = d.selectors.endpoints.label
					return e.Watch(ctx, options)
				},
			}
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.Watch(ctx, options)
				},
			}
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.Watch(ctx, options)
				},
			}
			eps := NewEndpoints(
				d.logger.WithField("role", "endpoint"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
				cache.NewSharedInformer(elw, &apiv1.Endpoints{}, resyncPeriod),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, eps)
			go eps.endpointsInf.Run(ctx.Done())
			go eps.serviceInf.Run(ctx.Done())
			go eps.podInf.Run(ctx.Done())
		}
	case RolePod:
		for _, namespace := range namespaces {
			p := d.client.CoreV1().Pods(namespace)
			plw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.pod.field
					options.LabelSelector = d.selectors.pod.label
					return p.Watch(ctx, options)
				},
			}
			pod := NewPod(
				d.logger.WithField("role", "pod"),
				cache.NewSharedInformer(plw, &apiv1.Pod{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, pod)
			go pod.informer.Run(ctx.Done())
		}
	case RoleService:
		for _, namespace := range namespaces {
			s := d.client.CoreV1().Services(namespace)
			slw := &cache.ListWatch{
				ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.List(ctx, options)
				},
				WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
					options.FieldSelector = d.selectors.service.field
					options.LabelSelector = d.selectors.service.label
					return s.Watch(ctx, options)
				},
			}
			svc := NewService(
				d.logger.WithField("role", "service"),
				cache.NewSharedInformer(slw, &apiv1.Service{}, resyncPeriod),
			)
			d.discoverers = append(d.discoverers, svc)
			go svc.informer.Run(ctx.Done())
		}
	case RoleIngress:
		// Check "networking.k8s.io/v1" availability with retries.
		// If "v1" is not avaiable, use "networking.k8s.io/v1beta1" for backward compatibility
		var v1Supported bool
		if retryOnError(ctx, 10*time.Second,
			func() (err error) {
				v1Supported, err = checkNetworkingV1Supported(d.client)
				if err != nil {
					d.logger.WithError(err).Error("failed to check networking.k8s.io/v1 availability")
				}
				return err
			},
		) {
			d.Unlock()
			return
		}

		for _, namespace := range namespaces {
			var informer cache.SharedInformer
			if v1Supported {
				i := d.client.NetworkingV1().Ingresses(namespace)
				ilw := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.Watch(ctx, options)
					},
				}
				informer = cache.NewSharedInformer(ilw, &networkv1.Ingress{}, resyncPeriod)
			} else {
				i := d.client.NetworkingV1beta1().Ingresses(namespace)
				ilw := &cache.ListWatch{
					ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.List(ctx, options)
					},
					WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
						options.FieldSelector = d.selectors.ingress.field
						options.LabelSelector = d.selectors.ingress.label
						return i.Watch(ctx, options)
					},
				}
				informer = cache.NewSharedInformer(ilw, &v1beta1.Ingress{}, resyncPeriod)
			}
			ingress := NewIngress(
				d.logger.WithField("role", "ingress"),
				informer,
			)
			d.discoverers = append(d.discoverers, ingress)
			go ingress.informer.Run(ctx.Done())
		}
	case RoleNode:
		nlw := &cache.ListWatch{
			ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
				options.FieldSelector = d.selectors.node.field
				options.LabelSelector = d.selectors.node.label
				return d.client.CoreV1().Nodes().List(ctx, options)
			},
			WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
				options.FieldSelector = d.selectors.node.field
				options.LabelSelector = d.selectors.node.label
				return d.client.CoreV1().Nodes().Watch(ctx, options)
			},
		}
		node := NewNode(
			d.logger.WithField("role", "node"),
			cache.NewSharedInformer(nlw, &apiv1.Node{}, resyncPeriod),
		)
		d.discoverers = append(d.discoverers, node)
		go node.informer.Run(ctx.Done())
	default:
		d.logger.WithField("role", d.role).Error("unknown Kubernetes discovery kind")
	}

	var wg sync.WaitGroup
	for _, dd := range d.discoverers {
		wg.Add(1)
		go func(d discovery.Discoverer) {
			defer wg.Done()
			d.Run(ctx, ch)
		}(dd)
	}

	d.Unlock()

	wg.Wait()
	<-ctx.Done()
}

func lv(s string) model.LabelValue {
	return model.LabelValue(s)
}

func send(ctx context.Context, ch chan<- []*targetgroup.Group, tg *targetgroup.Group) {
	if tg == nil {
		return
	}
	select {
	case <-ctx.Done():
	case ch <- []*targetgroup.Group{tg}:
	}
}

func retryOnError(ctx context.Context, interval time.Duration, f func() error) (canceled bool) {
	var err error
	err = f()
	for {
		if err == nil {
			return false
		}
		select {
		case <-ctx.Done():
			return true
		case <-time.After(interval):
			err = f()
		}
	}
}

func checkNetworkingV1Supported(client k8s.Interface) (bool, error) {
	k8sVer, err := client.Discovery().ServerVersion()
	if err != nil {
		return false, err
	}
	semVer, err := utilversion.ParseSemantic(k8sVer.String())
	if err != nil {
		return false, err
	}
	// networking.k8s.io/v1 is available since Kubernetes v1.19
	// https://github.com/kubernetes/kubernetes/blob/master/CHANGELOG/CHANGELOG-1.19.md
	return semVer.Major() >= 1 && semVer.Minor() >= 19, nil
}
