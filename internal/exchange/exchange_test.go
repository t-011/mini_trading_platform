package exchange

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/t-011/mini_trading_platform/internal/exchange/exchangetest"
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

func TestClientAgainstEngine(t *testing.T) {
	client, err := Dial(exchangetest.StartEngine(t, RingDir))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	must := func(events []Event, err error) []Event {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return events
	}

	// sell 10 @ 100 rests
	events := must(client.Submit(ctx, 1, false, 100, 10))
	if len(events) != 1 || events[0].Type != EvAccepted || events[0].Qty != 10 {
		t.Fatalf("rest: %+v", events)
	}

	// buy 4 @ 101 fills 4 @ 100 (maker's price), nothing rests
	events = must(client.Submit(ctx, 2, true, 101, 4))
	if len(events) != 2 {
		t.Fatalf("fill: %+v", events)
	}
	fill := events[0]
	if fill.Type != EvFill || fill.OrderID != 2 || fill.MakerID != 1 || fill.Price != 100 || fill.Qty != 4 {
		t.Fatalf("fill event: %+v", fill)
	}
	if events[1].Type != EvAccepted || events[1].Qty != 0 {
		t.Fatalf("fill terminal: %+v", events[1])
	}

	// cancel the 6 left on order 1, then again
	events = must(client.Cancel(ctx, 1))
	if len(events) != 1 || events[0].Type != EvCanceled || events[0].Qty != 6 {
		t.Fatalf("cancel: %+v", events)
	}
	events = must(client.Cancel(ctx, 1))
	if events[0].Type != EvRejected || events[0].Reason != ReasonUnknownOrder {
		t.Fatalf("cancel twice: %+v", events)
	}

	// zero qty rejected
	events = must(client.Submit(ctx, 3, true, 100, 0))
	if events[0].Type != EvRejected || events[0].Reason != ReasonBadOrder {
		t.Fatalf("bad order: %+v", events)
	}

	// many concurrent callers: each must get back the answer for its own order
	var callers sync.WaitGroup
	for i := range uint64(2000) {
		callers.Add(1)
		go func(id uint64) {
			defer callers.Done()
			events, err := client.Submit(ctx, id, true, 50, 1) // below any ask: rests
			if err != nil || len(events) != 1 || events[0].OrderID != id || events[0].Type != EvAccepted {
				t.Errorf("order %d got %+v %v", id, events, err)
			}
		}(1000 + i)
	}
	callers.Wait()
}

// TestEngineRecoversAfterCrash rests an order, SIGKILLs the engine, starts a
// new one on the same WAL and checks the replayed book still holds the order.
func TestEngineRecoversAfterCrash(t *testing.T) {
	engine := exchangetest.StartEngineWithWAL(t, RingDir, t.TempDir())

	// A Dialed client exits the process once its rings are replaced, so talk
	// to the first engine over the raw rings instead.
	inRing, err := attach[Request](filepath.Join(RingDir, engine.Symbol+".in"))
	if err != nil {
		t.Fatal(err)
	}
	outRing, err := attach[Event](filepath.Join(RingDir, engine.Symbol+".out"))
	if err != nil {
		t.Fatal(err)
	}
	// sell 10 @ 100 rests, then buy 4 @ 100 leaves 6 of it on the book
	inRing.Push(Request{OrderID: 1, Type: ReqNew, IsBuy: 0, Price: 100, Qty: 10})
	inRing.Push(Request{OrderID: 2, Type: ReqNew, IsBuy: 1, Price: 100, Qty: 4})

	// wait for all three answers (ACCEPTED, FILL, ACCEPTED): both requests are in the WAL
	var events []Event
	deadline := time.Now().Add(10 * time.Second)
	spins := 0
	for len(events) < 3 && time.Now().Before(deadline) {
		if event, ok := outRing.Pop(); ok {
			events = append(events, event)
		} else {
			idle(&spins)
		}
	}
	if len(events) != 3 {
		t.Fatalf("before crash: got %d events: %+v", len(events), events)
	}
	if events[0].Type != EvAccepted || events[0].OrderID != 1 || events[0].Qty != 10 {
		t.Fatalf("rest before crash: %+v", events[0])
	}
	fill := events[1]
	if fill.Type != EvFill || fill.OrderID != 2 || fill.MakerID != 1 || fill.Price != 100 || fill.Qty != 4 {
		t.Fatalf("fill before crash: %+v", fill)
	}
	if events[2].Type != EvAccepted || events[2].OrderID != 2 || events[2].Qty != 0 {
		t.Fatalf("taker terminal before crash: %+v", events[2])
	}

	engine.Crash()
	engine.Restart()

	client, err := Dial(engine.Symbol)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// the replayed book still has the 6 left on order 1
	events, err = client.Cancel(ctx, 1)
	if err != nil || len(events) != 1 || events[0].Type != EvCanceled || events[0].Qty != 6 {
		t.Fatalf("cancel after restart: %+v %v", events, err)
	}
	// order 2 filled completely before the crash, so it never rested
	events, err = client.Cancel(ctx, 2)
	if err != nil || len(events) != 1 || events[0].Type != EvRejected || events[0].Reason != ReasonUnknownOrder {
		t.Fatalf("cancel filled order after restart: %+v %v", events, err)
	}
}
