package novelty

import (
	"testing"
)

func TestSamplesAdd(t *testing.T) {
	t.Run("empty samples", func(t *testing.T) {
		s := NewSamples(0, 0.85)

		stack := []string{"func1", "func2"}
		values := []int64{10, 20}
		novelty := s.Add(stack, values)

		if novelty != 0.0 {
			t.Errorf("expected novelty 0.0 for first sample, got %f", novelty)
		}
		if len(s.profiles) != 1 {
			t.Errorf("expected 1 profile, got %d", len(s.profiles))
		}
		if len(s.totals) != 1 {
			t.Errorf("expected 1 total, got %d", len(s.totals))
		}
		if s.totals[0] != 30 {
			t.Errorf("expected total 30, got %d", s.totals[0])
		}
	})

	t.Run("equal samples merge", func(t *testing.T) {
		s := NewSamples(0, 0.85)

		// First sample
		stack1 := []string{"func1", "func2"}
		values1 := []int64{10, 20}
		s.Add(stack1, values1)

		// Second similar sample
		stack2 := []string{"func1", "func2"}
		values2 := []int64{15, 30}
		novelty2 := s.Add(stack2, values2)

		if novelty2 < 1.0 {
			t.Errorf("expected high novelty for similar samples, got %f", novelty2)
		}
		if len(s.profiles) != 1 {
			t.Errorf("expected profiles to merge, got %d profiles", len(s.profiles))
		}
		if s.totals[0] != 75 {
			t.Errorf("expected merged total 70, got %d", s.totals[0])
		}
	})

	t.Run("similar samples merge", func(t *testing.T) {
		s := NewSamples(0, 0.85)

		// First sample
		stack1 := []string{"func1", "func2", "func3"}
		values1 := []int64{10, 20, 2}
		s.Add(stack1, values1)

		// Second similar sample
		stack2 := []string{"func1", "func2"}
		values2 := []int64{15, 30}
		novelty2 := s.Add(stack2, values2)

		if novelty2 < 0.8 {
			t.Errorf("expected high novelty for similar samples, got %f", novelty2)
		}
		if len(s.profiles) != 1 {
			t.Errorf("expected profiles to merge, got %d profiles", len(s.profiles))
		}
		if s.totals[0] != 77 {
			t.Errorf("expected merged total 70, got %d", s.totals[0])
		}
	})

	t.Run("different samples create new profile", func(t *testing.T) {
		s := NewSamples(0, 0.85)

		// First sample
		stack1 := []string{"func1", "func2"}
		values1 := []int64{10, 20}
		s.Add(stack1, values1)

		// Second different sample
		stack2 := []string{"func3", "func4"}
		values2 := []int64{30, 40}
		novelty2 := s.Add(stack2, values2)

		if novelty2 > 0.0 {
			t.Errorf("expected low novelty for different samples, got %f", novelty2)
		}
		if len(s.profiles) != 2 {
			t.Errorf("expected 2 profiles, got %d", len(s.profiles))
		}
		if len(s.totals) != 2 {
			t.Errorf("expected 2 totals, got %d", len(s.totals))
		}
	})
}
