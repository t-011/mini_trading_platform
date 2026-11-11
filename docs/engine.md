# Engine

The engine is C++17 with no dependencies beyond the standard library and POSIX.

| File | What's in it |
|---|---|
| `book.hpp` | the order book and the request handler |
| `wal.hpp` | the write-ahead log |
| `ring_buffer.hpp` | the shared-memory ring |
| `protocol.hpp` | the message layout shared with Go |
| `matching_engine.cpp` | the server and the benchmark |
| `matching_engine_test.cpp` | the tests |

```sh
./me --serve MINI [MORE...]   # serve these stocks
./me --bench                  # benchmark the book
./me.test                     # run the tests (built from matching_engine_test.cpp)
```

## Order book

Each stock has one `Book`:

```cpp
std::map<int64_t, std::list<Order>> asks;   // best ask = begin()
std::map<int64_t, std::list<Order>> bids;   // best bid = prev(end())
std::unordered_map<uint64_t, Loc> order_index;
```

The engine matches by price, then time. Each price level is a FIFO list, so the oldest order at the best price fills first. A trade happens at the resting (maker) order's price, so an aggressive order can only get a better price than its limit, never a worse one. Whatever doesn't fill rests on the book at its limit.

`order_index` maps an order id to its side, price and list iterator. A cancel jumps straight to the order and erases it, instead of scanning the level.

## Ring buffer

Go and the engine share two rings per stock: `<SYMBOL>.in` for requests and `<SYMBOL>.out` for events. Each ring is a file under `MINI_EXCHANGE_DIR`, mapped with `mmap(MAP_SHARED)` by both processes.

```cpp
alignas(CACHE_LINE) std::atomic<uint64_t> head;
alignas(CACHE_LINE) std::atomic<uint64_t> tail;
alignas(CACHE_LINE) T buf[N];
```

- The ring has one producer and one consumer, so it needs no locks and no compare-and-swap. The producer writes a slot, then publishes `head` with a release store. The consumer reads `head` with an acquire load, then reads the slot. The ordering guarantees the consumer sees the slot's contents.
- `head` and `tail` sit on separate cache lines, so the two sides don't invalidate each other's line on every update. `CACHE_LINE` is 128 bytes, which is the line size on Apple silicon. 64 is enough on most x86 machines.
- `N` must be a power of two, so the index wraps with `& (N - 1)` instead of a division. One slot always stays empty to tell "full" from "empty", so a ring holds `N - 1` messages. `N` is 4096.

Go reads the same memory through `sync/atomic` in `internal/exchange/ring.go`. Static asserts on both sides pin the layout: `head` at byte 0, `tail` at 128, slots at 256.

## Protocol

Messages are fixed-size structs of integers. Neither side parses or allocates.

| Request (32 bytes) | | Event (48 bytes) | |
|---|---|---|---|
| `order_id` | u64 | `seq` | u64 |
| `price` | i64 | `order_id` | u64 |
| `qty` | i64 | `maker_id` | u64 |
| `type` | NEW / CANCEL | `price` | i64 |
| `is_buy` | u8 | `qty` | i64 |
| | | `type` | ACCEPTED / FILL / CANCELED / REJECTED |
| | | `reason` | BAD_ORDER / UNKNOWN_ORDER |

Price is in cents. Quantity is in 1e-8 shares, the same precision as `NUMERIC(20,8)` in Postgres.

Each request gets zero or more `FILL` events and then exactly one final event. Because answers arrive in request order, Go pairs them with requests FIFO. `seq` counts up per ring, and Go exits if it sees a gap.

## Spinning and pinning

When a ring is empty, the engine spins 1000 times with a CPU pause hint (`pause` on x86, `yield` on ARM), then sleeps 50 µs. That keeps an idle engine from eating a shared core.

With `MINI_EXCHANGE_SPIN=1`, the engine never sleeps. It reacts faster and burns 100% of a core. Docker Compose turns this on and gives the engine its own core with `cpuset: "2"`. With one stock, one thread does all the work, so pinning the container pins the thread. Several stocks on several cores will need `pthread_setaffinity_np` per thread.

## Benchmark

`./me --bench` builds 1M operations from a fixed seed: about 70% limit orders (random side, quantity 1 to 100, price 90 to 110) and 30% cancels of earlier ids. It times each operation against the book.

```
1000000 ops in 0.071s -> 14149367 ops/sec, 555220 fills
latency ns: p50 42  p99 250  p99.9 916  max 455291
```

That run was native on an Apple M5. In Docker on the same machine, it's about 5M ops/sec.

The numbers cover the book only. They leave out the rings, the WAL write (about 1 to 2 µs per request) and everything on the Go side. The two clock reads per operation add 20 to 40 ns to each sample.

## Tests

`./me.test` runs the tests. Build them with `-fsanitize=address,undefined`. They cover partial fills, FIFO order within a level, sweeping several levels, cancels, and WAL replay. A fuzz test runs 30,000 random requests through `handle()` and through a deliberately naive reference book, and checks that both give the same events. Another test checks the event contract: zero or more fills, then one final event, and the right reject reasons. The Go tests in `internal/exchange` build the real engine and talk to it over real rings.
