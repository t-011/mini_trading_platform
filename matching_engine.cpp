#include <algorithm>
#include <cassert>
#include <cstdint>
#include <cstdio>
#include <iterator>
#include <map>
#include <deque>
#include <vector>

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

std::map<int64_t, std::deque<Order>> asks;
std::map<int64_t, std::deque<Order>> bids;

std::vector<Fill> submit(Order order, int64_t price) {
    auto& side = order.is_buy ? bids : asks;
    auto& op = !order.is_buy ? bids : asks;

    std::vector<Fill> fills;

    while (order.qty > 0 && !op.empty()) {
        auto best = order.is_buy ? op.begin() : std::prev(op.end());    

        bool crosses = order.is_buy ? best->first <= price
                                    : best->first >= price;
        
        if (!crosses) break;

        auto level = order.is_buy ? op.begin() : std::prev(op.end());
        auto& maker = level->second.front();

        int64_t quantity = std::min(order.qty, maker.qty);

        fills.push_back(Fill{order.id, maker.id, level->first, quantity}); // level->first == level_price

        order.qty -= quantity;
        maker.qty -= quantity;

        if (maker.qty == 0) level->second.pop_front();
        if (level->second.empty()) op.erase(level);
    }

    if (order.qty > 0) {
        side[price].push_back(order);
    }

    return fills;
}

// --- tests: plain functions + assert. A failed assert aborts with file:line. ---

void reset() { asks.clear(); bids.clear(); }

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

int main() {
    test_rests_when_book_empty();
    test_partial_fill_at_maker_price();
    test_time_priority_same_level();
    test_sweeps_levels_best_first();
    test_no_cross_rests();
    test_sell_hits_highest_bid();
    std::puts("all tests passed");
}
