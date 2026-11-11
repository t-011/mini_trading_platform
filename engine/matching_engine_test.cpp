// Build and run:
//   c++ -std=c++17 -g -fsanitize=address,undefined matching_engine_test.cpp -o me.test && ./me.test
#undef NDEBUG  // the tests are asserts
#include <array>
#include <cassert>
#include <cstdio>
#include <vector>
#include <sys/stat.h>
#include <unistd.h>

#include "book.hpp"
#include "wal.hpp"

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

// Runs requests live (logging each to a temp WAL), then replays that WAL into
// a fresh book and checks both books hold the same orders in the same order.
void test_wal_replay_rebuilds_book() {
    char path[] = "/tmp/me_wal_test_XXXXXX";
    int wal = mkstemp(path);
    assert(wal >= 0);

    auto new_order = [](uint64_t id, bool is_buy, int64_t price, int64_t qty) {
        return Request{id, price, qty, REQ_NEW, (uint8_t)is_buy, {}};
    };
    std::vector<Request> requests = {
        new_order(1, false, 100, 10),
        new_order(2, false, 100, 5),
        new_order(3, true, 99, 7),
        new_order(4, true, 100, 4),     // fills 4 of order 1
        Request{2, 0, 0, REQ_CANCEL, 0, {}},
        new_order(5, false, 101, 3),
        new_order(6, true, 0, 1),       // bad price
        Request{42, 0, 0, REQ_CANCEL, 0, {}},
    };
    Book live;
    for (const Request& request : requests) {
        assert(append_wal(wal, request));
        handle(live, request, [](const Event&) {});
    }

    Book replayed;
    assert(replay_wal(wal, replayed) == requests.size());

    // price, id, qty of every resting order, in book order
    auto dump = [](const Book& book) {
        std::vector<std::array<int64_t, 3>> orders;
        for (auto* side : {&book.asks, &book.bids})
            for (auto& [price, level] : *side)
                for (const Order& order : level) orders.push_back({price, (int64_t)order.id, order.qty});
        return orders;
    };
    assert(dump(live) == dump(replayed));
    assert(replayed.asks.at(100).front().qty == 6);
    assert(replayed.cancel(3) == 7);

    close(wal);
    unlink(path);
}

void test_wal_drops_partial_record() {
    char path[] = "/tmp/me_wal_test_XXXXXX";
    int wal = mkstemp(path);
    assert(wal >= 0);

    assert(append_wal(wal, Request{1, 100, 5, REQ_NEW, 0, {}}));
    char junk[10] = {};
    assert(write(wal, junk, sizeof junk) == sizeof junk);

    Book book;
    assert(replay_wal(wal, book) == 1);
    assert(book.asks.at(100).front().id == 1);
    struct stat info;
    fstat(wal, &info);
    assert(info.st_size == sizeof(Request));

    close(wal);
    unlink(path);
}

int main() {
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
    test_wal_replay_rebuilds_book();
    test_wal_drops_partial_record();
    std::puts("all tests passed");
}
