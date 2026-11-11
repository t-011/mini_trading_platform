# Recovery

The order books live in the engine's memory. If the engine dies, the books die with it, while Postgres still says those orders are open. A write-ahead log (WAL) per stock keeps the two in step.

## Write-ahead log

Each stock appends to `<MINI_EXCHANGE_WAL_DIR>/<SYMBOL>.wal`. A record is the raw 32-byte request, exactly as it came off the ring.

While serving, the engine handles each request in this order:

1. Pop the request from the inbound ring.
2. `write()` it to the WAL.
3. Match it and push the events.

If the write fails, the engine exits. It never matches a request it hasn't logged.

On startup, before it creates the rings, the engine replays each WAL through the same request handler, with the event output switched off. Go already received those events. Matching uses no clock and no randomness, so replay rebuilds the same book: the same levels, the same FIFO order and the same quantities. The engine logs rejected requests too, and replay rejects them again.

If the engine crashed in the middle of a write, the file ends in a partial record, and its size isn't a multiple of 32. The engine never handled that request, so replay truncates it.

## Durability

The engine calls `write()` per request but not `fsync()`. The data reaches the OS page cache, which survives the engine crashing or being killed with `SIGKILL`. An OS crash or power cut can lose the last few requests. On a clean shutdown (`SIGTERM`), the engine calls `fsync` once.

An `fsync` per request would cost tens of microseconds to milliseconds, depending on the disk. Real exchanges replicate the journal to a second machine instead of waiting on a local disk.

## Restarts

On startup the engine deletes the old ring files and creates new ones. Go checks about once a second whether its mapped file still has the same inode. When the inode changes, or Go sees a gap in `seq`, the API exits. Compose restarts it, and it dials the new rings, retrying for up to 10 seconds. Until it connects, orders on that stock return `503 exchange unavailable`.

After the restart, the engine's books match Postgres. Open orders stay open and can be cancelled.

To try it, rest an order, then kill the engine process. `docker compose kill` counts as a manual stop and Compose won't restart the container, so kill the process from the host's PID namespace instead:

```sh
docker run --rm --privileged --pid=host alpine:3.20 \
  kill -9 $(docker inspect -f '{{.State.Pid}}' mini_trading_platform-engine-1)
docker compose logs engine    # "MINI: replayed N requests from /wal/MINI.wal"
```

Order ids come from a Postgres sequence. Wipe the database and the WAL together (`docker compose down -v`), or the new ids collide with replayed orders.

## Known gaps

- **The WAL grows forever.** Replay time grows with it. The fix is a periodic snapshot of each book, then a fresh WAL.
- **Events can be lost between the engine and Postgres.** If the engine handles a request and dies before Go reads the events, or Go dies before writing them, the book and the database disagree. The fix is an outbound event journal that Go can re-read from a given `seq`.
- **An order can reach Postgres but never reach the engine.** If the engine is down or the call times out, the order stays `open` with its reservation held. The fix is a reconcile step on startup that asks the engine which open order ids it knows.
