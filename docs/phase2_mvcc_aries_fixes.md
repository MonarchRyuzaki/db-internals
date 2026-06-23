# Phase 2 MVCC & ARIES Recovery: Critical Bug Fixes

This document logs the four critical architectural flaws and bugs discovered after the initial implementation of Phase 2 (Transactions, MVCC, and ARIES Recovery), and details how they were resolved to ensure data integrity and strict Serializable isolation.

## 1. Zombie Transactions (Connection State Leak)

**The Flaw:**
In `server.go`, if a client disconnected abruptly (e.g., via `QUIT`, `EXIT`, or a dropped TCP connection) while actively in a transaction (`client.InTx == true`), the system failed to trigger a rollback.
**The Impact:**
The abandoned transaction remained in the `txnTable` with a status of `TXN_RUNNING` forever. Any rows modified by this zombie transaction remained permanently locked, causing all subsequent writes to those keys by other users to fail with a `write-write conflict`.
**The Fix:**
A `defer` block was added at the start of `handleConnection`. When the connection closes for any reason, the deferred function checks if `client.InTx` is true. If so, it automatically calls `mvccDB.Rollback(client.TxID)`, ensuring all locks are freed and writes are aborted.

## 2. Catastrophic Restart Data Loss (Aborted Inserts Became Deletes)

**The Flaw:**
During a transaction `Rollback` or the ARIES Undo phase during crash recovery, the system needed to undo a `LogOpInsert`. It accomplished this by calculating the inverse operation as `LogOpDelete`, which in our MVCC engine wrote a physical Tombstone (`KEY_DELETED_FLAG`) over the cell. However, because the in-memory `txnTable` is lost upon crash/restart, the system defaults unknown transactions to `TXN_COMMITED`. 
**The Impact:**
Upon restart, the Tombstone written by the aborted transaction was interpreted as a *committed* Tombstone. This permanently hid the valid, pre-existing committed data underneath it, resulting in catastrophic data loss for those keys.
**The Fix:**
We introduced a new log operation type, `LogOpUndoInsert`. When reversing an insert during `Rollback` or ARIES Undo, the system now issues `LogOpUndoInsert`. The physical redo function, `redoUpsertOnPage`, handles this by *physically omitting* the cell when reconstructing the page slot array, completely erasing it from existence rather than leaving a Tombstone.

## 3. Vacuum Destroys MVCC Isolation Guarantees

**The Flaw:**
The background `Vacuum` process (`vacuum.go`) aggressively deleted older versions of keys to save space, keeping only the absolute newest version. It did not verify whether active transactions might still need to read those older versions.
**The Impact:**
This broke Snapshot Isolation. A long-running transaction attempting to read a key updated after the transaction started would see the new version as invisible. `FindLatest` would then look for the older version, but because the vacuum had already deleted it, the engine would erroneously return a `key not found` error.
**The Fix:**
We introduced a global watermark by adding `GetMinActiveTxID()` to the `TransactionManager`, which returns the `TxID` of the oldest running transaction (`min_active`). In `vacuumLeaf`, we now inspect the overriding (next) version of a key. We only delete an older version if its overriding version's `TxID` is strictly older than `min_active` (`nextTxID < min_active`). This mathematically guarantees that no currently active transaction will ever need the version being deleted.

## 4. Unbounded Memory Leaks in TransactionManager

**The Flaw:**
The `TransactionManager` kept every single transaction's state in `txnTable` and its last LSN in `lsnTable` forever to allow fast visibility checks during `FindLatest`.
**The Impact:**
Under sustained workloads, these maps would grow indefinitely, eventually causing an Out of Memory (OOM) crash.
**The Fix:**
We tied memory cleanup directly to the disk vacuuming cycle. After the `Vacuum` process successfully sweeps the B-Tree for dead versions, it calls `PruneTables(min_active)`. This function scans the maps and safely deletes any completed or aborted transaction whose `TxID` is older than `min_active`. Since all physical data for those transactions has been safely consolidated by the Vacuum process, their in-memory state is no longer needed.
