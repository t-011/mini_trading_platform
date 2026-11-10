// Package exchange talks to the C++ matching engine (engine/matching_engine.cpp)
// over shared-memory rings. Every type here mirrors engine/protocol.hpp byte for
// byte: change one, change the other.
package exchange

import "unsafe"

const (
	RingDir   = "/tmp/mini_exchange" // <SYMBOL>.in (Go -> engine), <SYMBOL>.out (engine -> Go)
	ringSlots = 4096                 // RING_SLOTS
	cacheLine = 128                  // CACHE_LINE: head @0, tail @128, slots @256
)

// Request types.
const (
	ReqNew    uint8 = 1
	ReqCancel uint8 = 2
)

// Event types. Each request yields zero or more EvFill, then exactly one of
// the others (the terminal event).
const (
	EvAccepted uint8 = 1
	EvFill     uint8 = 2
	EvCanceled uint8 = 3
	EvRejected uint8 = 4
)

// Reject reasons.
const (
	ReasonNone         uint8 = 0
	ReasonBadOrder     uint8 = 1
	ReasonUnknownOrder uint8 = 2
)

// Units: price in cents, qty in 1e-8 shares.
type Request struct {
	OrderID uint64
	Price   int64 // ReqNew only
	Qty     int64 // ReqNew only
	Type    uint8
	IsBuy   uint8 // ReqNew only: 1 buy, 0 sell
	_       [6]byte
}

type Event struct {
	Seq     uint64 // 1, 2, 3... per ring
	OrderID uint64 // the request's order (the taker, for EvFill)
	MakerID uint64 // EvFill only
	Price   int64  // EvFill: trade price
	Qty     int64  // EvFill: traded, EvAccepted: left resting, EvCanceled: removed
	Type    uint8
	Reason  uint8 // EvRejected only
	_       [6]byte
}

// Compile-time layout checks: these fail to build if a size drifts from C++.
var (
	_ [32]byte = [unsafe.Sizeof(Request{})]byte{}
	_ [48]byte = [unsafe.Sizeof(Event{})]byte{}
)
