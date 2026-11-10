#pragma once
#include <atomic>
#include <cstddef>
#include <cstdint>
#include <new>
#include <type_traits>

constexpr uint64_t CACHE_LINE = 128; // For apple silicon. CHANGE DEPENDING ON cpu ARCHITECTURE.

// One producer thread, one consumer thread. Holds up to N-1 items.
template <typename T, uint64_t N>
class RingBuffer {
    static_assert(N > 1 && (N & (N - 1)) == 0, "N must be a power of two");
    static_assert(std::is_trivially_copyable_v<T>, "slots are raw bytes shared with Go");
    static_assert(std::atomic<uint64_t>::is_always_lock_free, "atomics must be lock-free to work across processes");

    alignas(CACHE_LINE) std::atomic<uint64_t> head{0};
    alignas(CACHE_LINE) std::atomic<uint64_t> tail{0};
    alignas(CACHE_LINE) T buf[N];

public:
    bool push(const T& item) {
        uint64_t h = head.load(std::memory_order_relaxed);
        uint64_t next = (h + 1) & (N - 1);
        if (next == tail.load(std::memory_order_acquire))
            return false;
        buf[h] = item;
        head.store(next, std::memory_order_release);
        return true;
    }

    bool pop(T& out) {
        uint64_t t = tail.load(std::memory_order_relaxed);
        if (t == head.load(std::memory_order_acquire))
            return false;
        out = buf[t];
        tail.store((t + 1) & (N - 1), std::memory_order_release);
        return true;
    }
};

