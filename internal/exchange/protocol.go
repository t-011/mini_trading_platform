// Package exchange talks to the C++ engine over shared-memory rings.
// The types mirror engine/protocol.hpp.
package exchange

import (
	"os"
	"unsafe"
)

// RingDir holds <SYMBOL>.in and <SYMBOL>.out. Set by MINI_EXCHANGE_DIR, same as the engine.
var RingDir = ringDir()

func ringDir() string {
	if dir := os.Getenv("MINI_EXCHANGE_DIR"); dir != "" {
		return dir
	}
	return "/tmp/mini_exchange"
}

const (
	ringSlots = 4096
	cacheLine = 128 // head at 0, tail at 128, slots from 256
)

// Request types.
const (
	ReqNew    uint8 = 1
	ReqCancel uint8 = 2
)

// Event types. A request gets zero or more EvFill, then one of the others.
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
	Seq     uint64
	OrderID uint64 // the request's order (the taker, for EvFill)
	MakerID uint64 // EvFill only
	Price   int64  // EvFill: trade price
	Qty     int64  // EvFill: traded, EvAccepted: left resting, EvCanceled: removed
	Type    uint8
	Reason  uint8 // EvRejected only
	_       [6]byte
}

// Fails to compile if the sizes drift from the C++ structs.
var (
	_ [32]byte = [unsafe.Sizeof(Request{})]byte{}
	_ [48]byte = [unsafe.Sizeof(Event{})]byte{}
)
