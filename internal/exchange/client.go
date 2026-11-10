package exchange

import (
	"context"
	"path/filepath"
)

// Client is one stock's connection to the engine. Safe for concurrent use:
// callers hand requests to a single writer goroutine (the ring's only
// producer), and a single reader goroutine (the only consumer) hands events
// back.
//
// The engine answers requests strictly in order, each with zero or more
// fills and then one terminal event, so the reader pairs answers with
// requests FIFO through `pending`; no lookup table.
type Client struct {
	in      *Ring[Request]
	out     *Ring[Event]
	calls   chan call // callers -> writer
	pending chan call // writer -> reader, in the order requests hit the ring
}

type call struct {
	request Request
	done    chan []Event // buffered 1 so the reader never blocks on a caller that gave up
}

// Dial attaches to a running engine's rings for symbol. The engine must be
// started first (`engine/me --serve SYMBOL`); it creates the ring files.
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
	// ponytail: no Close, goroutines run for the life of the process
	return client, nil
}

// Submit places a limit order. The result is the fills (EvFill) followed by
// the terminal event: EvAccepted (Qty = left resting) or EvRejected.
func (client *Client) Submit(ctx context.Context, orderID uint64, isBuy bool, price, qty int64) ([]Event, error) {
	var side uint8 // 1 buy, 0 sell
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

// do sends one request and waits for its answer. If ctx ends after the
// request was sent, the engine still executes it; the caller must reconcile.
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
		client.pending <- nextCall // before Push: the reader must know who the answer is for
		spins := 0
		for !client.in.Push(nextCall.request) {
			idle(&spins)
		}
	}
}

// ponytail: Event.Seq isn't checked; check for gaps once the engine can restart under us
func (client *Client) read() {
	var answer []Event // events for the oldest pending call, collected until its terminal event
	spins := 0
	for {
		event, ok := client.out.Pop()
		if !ok {
			idle(&spins)
			continue
		}
		spins = 0
		answer = append(answer, event)
		if event.Type == EvFill {
			continue
		}
		answeredCall := <-client.pending
		answeredCall.done <- answer
		answer = nil
	}
}
