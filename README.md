# DB-Internals: High-Performance B-Tree Storage Engine

> **Note:** This is an educational project built from scratch while studying the concepts in the book *Database Internals* by Alex Petrov. The goal was to understand how the lowest physical layers of a database actually work under the hood. While it achieves surprisingly high performance, it is intended as a learning exercise rather than an industry-standard, production-ready system.

A lightning-fast, disk-backed Key-Value storage engine built entirely from scratch in Go. This project implements the physical storage layer found at the bottom of modern databases (like PostgreSQL, MySQL, and SQLite), focusing heavily on concurrency, memory management, and disk I/O optimization.

## 🚀 Features

- **Disk-Backed B-Tree**: A persistent B-Tree structure where data is stored in fixed 4KB pages.
- **Overflow Pages**: Automatically chunks massive payloads (like 10KB+ JSON objects) across linked overflow pages on disk.
- **Page-Level Latching (Latch Crabbing)**: Highly concurrent `sync.RWMutex` locking mechanism that allows hundreds of goroutines to traverse and mutate the tree simultaneously without global locks.
- **Zero-Copy Deserialization**: `Find` operations execute with exactly **0 memory allocations** (`1 alloc/op` due to testing overhead), preventing Garbage Collector pauses.
- **Buffer Pool Manager**: Intelligent memory caching layer that prevents thrashing the physical disk.
- **Background Vacuuming**: Deletions use *Tombstones*. A background goroutine periodically runs a Depth-First Search (DFS) Latch Crabbing traversal to drop tombstones and rewrite pages.
- **O(1) Space Reclamation**: Freed pages and dead overflow chains are instantly pushed to a persistent Free Page Stack (tracked by the `MetaPage`) and immediately reused during the next allocation, preventing disk bloat.
- **Write-Ahead Logging (WAL)**: Custom physiological logging mechanism utilizing raw byte offsets for continuous disk syncing without blocking page evictions.
- **ARIES Recovery System**: Full implementation of the ARIES protocol including Fuzzy Checkpointing, Analysis, and Redo (Repeating History). Guarantees absolute ACID durability and zero data loss against sudden power failures.

## ⚡ Performance 

The engine was heavily benchmarked on an AMD Ryzen 5 processor. The table below illustrates the raw speeds of the Phase 1 in-memory structure versus the Phase 2 implementation, which includes full MVCC Transaction Isolation and absolute ARIES WAL durability.

| Operation | Phase 1 (No MVCC/WAL) | Phase 2 (Full MVCC & ARIES WAL) | Difference |
| :--- | :--- | :--- | :--- |
| **Reads (Find)** | `2,060 ns/op` (2.0 µs) | `2,112 ns/op` (2.1 µs) | **Identical!** (No read penalty) |
| **Sequential Writes** | `16,927 ns/op` (0.01 ms)| `2,834,512 ns/op` (2.8 ms)| **~167x Slower** |
| **Random Writes** | `10,963 ns/op` (0.01 ms)| `2,623,984 ns/op` (2.6 ms)| **~239x Slower** |
| **Parallel Read/Write**| `1,882 ns/op` (1.8 µs) | `5,038,702 ns/op` (5.0 ms)| **~2600x Slower** |
| **E-Commerce Workload**| `3,913 ns/op` (3.9 µs) | `1,734,126 ns/op` (1.7 ms)| **~440x Slower** |

### ⚖️ The ACID Tradeoff
Seeing writes go from microseconds to milliseconds might look like a massive regression at first glance, but **this is exactly what we expect from a real database!** 

In Phase 1, a "write" was basically just throwing bytes into a cached memory page. In Phase 2, every single write must:
1. **Acquire a Transaction ID** globally from the `TransactionManager`.
2. **Serialize an ARIES Log Record** representing the physical undo/redo operation.
3. **Fsync to Disk:** Most importantly, we execute a raw `fsync()` system call to flush the Write-Ahead Log to the physical hard drive platter to guarantee data survives sudden power failures.
4. **Build MVCC Keys:** Generating multi-versioned timestamps (`[UserKey]\x00[TxID]`) so other transactions can read older versions safely.

We successfully traded raw, dangerous speed for absolute **ACID Durability** (crash-proofing) and **Serializable Isolation** (safe concurrency), precisely replicating the behavior of production databases like PostgreSQL and MySQL.

**The incredible win:** Despite having to navigate complex MVCC timestamps, reconstruct older versions of keys, and interact with the `TransactionManager` visibility map, our reads stayed at **2.1 microseconds**! This proves our Zero-Copy Deserialization and Buffer Pool caching mechanisms are flawlessly holding up under the heavy weight of MVCC!

> 📖 **Read the Case Study:** Check out [res/High Allocs Case Study.md](./res/High%20Allocs%20Case%20Study.md) to see how we dropped read allocations from 237 down to 1 using Go Escape Analysis!

## 💻 Interactive CLI (REPL)

You can interact directly with the storage engine using the built-in KV-store REPL. 

### Starting the Database
```bash
go run ./cmd/db/main.go
```

### Commands
Once the database initializes, you can execute raw physical commands:

```text
db> SET user_1 "John Doe"
OK (took 34.5µs)

db> GET user_1
"John Doe" (took 2.1µs)

db> DELETE user_1
OK (took 15.2µs)

db> EXIT
Shutting down database...
```

*(Note: The background Vacuum process runs every 10 seconds and will automatically clean up the tombstone left by the `DELETE` command).*

## 📚 Architecture Deep-Dives
If you want to learn more about how the internals of this database work, read the detailed architectural design documents:
- [Mutation & Vacuum Architecture](./docs/mutation_and_vacuum_plan.md)
- [Zero-Copy Optimization Case Study](./res/High%20Allocs%20Case%20Study.md)
- [ARIES Recovery Architecture](./docs/aries_recovery_architecture.md)
- [ARIES Meta Page Bug Fix](./docs/aries_meta_page_recovery_bug.md)