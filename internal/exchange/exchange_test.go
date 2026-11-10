package exchange

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

// newRingFile makes an empty ring file the way the engine would: zeroed
// bytes are a valid empty ring.
func newRingFile[T any](t *testing.T) string {
	var zeroSlot T
	path := filepath.Join(t.TempDir(), "ring")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(int64(2*cacheLine + ringSlots*int(unsafe.Sizeof(zeroSlot)))); err != nil {
		t.Fatal(err)
	}
	file.Close()
	return path
}

func TestRingEmptyFullFIFO(t *testing.T) {
	ring, err := attach[Request](newRingFile[Request](t))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ring.Pop(); ok {
		t.Fatal("pop on empty ring succeeded")
	}
	for i := range ringSlots - 1 { // capacity is N-1
		if !ring.Push(Request{OrderID: uint64(i)}) {
			t.Fatalf("push %d failed before full", i)
		}
	}
	if ring.Push(Request{}) {
		t.Fatal("push on full ring succeeded")
	}
	for i := range ringSlots - 1 {
		item, ok := ring.Pop()
		if !ok || item.OrderID != uint64(i) {
			t.Fatalf("pop %d: got %v %v", i, item.OrderID, ok)
		}
	}
}

func TestRingProducerConsumer(t *testing.T) {
	path := newRingFile[Request](t)
	producer, _ := attach[Request](path) // two separate mappings of one file,
	consumer, _ := attach[Request](path) // like two processes
	const messages = 200_000
	go func() {
		for i := range uint64(messages) {
			for !producer.Push(Request{OrderID: i, Price: int64(i) * 2, Qty: int64(i) * 3}) {
			}
		}
	}()
	for i := range uint64(messages) {
		item, ok := consumer.Pop()
		for !ok {
			item, ok = consumer.Pop()
		}
		if item.OrderID != i || item.Price != int64(i)*2 || item.Qty != int64(i)*3 {
			t.Fatalf("message %d out of order or torn: %+v", i, item)
		}
	}
}
