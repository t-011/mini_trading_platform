#pragma once
// Messages between Go and the engine. internal/exchange/protocol.go has the
// same layout, so change both together.
//
// Each stock has two rings: <SYMBOL>.in (Go -> engine, Request) and
// <SYMBOL>.out (engine -> Go, Event). The ring tells you the stock, so
// messages have no symbol field.
//
// A Request gets zero or more FILLs and then one ACCEPTED, CANCELED or
// REJECTED, in order, so Go can match answers to requests FIFO.
//
// Price is in cents, qty in 1e-8 shares (same as NUMERIC(20,8)).

#include <cstdint>
#include "ring_buffer.hpp"

enum ReqType : uint8_t { REQ_NEW = 1, REQ_CANCEL = 2 };
enum EvType : uint8_t { EV_ACCEPTED = 1, EV_FILL = 2, EV_CANCELED = 3, EV_REJECTED = 4 };
enum Reason : uint8_t { R_NONE = 0, R_BAD_ORDER = 1, R_UNKNOWN_ORDER = 2 };

// Largest fields first so the compiler adds no padding.
struct Request {
    uint64_t order_id;  // assigned by Go
    int64_t price;      // NEW only
    int64_t qty;        // NEW only
    uint8_t type;       // ReqType
    uint8_t is_buy;     // NEW only: 1 buy, 0 sell
    uint8_t _pad[6];
};

struct Event {
    uint64_t seq;       // per ring, Go checks for gaps
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
