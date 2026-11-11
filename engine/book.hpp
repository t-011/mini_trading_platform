#pragma once
// One stock's order book and the request handler that drives it. Shared by
// the server, the WAL replay and the tests.
#include <algorithm>
#include <cstdint>
#include <iterator>
#include <list>
#include <map>
#include <unordered_map>
#include <vector>

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

// Both live serving and WAL replay use this, so a replay ends up with the
// same book.
template <typename Emit>
void handle(Book& book, const Request& request, Emit&& emit) {
    auto reject = [&](uint8_t reason) {
        emit(Event{0, request.order_id, 0, 0, 0, EV_REJECTED, reason, {}});
    };

    if (request.type == REQ_NEW) {
        if (request.qty <= 0 || request.price <= 0) {
            reject(R_BAD_ORDER);
            return;
        }

        Order order{request.order_id, request.is_buy == 1, request.qty};

        for (const Fill& fill : book.submit(order, request.price)) {
            emit(Event{0, fill.taker, fill.maker, fill.price, fill.qty, EV_FILL, R_NONE, {}});
        }

        auto rested = book.order_index.find(request.order_id);
        int64_t resting = rested == book.order_index.end() ? 0 : rested->second.it->qty;
        emit(Event{0, request.order_id, 0, request.price, resting, EV_ACCEPTED, R_NONE, {}});

    } else if (request.type == REQ_CANCEL) {
        int64_t removed = book.cancel(request.order_id);
        if (removed == 0) reject(R_UNKNOWN_ORDER);
        else emit(Event{0, request.order_id, 0, 0, removed, EV_CANCELED, R_NONE, {}});
    } else {
        reject(R_BAD_ORDER);
    }
}
