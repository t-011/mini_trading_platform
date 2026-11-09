#include <algorithm>
#include <cassert>
#include <cstdint>
#include <cstdio>
#include <iterator>
#include <map>
#include <vector>
#include <list>
#include <unordered_map>

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
std::unordered_map<uint64_t, Loc> order_index;

std::map<int64_t, std::list<Order>> asks;
std::map<int64_t, std::list<Order>> bids;

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

bool cancel(uint64_t id) {
    auto it = order_index.find(id);
    
    if (it == order_index.end()) return false;

    Loc& loc = it->second;

    auto& book = loc.is_buy ? bids : asks;

    auto level = book.find(loc.price);
    level->second.erase(loc.it);
    if (level->second.empty()) book.erase(level);

    order_index.erase(id);

    return true;
}

// --- tests: plain functions + assert. A failed assert aborts with file:line. ---

void reset() { asks.clear(); bids.clear(); order_index.clear(); }

void test_rests_when_book_empty() {
    reset();
    auto fills = submit({1, false, 10}, 100);     // sell 10 @ 100
    assert(fills.empty());
    assert(asks.at(100).front().qty == 10);       // resting on the ask side
}

void test_partial_fill_at_maker_price() {
    reset();
    submit({1, false, 10}, 100);                  // maker: sell 10 @ 100
    auto fills = submit({2, true, 4}, 101);       // taker: buy 4 @ 101
    assert(fills.size() == 1);
    assert(fills[0].taker == 2 && fills[0].maker == 1);
    assert(fills[0].price == 100);                // maker's price, not 101
    assert(fills[0].qty == 4);
    assert(asks.at(100).front().qty == 6);        // remainder still resting
    assert(bids.empty());                         // taker fully filled, didn't rest
}

void test_time_priority_same_level() {
    reset();
    submit({1, false, 5}, 100);                   // first in line
    submit({2, false, 5}, 100);                   // second in line
    auto fills = submit({3, true, 7}, 100);
    assert(fills.size() == 2);
    assert(fills[0].maker == 1 && fills[0].qty == 5);
    assert(fills[1].maker == 2 && fills[1].qty == 2);
    assert(asks.at(100).front().id == 2 && asks.at(100).front().qty == 3);
}

void test_sweeps_levels_best_first() {
    reset();
    submit({1, false, 5}, 101);
    submit({2, false, 5}, 100);                   // better ask, added later
    auto fills = submit({3, true, 8}, 101);
    assert(fills.size() == 2);
    assert(fills[0].price == 100 && fills[0].qty == 5);
    assert(fills[1].price == 101 && fills[1].qty == 3);
    assert(asks.count(100) == 0);                 // emptied level got erased
}

void test_no_cross_rests() {
    reset();
    submit({1, false, 5}, 100);
    auto fills = submit({2, true, 5}, 99);
    assert(fills.empty());
    assert(bids.at(99).front().id == 2);
    assert(asks.at(100).front().qty == 5);        // untouched
}

void test_sell_hits_highest_bid() {
    reset();
    submit({1, true, 5}, 99);
    submit({2, true, 5}, 101);                    // best bid
    auto fills = submit({3, false, 5}, 98);
    assert(fills.size() == 1);
    assert(fills[0].maker == 2 && fills[0].price == 101);
}

void test_cancel_resting() {
    reset();
    submit({1, false, 5}, 100);
    assert(cancel(1));
    assert(asks.empty());                         // emptied level erased
    assert(submit({2, true, 5}, 100).empty());    // nothing left to hit
}

void test_cancel_unknown() {
    reset();
    assert(!cancel(42));
}

void test_cancel_after_full_fill() {
    reset();
    submit({1, false, 5}, 100);
    submit({2, true, 5}, 100);                    // fills 1 completely
    assert(!cancel(1));
    assert(!cancel(2));                           // taker never rested
}

void test_cancel_partial_remainder() {
    reset();
    submit({1, false, 10}, 100);
    submit({2, true, 4}, 100);                    // 6 left on order 1
    assert(cancel(1));
    assert(asks.empty());
}

void test_cancel_middle_keeps_priority() {
    reset();
    submit({1, false, 5}, 100);
    submit({2, false, 5}, 100);
    submit({3, false, 5}, 100);
    assert(cancel(2));
    auto fills = submit({4, true, 10}, 100);
    assert(fills.size() == 2);
    assert(fills[0].maker == 1 && fills[1].maker == 3);
    assert(asks.empty());
}

void test_cancel_twice() {
    reset();
    submit({1, true, 5}, 99);
    assert(cancel(1));
    assert(!cancel(1));
    assert(bids.empty());
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
    std::puts("all tests passed");
}
