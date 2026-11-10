// Build:
//   tests:   c++ -std=c++17 -g -fsanitize=address,undefined matching_engine.cpp -o me && ./me
//   bench:   c++ -std=c++17 -O2 -DNDEBUG matching_engine.cpp -o me && ./me --bench
//   serve:   c++ -std=c++17 -O2 matching_engine.cpp -o me && ./me --serve MINI [MORE...]
#include <algorithm>
#include <cassert>
#include <cstdint>
#include <chrono>
#include <cstdio>
#include <cstring>
#include <random>
#include <iterator>
#include <map>
#include <vector>
#include <list>
#include <unordered_map>
#include <string>
#include <thread>
#include <sys/stat.h>

#include "protocol.hpp"

struct Order {
    uint64_t id;
    bool is_buy;
    int64_t qty;
};

struct Fill {
    uint64_t taker;
    uint64_t maker;
    int64_t price;
    int64_t qty;
};

struct Loc {
    bool is_buy;
    int64_t price;
    std::list<Order>::iterator it;
};

// One stock's order book. Only ever touched by that stock's thread: no locks.
struct Book {
    std::map<int64_t, std::list<Order>> asks;
    std::map<int64_t, std::list<Order>> bids;
    std::unordered_map<uint64_t, Loc> order_index;

    std::vector<Fill> submit(Order order, int64_t price) {
        auto& side = order.is_buy ? bids : asks;
        auto& op = !order.is_buy ? bids : asks;

        std::vector<Fill> fills;

        while (order.qty > 0 && !op.empty()) {
            auto level = order.is_buy ? op.begin() : std::prev(op.end());

            bool crosses = order.is_buy ? level->first <= price
                                        : level->first >= price;

            if (!crosses) break;

            auto& maker = level->second.front();

            int64_t quantity = std::min(order.qty, maker.qty);

            fills.push_back(Fill{order.id, maker.id, level->first, quantity}); // level->first == level_price

            order.qty -= quantity;
            maker.qty -= quantity;

            if (maker.qty == 0) {
                order_index.erase(maker.id);
                level->second.pop_front();
            }

            if (level->second.empty()) op.erase(level);
        }

        if (order.qty > 0) {
            auto& level = side[price];
            level.push_back(order);
            order_index[order.id] = {order.is_buy, price, std::prev(level.end())};
        }

        return fills;
    }

    // Returns the qty removed (Go releases that much of the reservation), 0 if not resting.
    int64_t cancel(uint64_t id) {
        auto it = order_index.find(id);

        if (it == order_index.end()) return 0;

        Loc& loc = it->second;

        auto& book = loc.is_buy ? bids : asks;

        auto level = book.find(loc.price);
        int64_t removed = loc.it->qty;
        level->second.erase(loc.it);
        if (level->second.empty()) book.erase(level);

        order_index.erase(it);

        return removed;
    }
};

// --- serve: one thread per stock, each owning its Book and its two rings. ---

// Spin briefly, then sleep, so an idle stock doesn't pin a whole core.
void idle(int& spins) {
    if (++spins < 1000) return;
    std::this_thread::sleep_for(std::chrono::microseconds(50));
}

void run_stock(const std::string& symbol) {
    std::string base = std::string(RING_DIR) + "/" + symbol;
    InRing* in = map_ring<InRing>((base + ".in").c_str(), true);
    OutRing* out = map_ring<OutRing>((base + ".out").c_str(), true);
    if (!in || !out) {
        std::fprintf(stderr, "%s: can't map rings in %s\n", symbol.c_str(), RING_DIR);
        std::exit(1);
    }

    Book book;
    uint64_t seq = 0;
    int spins = 0;

    auto emit = [&](Event event) {
        event.seq = ++seq;
        int full_spins = 0;
        while (!out->push(event)) idle(full_spins);  // ponytail: blocks the book if Go stops reading
    };
    auto reject = [&](uint64_t id, uint8_t reason) {
        emit(Event{0, id, 0, 0, 0, EV_REJECTED, reason, {}});
    };

    std::printf("%s: serving on %s.{in,out}\n", symbol.c_str(), base.c_str());
    std::fflush(stdout);

    Request request;
    while (true) {
        if (!in->pop(request)) { idle(spins); continue; }
        spins = 0;

        if (request.type == REQ_NEW) {
            if (request.qty <= 0 || request.price <= 0) { reject(request.order_id, R_BAD_ORDER); continue; }
            Order order{request.order_id, request.is_buy == 1, request.qty};
            for (const Fill& fill : book.submit(order, request.price))
                emit(Event{0, fill.taker, fill.maker, fill.price, fill.qty, EV_FILL, R_NONE, {}});
            auto rested = book.order_index.find(request.order_id);
            int64_t resting = rested == book.order_index.end() ? 0 : rested->second.it->qty;
            emit(Event{0, request.order_id, 0, request.price, resting, EV_ACCEPTED, R_NONE, {}});
        } else if (request.type == REQ_CANCEL) {
            int64_t removed = book.cancel(request.order_id);
            if (removed == 0) reject(request.order_id, R_UNKNOWN_ORDER);
            else emit(Event{0, request.order_id, 0, 0, removed, EV_CANCELED, R_NONE, {}});
        } else {
            reject(request.order_id, R_BAD_ORDER);
        }
    }
}

void serve(int count, char** symbols) {
    mkdir(RING_DIR, 0700);  // EEXIST is fine
    std::vector<std::thread> threads;
    for (int index = 0; index < count; index++) threads.emplace_back(run_stock, std::string(symbols[index]));
    for (auto& thread : threads) thread.join();
}

// --- tests: plain functions + assert. A failed assert aborts with file:line. ---

void test_rests_when_book_empty() {
    Book book;
    auto fills = book.submit({1, false, 10}, 100);     // sell 10 @ 100
    assert(fills.empty());
    assert(book.asks.at(100).front().qty == 10);       // resting on the ask side
}

void test_partial_fill_at_maker_price() {
    Book book;
    book.submit({1, false, 10}, 100);                  // maker: sell 10 @ 100
    auto fills = book.submit({2, true, 4}, 101);       // taker: buy 4 @ 101
    assert(fills.size() == 1);
    assert(fills[0].taker == 2 && fills[0].maker == 1);
    assert(fills[0].price == 100);                // maker's price, not 101
    assert(fills[0].qty == 4);
    assert(book.asks.at(100).front().qty == 6);        // remainder still resting
    assert(book.bids.empty());                         // taker fully filled, didn't rest
}

void test_time_priority_same_level() {
    Book book;
    book.submit({1, false, 5}, 100);                   // first in line
    book.submit({2, false, 5}, 100);                   // second in line
    auto fills = book.submit({3, true, 7}, 100);
    assert(fills.size() == 2);
    assert(fills[0].maker == 1 && fills[0].qty == 5);
    assert(fills[1].maker == 2 && fills[1].qty == 2);
    assert(book.asks.at(100).front().id == 2 && book.asks.at(100).front().qty == 3);
}

void test_sweeps_levels_best_first() {
    Book book;
    book.submit({1, false, 5}, 101);
    book.submit({2, false, 5}, 100);                   // better ask, added later
    auto fills = book.submit({3, true, 8}, 101);
    assert(fills.size() == 2);
    assert(fills[0].price == 100 && fills[0].qty == 5);
    assert(fills[1].price == 101 && fills[1].qty == 3);
    assert(book.asks.count(100) == 0);                 // emptied level got erased
}

void test_no_cross_rests() {
    Book book;
    book.submit({1, false, 5}, 100);
    auto fills = book.submit({2, true, 5}, 99);
    assert(fills.empty());
    assert(book.bids.at(99).front().id == 2);
    assert(book.asks.at(100).front().qty == 5);        // untouched
}

void test_sell_hits_highest_bid() {
    Book book;
    book.submit({1, true, 5}, 99);
    book.submit({2, true, 5}, 101);                    // best bid
    auto fills = book.submit({3, false, 5}, 98);
    assert(fills.size() == 1);
    assert(fills[0].maker == 2 && fills[0].price == 101);
}

void test_cancel_resting() {
    Book book;
    book.submit({1, false, 5}, 100);
    assert(book.cancel(1));
    assert(book.asks.empty());                         // emptied level erased
    assert(book.submit({2, true, 5}, 100).empty());    // nothing left to hit
}

void test_cancel_unknown() {
    Book book;
    assert(!book.cancel(42));
}

void test_cancel_after_full_fill() {
    Book book;
    book.submit({1, false, 5}, 100);
    book.submit({2, true, 5}, 100);                    // fills 1 completely
    assert(!book.cancel(1));
    assert(!book.cancel(2));                           // taker never rested
}

void test_cancel_partial_remainder() {
    Book book;
    book.submit({1, false, 10}, 100);
    book.submit({2, true, 4}, 100);                    // 6 left on order 1
    assert(book.cancel(1));
    assert(book.asks.empty());
}

void test_cancel_middle_keeps_priority() {
    Book book;
    book.submit({1, false, 5}, 100);
    book.submit({2, false, 5}, 100);
    book.submit({3, false, 5}, 100);
    assert(book.cancel(2));
    auto fills = book.submit({4, true, 10}, 100);
    assert(fills.size() == 2);
    assert(fills[0].maker == 1 && fills[1].maker == 3);
    assert(book.asks.empty());
}

void test_cancel_twice() {
    Book book;
    book.submit({1, true, 5}, 99);
    assert(book.cancel(1));
    assert(!book.cancel(1));
    assert(book.bids.empty());
}

// --- benchmark: ./me --bench. Build with -O2 -DNDEBUG, no sanitizers. ---

struct Op {
    bool is_cancel;
    Order order;       // used when !is_cancel
    int64_t price;
    uint64_t cancel_id;
};

void bench(uint64_t n) {
    Book book;
    std::mt19937_64 rng(42);                      // fixed seed: same workload every run
    std::vector<Op> ops;
    ops.reserve(n);
    uint64_t next_id = 1;
    for (uint64_t i = 0; i < n; i++) {
        if (next_id > 1 && rng() % 100 < 30) {    // ~30% cancels of some earlier id
            ops.push_back({true, {}, 0, 1 + rng() % (next_id - 1)});
        } else {
            Order order{next_id++, rng() % 2 == 0, (int64_t)(1 + rng() % 100)};
            ops.push_back({false, order, 90 + (int64_t)(rng() % 21), 0});  // 90..110
        }
    }

    std::vector<int64_t> latencies(n);                  // ns per op
    uint64_t fills = 0;
    using clk = std::chrono::steady_clock;
    auto start = clk::now();
    for (uint64_t i = 0; i < n; i++) {
        auto op_start = clk::now();
        if (ops[i].is_cancel) book.cancel(ops[i].cancel_id);
        else fills += book.submit(ops[i].order, ops[i].price).size();
        latencies[i] = std::chrono::duration_cast<std::chrono::nanoseconds>(clk::now() - op_start).count();
    }
    double secs = std::chrono::duration<double>(clk::now() - start).count();

    std::sort(latencies.begin(), latencies.end());
    auto percentile = [&](double fraction) { return latencies[(uint64_t)(fraction * (n - 1))]; };
    std::printf("%llu ops in %.3fs -> %.0f ops/sec, %llu fills\n",
                (unsigned long long)n, secs, n / secs, (unsigned long long)fills);
    std::printf("latency ns: p50 %lld  p99 %lld  p99.9 %lld  max %lld\n",
                (long long)percentile(0.50), (long long)percentile(0.99), (long long)percentile(0.999), (long long)latencies.back());
    // ponytail: per-op clock reads add ~20-40ns each; ops/sec includes that overhead
}

int main(int argc, char** argv) {
    if (argc > 2 && std::strcmp(argv[1], "--serve") == 0) {
        serve(argc - 2, argv + 2);
        return 0;
    }
    if (argc > 1 && std::strcmp(argv[1], "--bench") == 0) {
        bench(1'000'000);
        return 0;
    }

    test_rests_when_book_empty();
    test_partial_fill_at_maker_price();
    test_time_priority_same_level();
    test_sweeps_levels_best_first();
    test_no_cross_rests();
    test_sell_hits_highest_bid();
    test_cancel_resting();
    test_cancel_unknown();
    test_cancel_after_full_fill();
    test_cancel_partial_remainder();
    test_cancel_middle_keeps_priority();
    test_cancel_twice();
    std::puts("all tests passed");
}
