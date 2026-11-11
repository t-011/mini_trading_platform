package exchange

import (
	"context"
	"log"
	"path/filepath"
	"time"
)

// Client is one stock's connection to the engine, safe for concurrent use.
// One goroutine writes requests and one reads events. The engine answers in
// order, so answers are matched to requests FIFO through pending.
type Client struct {
	in      *Ring[Request]
	out     *Ring[Event]
	calls   chan call
	pending chan call
}

type call struct {
	request Request
	done    chan []Event // buffered so the reader doesn't block if the caller gave up
}

// Dial attaches to the rings of a running engine (`me --serve SYMBOL`).
func Dial(symbol string) (*Client, error) {
	basePath := filepath.Join(RingDir, symbol)
	inRing, err := attach[Request](basePath + ".in")
	if err != nil {
		return nil, err
	}
	outRing, err := attach[Event](basePath + ".out")
	if err != nil {
		return nil, err
	}

	client := &Client{
		in:      inRing,
		out:     outRing,
		calls:   make(chan call),
		pending: make(chan call, ringSlots),
	}
	go client.write()
	go client.read()
	// TODO: no Close, the goroutines live as long as the process
	return client, nil
}

// Submit places a limit order. The result is the fills (EvFill) followed by
// the terminal event: EvAccepted (Qty = left resting) or EvRejected.
func (client *Client) Submit(ctx context.Context, orderID uint64, isBuy bool, price, qty int64) ([]Event, error) {
	var side uint8
	if isBuy {
		side = 1
	}
	return client.do(ctx, Request{OrderID: orderID, Type: ReqNew, IsBuy: side, Price: price, Qty: qty})
}

// Cancel removes a resting order. The result is EvCanceled (Qty = removed)
// or EvRejected with ReasonUnknownOrder.
func (client *Client) Cancel(ctx context.Context, orderID uint64) ([]Event, error) {
	return client.do(ctx, Request{OrderID: orderID, Type: ReqCancel})
}

// If ctx ends after the request was sent, the engine still runs it.
func (client *Client) do(ctx context.Context, request Request) ([]Event, error) {
	newCall := call{request: request, done: make(chan []Event, 1)}
	select {
	case client.calls <- newCall:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case events := <-newCall.done:
		return events, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (client *Client) write() {
	for nextCall := range client.calls {
		client.pending <- nextCall // before Push, or the answer could arrive first
		spins := 0
		for !client.in.Push(nextCall.request) {
			idle(&spins)
		}
	}
}

// read hands each answer to its caller. If the engine restarts (new ring
// files, or a gap in Event.Seq) it exits the process; compose restarts it and
// it dials the new rings. The engine itself rebuilds its books from its WAL.
// TODO: re-dial in place instead of exiting
func (client *Client) read() {
	var answer []Event
	var lastSeq uint64
	lastCheck := time.Now()
	spins := 0
	for {
		event, ok := client.out.Pop()
		if !ok {
			if time.Since(lastCheck) > time.Second {
				lastCheck = time.Now()
				if client.out.replaced() {
					log.Fatalf("exchange: %s was replaced, engine restarted", client.out.path)
				}
			}
			idle(&spins)
			continue
		}
		spins = 0
		if event.Seq != lastSeq+1 {
			log.Fatalf("exchange: event seq %d after %d, engine restarted", event.Seq, lastSeq)
		}
		lastSeq = event.Seq
		answer = append(answer, event)
		if event.Type == EvFill {
			continue
		}
		answeredCall := <-client.pending
		answeredCall.done <- answer
		answer = nil
	}
}
