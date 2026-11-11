// Build and run:
//   c++ -std=c++17 -Wall -Wextra -g -fsanitize=address,undefined matching_engine_test.cpp -o me.test && ./me.test
#undef NDEBUG  // the tests are asserts
#include <array>
#include <cassert>
#include <cstdio>
#include <random>
#include <utility>
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

Request new_order(uint64_t id, bool is_buy, int64_t price, int64_t qty) {
    return Request{id, price, qty, REQ_NEW, (uint8_t)is_buy, {}};
}

// Runs one request through handle() and returns every event it emitted.
std::vector<Event> handle_events(Book& book, const Request& request) {
    std::vector<Event> events;
    handle(book, request, [&](const Event& event) { events.push_back(event); });
    return events;
}

// Runs requests live (logging each to a temp WAL), then replays that WAL into
// a fresh book and checks both books hold the same orders in the same order.
void test_wal_replay_rebuilds_book() {
    char path[] = "/tmp/me_wal_test_XXXXXX";
    int wal = mkstemp(path);
    assert(wal >= 0);

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

// Zero or more FILLs, then exactly one terminal event.
void assert_contract(const std::vector<Event>& events) {
    assert(!events.empty());
    for (size_t index = 0; index + 1 < events.size(); index++) assert(events[index].type == EV_FILL);
    assert(events.back().type != EV_FILL);
}

void test_handle_event_contract() {
    Book book;

    // resting order: ACCEPTED with the full qty resting
    auto events = handle_events(book, new_order(1, false, 100, 5));
    assert(events.size() == 1 && events[0].type == EV_ACCEPTED && events[0].qty == 5);
    handle_events(book, new_order(2, false, 101, 5));

    // sweeps both asks, then rests 2: two FILLs for the taker, ACCEPTED with 2
    events = handle_events(book, new_order(3, true, 101, 12));
    assert(events.size() == 3);
    assert(events[0].type == EV_FILL && events[0].order_id == 3 && events[0].maker_id == 1);
    assert(events[0].price == 100 && events[0].qty == 5);
    assert(events[1].type == EV_FILL && events[1].order_id == 3 && events[1].maker_id == 2);
    assert(events[1].price == 101 && events[1].qty == 5);
    assert(events[2].type == EV_ACCEPTED && events[2].order_id == 3 && events[2].qty == 2);

    // fully filled taker: ACCEPTED with nothing resting
    events = handle_events(book, new_order(4, false, 101, 2));
    assert(events.size() == 2 && events[0].type == EV_FILL);
    assert(events[1].type == EV_ACCEPTED && events[1].qty == 0);

    // cancel of a resting order: CANCELED with the qty removed
    handle_events(book, new_order(5, true, 90, 7));
    events = handle_events(book, Request{5, 0, 0, REQ_CANCEL, 0, {}});
    assert(events.size() == 1 && events[0].type == EV_CANCELED && events[0].qty == 7);

    // rejects: one REJECTED with the reason, and the resting ask is untouched
    handle_events(book, new_order(11, false, 100, 5));
    std::vector<std::pair<Request, uint8_t>> rejects = {
        {new_order(6, true, 200, 0), R_BAD_ORDER},          // zero qty
        {new_order(7, true, 200, -3), R_BAD_ORDER},         // negative qty
        {new_order(8, true, 0, 1), R_BAD_ORDER},            // zero price
        {new_order(9, true, -100, 1), R_BAD_ORDER},         // negative price
        {Request{10, 200, 1, 99, 1, {}}, R_BAD_ORDER},      // unknown request type, would cross
        {Request{42, 0, 0, REQ_CANCEL, 0, {}}, R_UNKNOWN_ORDER},
        {Request{5, 0, 0, REQ_CANCEL, 0, {}}, R_UNKNOWN_ORDER},  // already canceled
    };
    for (const auto& [request, reason] : rejects) {
        events = handle_events(book, request);
        assert(events.size() == 1 && events[0].type == EV_REJECTED);
        assert(events[0].order_id == request.order_id && events[0].reason == reason);
    }
    assert(book.asks.at(100).front().id == 11 && book.asks.at(100).front().qty == 5);
    assert(book.bids.empty() && book.order_index.size() == 1);
}

// A deliberately naive book: resting orders in arrival order, scanned
// linearly for the best price. Book must produce exactly the same fills.
struct ReferenceBook {
    struct Resting {
        Order order;
        int64_t price;
    };
    std::vector<Resting> resting;  // arrival order

    std::vector<Fill> submit(Order order, int64_t price) {
        std::vector<Fill> fills;
        while (order.qty > 0) {
            // best crossing maker; strict comparison keeps the earliest at a tie
            int best = -1;
            for (int index = 0; index < (int)resting.size(); index++) {
                const Resting& maker = resting[index];
                if (maker.order.is_buy == order.is_buy) continue;
                bool crosses = order.is_buy ? maker.price <= price : maker.price >= price;
                bool better = best < 0 || (order.is_buy ? maker.price < resting[best].price
                                                        : maker.price > resting[best].price);
                if (crosses && better) best = index;
            }
            if (best < 0) break;

            Resting& maker = resting[best];
            int64_t quantity = std::min(order.qty, maker.order.qty);
            fills.push_back(Fill{order.id, maker.order.id, maker.price, quantity});
            order.qty -= quantity;
            maker.order.qty -= quantity;
            if (maker.order.qty == 0) resting.erase(resting.begin() + best);
        }
        if (order.qty > 0) resting.push_back({order, price});
        return fills;
    }

    int64_t cancel(uint64_t id) {
        for (auto maker = resting.begin(); maker != resting.end(); ++maker) {
            if (maker->order.id != id) continue;
            int64_t removed = maker->order.qty;
            resting.erase(maker);
            return removed;
        }
        return 0;
    }
};

// Random requests through handle() and the reference: same events, same order.
void test_book_matches_reference() {
    Book book;
    ReferenceBook reference;
    std::mt19937_64 rng(7);                            // fixed seed: failures reproduce
    uint64_t next_id = 1;

    for (int step = 0; step < 30000; step++) {
        if (next_id > 1 && rng() % 100 < 30) {         // ~30% cancels, of any earlier id
            uint64_t id = 1 + rng() % (next_id - 1);
            auto events = handle_events(book, Request{id, 0, 0, REQ_CANCEL, 0, {}});
            assert_contract(events);
            int64_t removed = reference.cancel(id);
            if (removed > 0) assert(events[0].type == EV_CANCELED && events[0].qty == removed);
            else assert(events[0].type == EV_REJECTED && events[0].reason == R_UNKNOWN_ORDER);
        } else {
            Order order{next_id++, rng() % 2 == 0, (int64_t)(1 + rng() % 20)};
            int64_t price = 95 + (int64_t)(rng() % 11);   // narrow range: lots of crossing
            auto events = handle_events(book, new_order(order.id, order.is_buy, price, order.qty));
            assert_contract(events);
            std::vector<Fill> expected = reference.submit(order, price);
            assert(events.size() == expected.size() + 1);
            int64_t resting = order.qty;
            for (size_t index = 0; index < expected.size(); index++) {
                assert(events[index].order_id == expected[index].taker);
                assert(events[index].maker_id == expected[index].maker);
                assert(events[index].price == expected[index].price);
                assert(events[index].qty == expected[index].qty);
                resting -= expected[index].qty;
            }
            assert(events.back().type == EV_ACCEPTED && events.back().qty == resting);
        }
        assert(book.order_index.size() == reference.resting.size());  // no leaked or lost ids
    }
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
    test_handle_event_contract();
    test_book_matches_reference();
    std::puts("all tests passed");
}
