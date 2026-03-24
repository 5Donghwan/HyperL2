package queue

import (
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
)

var (
	ErrCapacityPowerOfTwo = errors.New("ring capacity must be a power of two")
)

type cell[T any] struct {
	sequence atomic.Uint64
	value    T
}

// Ring is a bounded lock-free MPMC ring queue.
type Ring[T any] struct {
	mask       uint64
	cells      []cell[T]
	enqueuePos atomic.Uint64
	dequeuePos atomic.Uint64
	length     atomic.Int64
}

func NewRing[T any](capacity uint64) (*Ring[T], error) {
	if capacity < 2 || capacity&(capacity-1) != 0 {
		return nil, fmt.Errorf("%w: got %d", ErrCapacityPowerOfTwo, capacity)
	}
	cells := make([]cell[T], capacity)
	for i := uint64(0); i < capacity; i++ {
		cells[i].sequence.Store(i)
	}
	q := &Ring[T]{
		mask:  capacity - 1,
		cells: cells,
	}
	return q, nil
}

func (q *Ring[T]) Enqueue(v T) bool {
	for {
		pos := q.enqueuePos.Load()
		c := &q.cells[pos&q.mask]
		seq := c.sequence.Load()
		dif := int64(seq) - int64(pos)
		switch {
		case dif == 0:
			if q.enqueuePos.CompareAndSwap(pos, pos+1) {
				c.value = v
				c.sequence.Store(pos + 1)
				q.length.Add(1)
				return true
			}
		case dif < 0:
			return false
		default:
			runtime.Gosched()
		}
	}
}

func (q *Ring[T]) Dequeue() (T, bool) {
	var zero T
	for {
		pos := q.dequeuePos.Load()
		c := &q.cells[pos&q.mask]
		seq := c.sequence.Load()
		dif := int64(seq) - int64(pos+1)
		switch {
		case dif == 0:
			if q.dequeuePos.CompareAndSwap(pos, pos+1) {
				v := c.value
				c.value = zero
				c.sequence.Store(pos + q.mask + 1)
				q.length.Add(-1)
				return v, true
			}
		case dif < 0:
			return zero, false
		default:
			runtime.Gosched()
		}
	}
}

func (q *Ring[T]) Len() int64 {
	return q.length.Load()
}

func (q *Ring[T]) Capacity() uint64 {
	return uint64(len(q.cells))
}
