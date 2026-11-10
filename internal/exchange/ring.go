package exchange

import (
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Ring is the Go end of a RingBuffer<T, RING_SLOTS> (engine/ring_buffer.hpp) living
// in a shared file mapping. Single producer, single consumer: on a given ring
// Go only ever calls Push (the .in ring) or only Pop (the .out ring).
type Ring[T any] struct {
	head  *uint64 // next slot to write; only the producer stores it
	tail  *uint64 // next slot to read; only the consumer stores it
	slots []T
}

// attach maps a ring file the engine already created.
func attach[T any](path string) (*Ring[T], error) {
	var zeroSlot T
	ringSize := 2*cacheLine + ringSlots*int(unsafe.Sizeof(zeroSlot))

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close() // the mapping outlives the descriptor

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() != int64(ringSize) {
		return nil, fmt.Errorf("%s: size %d, want %d (Go and C++ layouts differ)", path, info.Size(), ringSize)
	}

	memory, err := syscall.Mmap(int(file.Fd()), 0, ringSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return nil, fmt.Errorf("mmap %s: %w", path, err)
	}

	return &Ring[T]{
		head:  (*uint64)(unsafe.Pointer(&memory[0])),
		tail:  (*uint64)(unsafe.Pointer(&memory[cacheLine])),
		slots: unsafe.Slice((*T)(unsafe.Pointer(&memory[2*cacheLine])), ringSlots),
	}, nil
}

// Push is the producer side. False means full; the caller retries.
func (ring *Ring[T]) Push(item T) bool {
	head := atomic.LoadUint64(ring.head)
	nextHead := (head + 1) & (ringSlots - 1)
	if nextHead == atomic.LoadUint64(ring.tail) {
		return false
	}
	ring.slots[head] = item
	atomic.StoreUint64(ring.head, nextHead) // publishes the slot write
	return true
}

// Pop is the consumer side. False means empty.
func (ring *Ring[T]) Pop() (T, bool) {
	var item T
	tail := atomic.LoadUint64(ring.tail)
	if tail == atomic.LoadUint64(ring.head) {
		return item, false
	}
	item = ring.slots[tail]
	atomic.StoreUint64(ring.tail, (tail+1)&(ringSlots-1)) // frees the slot
	return item, true
}

// idle backs off while a ring is empty or full: yield first, then sleep,
// so a quiet stock doesn't pin a core.
func idle(spins *int) {
	*spins++
	if *spins < 100 {
		runtime.Gosched()
		return
	}
	time.Sleep(50 * time.Microsecond)
}
