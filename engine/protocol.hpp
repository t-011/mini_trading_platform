#pragma once
// The Go <-> engine contract. Go mirrors every byte of this in
// internal/exchange/protocol.go: change one, change the other.
//
// Each stock has two rings in the ring dir (default /tmp/mini_exchange/):
//   <SYMBOL>.in   Go -> engine   (Request)
//   <SYMBOL>.out  engine -> Go   (Event)
// The ring itself identifies the stock, so messages carry no symbol.
//
// Every Request produces zero or more FILLs followed by exactly one terminal
// event (ACCEPTED, CANCELED or REJECTED), in order. That lets Go pair
// responses with requests FIFO, no lookup table.
//
// Units: price in cents, qty in 1e-8 shares (matches NUMERIC(20,8)).

#include <cstdint>
#include "ring_buffer.hpp"

enum ReqType : uint8_t { REQ_NEW = 1, REQ_CANCEL = 2 };
enum EvType : uint8_t { EV_ACCEPTED = 1, EV_FILL = 2, EV_CANCELED = 3, EV_REJECTED = 4 };
enum Reason : uint8_t { R_NONE = 0, R_BAD_ORDER = 1, R_UNKNOWN_ORDER = 2 };

// Fields ordered largest first so there is no hidden padding.
struct Request {
    uint64_t order_id;  // assigned by Go
    int64_t price;      // NEW only
    int64_t qty;        // NEW only
    uint8_t type;       // ReqType
    uint8_t is_buy;     // NEW only: 1 buy, 0 sell
    uint8_t _pad[6];
};

struct Event {
    uint64_t seq;       // 1, 2, 3... per ring, for gap detection
    uint64_t order_id;  // the request's order (the taker, for FILL)
    uint64_t maker_id;  // FILL only
    int64_t price;      // FILL: trade price
    int64_t qty;        // FILL: traded qty, ACCEPTED: qty left resting, CANCELED: qty removed
    uint8_t type;       // EvType
    uint8_t reason;     // REJECTED only
    uint8_t _pad[6];
};

static_assert(sizeof(Request) == 32, "Go mirrors this layout");
static_assert(sizeof(Event) == 48, "Go mirrors this layout");

constexpr uint64_t RING_SLOTS = 4096;
using InRing = RingBuffer<Request, RING_SLOTS>;
using OutRing = RingBuffer<Event, RING_SLOTS>;

// Ring layout Go relies on: head @0, tail @CACHE_LINE, slots @2*CACHE_LINE.
static_assert(sizeof(InRing) == 2 * CACHE_LINE + RING_SLOTS * sizeof(Request));
static_assert(sizeof(OutRing) == 2 * CACHE_LINE + RING_SLOTS * sizeof(Event));

// MINI_EXCHANGE_DIR overrides it (compose uses /rings).
constexpr const char* DEFAULT_RING_DIR = "/tmp/mini_exchange";
