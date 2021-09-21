package lfu

import (
	"testing"
)

func TestLFU(t *testing.T) {
	c := New()
	c.Set("a", "a")
	if v := c.Get("a"); v != "a" {
		t.Errorf("Value was not saved: %v != 'a'", v)
	}
	if l := c.Len(); l != 1 {
		t.Errorf("Length was not updated: %v != 1", l)
	}

	c.Set("b", "b")
	if v := c.Get("b"); v != "b" {
		t.Errorf("Value was not saved: %v != 'b'", v)
	}
	if l := c.Len(); l != 2 {
		t.Errorf("Length was not updated: %v != 2", l)
	}

	c.Get("a")
	evicted := c.Evict(1)
	if v := c.Get("a"); v != "a" {
		t.Errorf("Value was improperly evicted: %v != 'a'", v)
	}
	if v := c.Get("b"); v != nil {
		t.Errorf("Value was not evicted: %v", v)
	}
	if l := c.Len(); l != 1 {
		t.Errorf("Length was not updated: %v != 1", l)
	}
	if evicted != 1 {
		t.Errorf("Number of evicted items is wrong: %v != 1", evicted)
	}
}

func TestEviction(t *testing.T) {
	ch := make(chan Eviction, 1)

	c := New()
	c.EvictionChannel = ch
	c.Set("a", "b")
	c.Evict(1)

	ev := <-ch

	if ev.Key != "a" || ev.Value.(string) != "b" {
		t.Error("Incorrect item")
	}
}

func TestEvictionOrder(t *testing.T) {
	c := New()
	c.Set("a1", 1)
	c.Set("a2", 2)
	c.Set("a3", 3)
	c.Get("a1")
	c.Evict(2)

	if e := c.Get("a1"); e == nil {
		t.Error("Incorrect eviction order")
	}
}
