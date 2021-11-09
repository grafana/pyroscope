package scrape

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/prometheus/prometheus/pkg/labels"
)

// TargetHealth describes the health state of a target.
type TargetHealth string

// The possible health states of a target based on the last performed scrape.
const (
	HealthUnknown TargetHealth = "unknown"
	HealthGood    TargetHealth = "up"
	HealthBad     TargetHealth = "down"
)

// Target refers to a singular HTTP or HTTPS endpoint.
type Target struct {
	// Labels before any processing.
	discoveredLabels labels.Labels
	// Any labels that are added to this target and its metrics.
	labels labels.Labels
	// Additional URL parameters that are part of the target URL.
	params url.Values

	mtx                sync.RWMutex
	lastError          error
	lastScrape         time.Time
	lastScrapeDuration time.Duration
	health             TargetHealth
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(labels, discoveredLabels labels.Labels, params url.Values) *Target {
	return &Target{
		labels:           labels,
		discoveredLabels: discoveredLabels,
		params:           params,
		health:           HealthUnknown,
	}
}

func (t *Target) String() string {
	return t.URL().String()
}

// hash returns an identifying hash for the target.
func (t *Target) hash() uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(fmt.Sprintf("%016d", t.labels.Hash())))
	_, _ = h.Write([]byte(t.URL().String()))
	return h.Sum64()
}

// offset returns the time until the next scrape cycle for the target.
func (t *Target) offset(interval time.Duration) time.Duration {
	now := time.Now().UnixNano()

	// Base is a pinned to absolute time, no matter how often offset is called.
	var (
		base   = int64(interval) - now%int64(interval)
		offset = t.hash() % uint64(interval)
		next   = base + int64(offset)
	)

	if next > int64(interval) {
		next -= int64(interval)
	}
	return time.Duration(next)
}

// Labels returns a copy of the set of all public labels of the target.
func (t *Target) Labels() labels.Labels {
	lset := make(labels.Labels, 0, len(t.labels))
	for _, l := range t.labels {
		if l.Name == "__name__" || !strings.HasPrefix(l.Name, ReservedLabelPrefix) {
			lset = append(lset, l)
		}
	}
	return lset
}

// DiscoveredLabels returns a copy of the target's labels before any processing.
func (t *Target) DiscoveredLabels() labels.Labels {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	lset := make(labels.Labels, len(t.discoveredLabels))
	copy(lset, t.discoveredLabels)
	return lset
}

// SetDiscoveredLabels sets new DiscoveredLabels
func (t *Target) SetDiscoveredLabels(l labels.Labels) {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	t.discoveredLabels = l
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	params := url.Values{}

	for k, v := range t.params {
		params[k] = make([]string, len(v))
		copy(params[k], v)
	}

	return &url.URL{
		Scheme:   t.labels.Get(SchemeLabel),
		Host:     t.labels.Get(AddressLabel),
		Path:     t.labels.Get(ProfilePathLabel),
		RawQuery: params.Encode(),
	}
}

// Report sets target data about the last scrape.
func (t *Target) Report(start time.Time, dur time.Duration, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if err == nil {
		t.health = HealthGood
	} else {
		t.health = HealthBad
	}

	t.lastError = err
	t.lastScrape = start
	t.lastScrapeDuration = dur
}

// LastError returns the error encountered during the last scrape.
func (t *Target) LastError() error {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastError
}

// LastScrape returns the time of the last scrape.
func (t *Target) LastScrape() time.Time {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrape
}

// LastScrapeDuration returns how long the last scrape of the target took.
func (t *Target) LastScrapeDuration() time.Duration {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrapeDuration
}

// Health returns the last known health state of the target.
func (t *Target) Health() TargetHealth {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.health
}

// intervalAndTimeout returns the interval and timeout derived from
// the targets labels.
func (t *Target) intervalAndTimeout(defaultInterval, defaultDuration time.Duration) (time.Duration, time.Duration, error) {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	intervalLabel := t.labels.Get(ScrapeIntervalLabel)
	interval, err := time.ParseDuration(intervalLabel)
	if err != nil {
		return defaultInterval, defaultDuration, fmt.Errorf("parsing interval label %q: %w", intervalLabel, err)
	}
	timeoutLabel := t.labels.Get(ScrapeTimeoutLabel)
	timeout, err := time.ParseDuration(timeoutLabel)
	if err != nil {
		return defaultInterval, defaultDuration, fmt.Errorf("parsing timeout label %q: %w", timeoutLabel, err)
	}

	return interval, timeout, nil
}

// GetValue gets a label value from the entire label set.
func (t *Target) GetValue(name string) string {
	return t.labels.Get(name)
}

// Targets is a sortable list of targets.
type Targets []*Target

func (ts Targets) Len() int           { return len(ts) }
func (ts Targets) Less(i, j int) bool { return ts[i].URL().String() < ts[j].URL().String() }
func (ts Targets) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }

// PopulateLabels builds a label set from the given label set and scrape configuration.
// It returns a label set before relabeling was applied as the second return value.
// Returns the original discovered label set found before relabelling was applied if the target is dropped during relabeling.
func PopulateLabels(lset labels.Labels, cfg *ScrapeConfig) (res, orig labels.Labels, err error) {
	// Copy labels into the labelset for the target if they are not set already.
	scrapeLabels := []labels.Label{
		{Name: JobLabel, Value: cfg.JobName},
		{Name: ScrapeIntervalLabel, Value: cfg.ScrapeInterval.String()},
		{Name: ScrapeTimeoutLabel, Value: cfg.ScrapeTimeout.String()},
		{Name: SchemeLabel, Value: cfg.Scheme},
	}
	lb := labels.NewBuilder(lset)

	for _, l := range scrapeLabels {
		if lv := lset.Get(l.Name); lv == "" {
			lb.Set(l.Name, l.Value)
		}
	}

	preRelabelLabels := lb.Labels()
	// TODO(kolesnikovae): Add support for relabeling
	//	lset = relabel.Process(preRelabelLabels, cfg.RelabelConfigs...)
	lset = lb.Labels()
	// Check if the target was dropped.
	if lset == nil {
		return nil, preRelabelLabels, nil
	}
	addr := lset.Get(AddressLabel)
	if addr == "" {
		return nil, nil, errors.New("no address")
	}
	if v := lset.Get(AppNameLabel); v == "" {
		return nil, nil, errors.New("no app name")
	}

	lb = labels.NewBuilder(lset)
	// addPort checks whether we should add a default port to the address.
	// If the address is not valid, we don't append a port either.
	addPort := func(s string) bool {
		// If we can split, a port exists and we don't have to add one.
		if _, _, err := net.SplitHostPort(s); err == nil {
			return false
		}
		// If adding a port makes it valid, the previous error
		// was not due to an invalid address and we can append a port.
		_, _, err := net.SplitHostPort(s + ":1234")
		return err == nil
	}

	// If it's an address with no trailing port, infer it based on the used scheme.
	if addPort(addr) {
		// Addresses reaching this point are already wrapped in [] if necessary.
		switch lset.Get(SchemeLabel) {
		case "http", "":
			addr = addr + ":80"
		case "https":
			addr = addr + ":443"
		default:
			return nil, nil, fmt.Errorf("invalid scheme: %q", cfg.Scheme)
		}
		lb.Set(AddressLabel, addr)
	}

	if err = checkTargetAddress(addr); err != nil {
		return nil, nil, err
	}

	var interval string
	var intervalDuration time.Duration
	if interval = lset.Get(ScrapeIntervalLabel); interval != cfg.ScrapeInterval.String() {
		intervalDuration, err = time.ParseDuration(interval)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing scrape interval: %w", err)
		}
		if intervalDuration == 0 {
			return nil, nil, errors.New("scrape interval cannot be 0")
		}
	}

	var timeout string
	var timeoutDuration time.Duration
	if timeout = lset.Get(ScrapeTimeoutLabel); timeout != cfg.ScrapeTimeout.String() {
		timeoutDuration, err = time.ParseDuration(timeout)
		if err != nil {
			return nil, nil, fmt.Errorf("error parsing scrape timeout: %w", err)
		}
		if timeoutDuration == 0 {
			return nil, nil, errors.New("scrape timeout cannot be 0")
		}
	}

	if timeoutDuration > intervalDuration {
		return nil, nil, fmt.Errorf("scrape timeout cannot be greater than scrape interval (%q > %q)", timeout, interval)
	}

	// Meta labels are deleted after relabelling. Other internal labels propagate to
	// the target which decides whether they will be part of their label set.
	for _, l := range lset {
		if strings.HasPrefix(l.Name, MetaLabelPrefix) {
			lb.Del(l.Name)
		}
	}

	// Default the instance label to the target address.
	if v := lset.Get(InstanceLabel); v == "" {
		lb.Set(InstanceLabel, addr)
	}

	res = lb.Labels()
	for _, l := range res {
		// Check label values are valid, drop the target if not.
		if !isValidLabelValue(l.Value) {
			return nil, nil, fmt.Errorf("invalid label value for %q: %q", l.Name, l.Value)
		}
	}

	return res, preRelabelLabels, nil
}

func isValidLabelValue(v string) bool { return utf8.ValidString(v) }

func checkTargetAddress(address string) error {
	if strings.Contains(address, "/") {
		return fmt.Errorf("%q is not a valid hostname", address)
	}
	return nil
}

// TargetsFromGroup builds targets based on the given TargetGroup and config.
func (sp *scrapePool) TargetsFromGroup(tg Group, cfg *ScrapeConfig) ([]*Target, []error) {
	targets := make([]*Target, 0, len(tg.Targets))
	failures := []error{}

	for i, tlset := range tg.Targets {
		lbls := make([]labels.Label, 0, len(tlset)+len(tg.Labels))
		for ln, lv := range tlset {
			lbls = append(lbls, labels.Label{Name: ln, Value: lv})
		}
		for ln, lv := range tg.Labels {
			if _, ok := tlset[ln]; !ok {
				lbls = append(lbls, labels.Label{Name: ln, Value: lv})
			}
		}

		for _, profileType := range cfg.EnabledProfiles {
			lset := labels.New(lbls...)
			lbls, origLabels, err := PopulateLabels(lset, cfg)
			if err != nil {
				failures = append(failures, fmt.Errorf("instance %d in group %s: %w", i, tg.Source, err))
			}
			if lbls == nil || origLabels == nil {
				continue
			}
			lbls = append(lbls, labels.Label{
				Name:  ProfilePathLabel,
				Value: cfg.ProfilingConfigs[profileType].Path,
			})

			// TODO(kolesnikovae): Refactor.
			params := cfg.ProfilingConfigs[profileType].Params
			switch profileType {
			case ProfileCPU:
				scrapeInterval, _ := time.ParseDuration(lbls.Get(ScrapeIntervalLabel))
				s, ok := params["seconds"]
				if !ok || len(s) == 0 {
					params["seconds"] = []string{strconv.Itoa(int(scrapeInterval.Seconds()))}
					break
				}
				scrapeDurationSeconds, err := strconv.Atoi(s[0])
				if err != nil {
					sp.logger.WithError(err).WithField("target", lbls).Errorf("invalid scrape duration")
					params["seconds"] = []string{strconv.Itoa(int(scrapeInterval.Seconds()))}
					break
				}
				if scrapeDurationSeconds == 0 || scrapeDurationSeconds > int(scrapeInterval.Seconds()) {
					sp.logger.WithField("target", lbls).
						Errorf("scrape duration can not be zero or greater than scrape interval")
					params["seconds"] = []string{strconv.Itoa(int(scrapeInterval.Seconds()))}
				}
			}

			targets = append(targets, NewTarget(lbls, origLabels, params))
		}
	}

	return targets, failures
}
