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

// Ring is the Go side of RingBuffer in engine/ring_buffer.hpp. Go only
// pushes to the .in ring and only pops from the .out ring.
type Ring[T any] struct {
	head  *uint64
	tail  *uint64
	slots []T
	path  string
	inode uint64 // at attach time, to notice the engine replacing the file
}

// attach maps a ring file the engine already created.
func attach[T any](path string) (*Ring[T], error) {
	var zeroSlot T
	ringSize := 2*cacheLine + ringSlots*int(unsafe.Sizeof(zeroSlot))

	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()

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
		path:  path,
		inode: info.Sys().(*syscall.Stat_t).Ino,
	}, nil
}

// replaced reports whether a restarted engine created a new file at path.
// A missing file (engine not back yet) doesn't count.
func (ring *Ring[T]) replaced() bool {
	info, err := os.Stat(ring.path)
	return err == nil && info.Sys().(*syscall.Stat_t).Ino != ring.inode
}

// Push returns false if the ring is full.
func (ring *Ring[T]) Push(item T) bool {
	head := atomic.LoadUint64(ring.head)
	nextHead := (head + 1) & (ringSlots - 1)
	if nextHead == atomic.LoadUint64(ring.tail) {
		return false
	}
	ring.slots[head] = item
	atomic.StoreUint64(ring.head, nextHead)
	return true
}

// Pop returns false if the ring is empty.
func (ring *Ring[T]) Pop() (T, bool) {
	var item T
	tail := atomic.LoadUint64(ring.tail)
	if tail == atomic.LoadUint64(ring.head) {
		return item, false
	}
	item = ring.slots[tail]
	atomic.StoreUint64(ring.tail, (tail+1)&(ringSlots-1))
	return item, true
}

// idle yields for a while, then sleeps, so a quiet stock doesn't eat a core.
func idle(spins *int) {
	*spins++
	if *spins < 100 {
		runtime.Gosched()
		return
	}
	time.Sleep(50 * time.Microsecond)
}
