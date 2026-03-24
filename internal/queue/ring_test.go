package queue

import "testing"

func TestRingBasic(t *testing.T) {
	q, err := NewRing[int](8)
	if err != nil {
		t.Fatalf("new ring: %v", err)
	}
	for i := 0; i < 8; i++ {
		ok := q.Enqueue(i)
		if !ok {
			t.Fatalf("enqueue %d failed", i)
		}
	}
	if q.Enqueue(9) {
		t.Fatalf("enqueue should fail when full")
	}
	for i := 0; i < 8; i++ {
		v, ok := q.Dequeue()
		if !ok {
			t.Fatalf("dequeue %d failed", i)
		}
		if v != i {
			t.Fatalf("dequeue order mismatch got %d want %d", v, i)
		}
	}
	if _, ok := q.Dequeue(); ok {
		t.Fatalf("dequeue should fail when empty")
	}
}

func TestRingPowerOfTwo(t *testing.T) {
	if _, err := NewRing[int](10); err == nil {
		t.Fatalf("expected error for non power-of-two capacity")
	}
}
