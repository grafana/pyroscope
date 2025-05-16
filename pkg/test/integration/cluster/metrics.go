package cluster

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	pm "github.com/prometheus/client_model/go"
)

type gatherCheck struct {
	g          prometheus.Gatherer
	conditions []gatherCoditions
}

func matchValue(exp float64) func(float64) error {
	return func(value float64) error {
		if value == exp {
			return nil
		}
		return fmt.Errorf("expected %f, got %f", exp, value)
	}
}

//nolint:unparam
func (c *gatherCheck) addExpectValue(value float64, metricName string, labelPairs ...string) *gatherCheck {
	c.conditions = append(c.conditions, gatherCoditions{
		metricName: metricName,
		labelPairs: labelPairs,
		valueCheck: matchValue(value),
	})
	return c
}

func retrieveValue(ch chan float64) func(float64) error {
	return func(value float64) error {
		ch <- value
		return nil
	}
}

func (c *gatherCheck) addRetrieveValue(valueCh chan float64, metricName string, labelPairs ...string) *gatherCheck {
	c.conditions = append(c.conditions, gatherCoditions{
		metricName: metricName,
		labelPairs: labelPairs,
		valueCheck: retrieveValue(valueCh),
	})
	return c
}

type gatherCoditions struct {
	metricName string
	labelPairs []string
	valueCheck func(float64) error
}

func (c *gatherCoditions) String() string {
	b := strings.Builder{}
	b.WriteString(c.metricName)
	b.WriteRune('{')
	for i := 0; i < len(c.labelPairs); i += 2 {
		b.WriteString(c.labelPairs[i])
		b.WriteRune('=')
		b.WriteString(c.labelPairs[i+1])
		b.WriteRune(',')
	}
	s := b.String()
	return s[:len(s)-1] + "}"
}

func (c *gatherCoditions) matches(pairs []*pm.LabelPair) bool {
outer:
	for i := 0; i < len(c.labelPairs); i += 2 {
		for _, l := range pairs {
			if l.GetName() != c.labelPairs[i] {
				continue
			}
			if l.GetValue() == c.labelPairs[i+1] {
				continue outer // match move to next pair
			}
			return false // value wrong
		}
		return false // label not found
	}
	return true
}

func (comp *Component) checkMetrics() *gatherCheck {
	return &gatherCheck{
		g: comp.reg,
	}
}

func (g *gatherCheck) run(ctx context.Context) error {
	actualValues := make([]float64, len(g.conditions))

	// maps from metric name to condition index
	nameMap := make(map[string][]int)
	for idx, c := range g.conditions {
		// not a number
		actualValues[idx] = math.NaN()
		nameMap[c.metricName] = append(nameMap[c.metricName], idx)
	}

	// now gather actual metrics
	metrics, err := g.g.Gather()
	if err != nil {
		return err
	}

	for _, m := range metrics {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		conditions, ok := nameMap[m.GetName()]
		if !ok {
			continue
		}

		// now iterate over all label pairs
		for _, sm := range m.GetMetric() {
			// check for each condition if it matches with he labels
			for _, condIdx := range conditions {
				if g.conditions[condIdx].matches(sm.Label) {
					v := -1.0 // -1.0 is an invalid value, when metric type is not gauge or counter
					if g := sm.GetGauge(); g != nil {
						v = g.GetValue()
					} else if c := sm.GetCounter(); c != nil {
						v = c.GetValue()
					}
					actualValues[condIdx] = v
				}
			}
		}
	}

	errs := make([]error, len(actualValues))
	for idx, actual := range actualValues {
		cond := g.conditions[idx]
		if math.IsNaN(actual) {
			errs[idx] = fmt.Errorf("metric for %s not found", cond.String())
			continue
		}
		if err := cond.valueCheck(actual); err != nil {
			errs[idx] = fmt.Errorf("unexpected value for %s: %w", cond.String(), err)
		}
	}

	return errors.Join(errs...)
}
