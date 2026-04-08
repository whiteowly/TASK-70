package cache

import (
	"testing"
	"time"
)

func TestLRUGetSetExpiry(t *testing.T) {
	c := NewLRU(10, 50*time.Millisecond)

	c.Set("a", "hello")

	v, ok := c.Get("a")
	if !ok || v.(string) != "hello" {
		t.Fatalf("expected hit with value hello, got ok=%v v=%v", ok, v)
	}

	// Wait for expiry
	time.Sleep(60 * time.Millisecond)

	v, ok = c.Get("a")
	if ok {
		t.Fatalf("expected miss after expiry, got ok=%v v=%v", ok, v)
	}

	if c.Len() != 0 {
		t.Fatalf("expected len 0 after expiry eviction, got %d", c.Len())
	}
}

func TestLRUEviction(t *testing.T) {
	c := NewLRU(3, 10*time.Second)

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	if c.Len() != 3 {
		t.Fatalf("expected len 3, got %d", c.Len())
	}

	// Adding a 4th item should evict "a" (least recently used)
	c.Set("d", 4)

	if c.Len() != 3 {
		t.Fatalf("expected len 3 after eviction, got %d", c.Len())
	}

	_, ok := c.Get("a")
	if ok {
		t.Fatal("expected 'a' to be evicted")
	}

	v, ok := c.Get("d")
	if !ok || v.(int) != 4 {
		t.Fatalf("expected 'd'=4, got ok=%v v=%v", ok, v)
	}
}

func TestLRUEvictionOrder(t *testing.T) {
	c := NewLRU(3, 10*time.Second)

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	// Access "a" to make it recently used
	c.Get("a")

	// Adding "d" should evict "b" (now least recently used)
	c.Set("d", 4)

	_, ok := c.Get("b")
	if ok {
		t.Fatal("expected 'b' to be evicted")
	}

	_, ok = c.Get("a")
	if !ok {
		t.Fatal("expected 'a' to still be present")
	}
}

func TestLRUInvalidate(t *testing.T) {
	c := NewLRU(10, 10*time.Second)

	c.Set("x", "val")

	v, ok := c.Get("x")
	if !ok || v.(string) != "val" {
		t.Fatalf("expected hit, got ok=%v v=%v", ok, v)
	}

	c.Invalidate("x")

	_, ok = c.Get("x")
	if ok {
		t.Fatal("expected miss after invalidation")
	}

	if c.Len() != 0 {
		t.Fatalf("expected len 0, got %d", c.Len())
	}
}

func TestLRUClear(t *testing.T) {
	c := NewLRU(10, 10*time.Second)

	c.Set("a", 1)
	c.Set("b", 2)
	c.Set("c", 3)

	c.Clear()

	if c.Len() != 0 {
		t.Fatalf("expected len 0 after clear, got %d", c.Len())
	}

	_, ok := c.Get("a")
	if ok {
		t.Fatal("expected miss after clear")
	}
}

func TestLRUOverwrite(t *testing.T) {
	c := NewLRU(10, 10*time.Second)

	c.Set("a", 1)
	c.Set("a", 2)

	if c.Len() != 1 {
		t.Fatalf("expected len 1 after overwrite, got %d", c.Len())
	}

	v, ok := c.Get("a")
	if !ok || v.(int) != 2 {
		t.Fatalf("expected 2 after overwrite, got ok=%v v=%v", ok, v)
	}
}
