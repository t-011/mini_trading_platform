// Build:
//   serve:   c++ -std=c++17 -O2 matching_engine.cpp -o me && ./me --serve MINI [MORE...]
//   bench:   c++ -std=c++17 -O2 -DNDEBUG matching_engine.cpp -o me && ./me --bench
//   tests:   see matching_engine_test.cpp
//
// Env:
//   MINI_EXCHANGE_SPIN=1   busy-spin on empty rings. Only with a core to
//                          ourselves (compose cpuset).
//   MINI_EXCHANGE_DIR      rings dir, default /tmp/mini_exchange. <dir>/ready
//                          exists while serving (compose healthcheck).
//   MINI_EXCHANGE_WAL_DIR  WAL dir, default /tmp/mini_exchange_wal. Needs to
//                          be on disk, not with the tmpfs rings.
//
// TODO: more stocks on more cores needs pthread_setaffinity_np per thread
#include <algorithm>
#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <memory>
#include <random>
#include <string>
#include <thread>
#include <vector>
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#include "book.hpp"
#include "protocol.hpp"
#include "wal.hpp"

// --- serve: one thread per stock, each with its own Book and rings ---

bool busy_spin = false;

// pause/yield hint for spin loops
inline void cpu_relax() {
#if defined(__x86_64__)
    __builtin_ia32_pause();
#elif defined(__aarch64__)
    asm volatile("yield");
#endif
}

// Without busy_spin, sleep after a while so an idle stock doesn't hog a shared core.
void idle(int& spins) {
    cpu_relax();
    if (busy_spin) return;
    if (++spins < 1000) return;
    std::this_thread::sleep_for(std::chrono::microseconds(50));
}

std::atomic<bool> stopping{false};

void request_stop(int) { stopping.store(true); }

struct Stock {
    std::string symbol;
    InRing* in;
    OutRing* out;
    int wal;
    std::unique_ptr<Book> book;
};

void run_stock(Stock* stock) {
    uint64_t seq = 0;
    int spins = 0;

    auto emit = [&](Event event) {
        event.seq = ++seq;
        int full_spins = 0;
        // NOTE: if Go stops reading, this stock stalls here
        while (!stock->out->push(event) && !stopping.load(std::memory_order_relaxed)) idle(full_spins);
    };

    Request request;
    while (!stopping.load(std::memory_order_relaxed)) {
        if (!stock->in->pop(request)) { idle(spins); continue; }
        spins = 0;

        // log first: a request missing from the WAL would be lost on replay
        if (!append_wal(stock->wal, request)) {
            std::perror((stock->symbol + ": wal write").c_str());
            std::_Exit(1);
        }
        handle(*stock->book, request, emit);
    }
}

// Every book is replayed and every ring mapped before <dir>/ready is created.
void serve(const std::string& ring_dir, const std::string& wal_dir, int count, char** symbols) {
    mkdir(ring_dir.c_str(), 0700);
    mkdir(wal_dir.c_str(), 0700);
    std::string ready = ring_dir + "/ready";
    unlink(ready.c_str());

    std::vector<Stock> stocks;
    for (int index = 0; index < count; index++) {
        std::string symbol = symbols[index];

        std::string wal_path = wal_dir + "/" + symbol + ".wal";
        int wal = open_wal(wal_path);
        if (wal < 0) {
            std::perror(("can't open " + wal_path).c_str());
            std::exit(1);
        }
        auto book = std::make_unique<Book>();
        uint64_t replayed = replay_wal(wal, *book);
        std::printf("%s: replayed %llu requests from %s\n", symbol.c_str(), (unsigned long long)replayed, wal_path.c_str());

        std::string base = ring_dir + "/" + symbol;
        // New files instead of truncating: a Go process still mapping the old
        // ones would SIGBUS on a truncated file.
        unlink((base + ".in").c_str());
        unlink((base + ".out").c_str());
        InRing* in = map_ring<InRing>((base + ".in").c_str(), true);
        OutRing* out = map_ring<OutRing>((base + ".out").c_str(), true);
        if (!in || !out) {
            std::fprintf(stderr, "%s: can't map rings in %s\n", symbol.c_str(), ring_dir.c_str());
            std::exit(1);
        }
        std::printf("%s: serving on %s.{in,out}%s\n", symbol.c_str(), base.c_str(), busy_spin ? " (busy-spin)" : "");
        stocks.push_back(Stock{symbol, in, out, wal, std::move(book)});
    }

    struct sigaction on_stop {};
    on_stop.sa_handler = request_stop;
    sigaction(SIGTERM, &on_stop, nullptr);
    sigaction(SIGINT, &on_stop, nullptr);

    std::vector<std::thread> threads;
    for (Stock& stock : stocks) threads.emplace_back(run_stock, &stock);

    close(open(ready.c_str(), O_CREAT | O_WRONLY, 0600));
    std::printf("ready\n");
    std::fflush(stdout);

    for (auto& thread : threads) thread.join();
    for (Stock& stock : stocks) {
        fsync(stock.wal);
        close(stock.wal);
    }
    unlink(ready.c_str());
    std::printf("stopped\n");
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
    // NOTE: the two clock reads per op add ~20-40ns, ops/sec includes that
}

int main(int argc, char** argv) {
    if (argc > 2 && std::strcmp(argv[1], "--serve") == 0) {
        const char* spin = std::getenv("MINI_EXCHANGE_SPIN");
        busy_spin = spin && std::strcmp(spin, "1") == 0;
        const char* dir = std::getenv("MINI_EXCHANGE_DIR");
        const char* wal_dir = std::getenv("MINI_EXCHANGE_WAL_DIR");
        serve(dir && *dir ? dir : DEFAULT_RING_DIR,
              wal_dir && *wal_dir ? wal_dir : "/tmp/mini_exchange_wal", argc - 2, argv + 2);
        return 0;
    }
    if (argc > 1 && std::strcmp(argv[1], "--bench") == 0) {
        bench(1'000'000);
        return 0;
    }

    std::fprintf(stderr, "usage: %s --serve SYMBOL... | --bench\n", argv[0]);
    return 1;
}
