# Roadmap

The project is a work in progress. This page lists what's done, what's next, and the shortcuts that need fixing.

## Done

- Price-time order book in C++, one thread per stock
- Lock-free shared-memory rings between Go and the engine
- Orders through the API, with cash and shares reserved up front
- Write-ahead log, with the books rebuilt on restart
- Docker Compose stack with the engine pinned to its own core
- Users, JWT auth, stocks, price bars, trade history, notes, portfolio

## Next

1. **Market-maker bots.** The books start empty, so nothing trades until someone sells. Bots will quote both sides around a reference price and give users something to trade against.

## Later

- **Market data service.** Stream trades and prices over WebSockets: engine trades for our stocks, outside data for public ones.
- **Backtesting service.** Users write a strategy, such as a 20-day moving average crossing a 50-day one, and run it on past prices to see returns, drawdown and Sharpe ratio. The work is heavy and bursty, so it runs on queued workers.
- **Real market data** for the read-only public stocks, from a prices API.
- **More stocks**, each on its own thread and core (`pthread_setaffinity_np`).

## Known shortcuts

Recovery (details in [recovery.md](recovery.md)):

- The WAL grows forever. Add book snapshots.
- Events can be lost between the engine and Postgres. Add an event journal that Go can re-read by `seq`.
- An order that never reached the engine stays open with its cash held. Add reconciliation on startup.

Broker:

- A restarted engine makes the API exit and restart. Re-dial in place instead.
- One mutex applies all engine answers. Use one per stock if throughput matters.
- Holding values use floating-point math, and the average cost is a weighted average rather than first-in-first-out lots.
- The portfolio reads cash and holdings in two queries, not one snapshot.

API:

- No rate limiting or request body size limit.
