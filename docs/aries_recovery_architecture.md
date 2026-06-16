# ARIES Recovery & Architecture Design

This document summarizes the architectural decisions, observations, and implementation strategies for our database's Write-Ahead Log (WAL) and ARIES Recovery manager.

## 1. Steal / No-Force Buffer Management
Our database officially operates on a **Steal / No-Force** policy. This is the fastest, highest-performance configuration for a database buffer pool:
- **Steal:** Our `BufferManager` is allowed to "steal" memory by evicting a dirty page to disk *before* its transaction commits. We natively support this because our `evict()` function writes any page with `PinCount == 0` to disk to free up space. **(This necessitates the UNDO phase).**
- **No-Force:** When a transaction commits, we do *not* force the B-Tree pages to disk. We only force (`fsync`) the WAL to disk. **(This necessitates the REDO phase).**

## 2. MVCC & The "No Before-Image" Undo Strategy
A brilliant architectural realization was made regarding Phase 2 (Multi-Version Concurrency Control):
Because our database will use MVCC, an `UPDATE` does not physically overwrite the old bytes. Instead, it inserts a brand new version of the key (e.g., `k_v2`), leaving the old version (`k_v1`) completely unharmed on disk. 

This drastically simplifies ARIES:
- We do **not** need to store massive "Before Images" in our log records.
- If a transaction aborts and we need to **UNDO**, we simply delete the newly inserted `k_v2` record. The `k_v1` record is already intact beneath it.

## 3. Fuzzy Checkpoints
To prevent scanning gigabytes of WAL logs on every boot, we use **Checkpoints**.
Crucially, we do **not** flush the physical 4KB contents of dirty pages into the checkpoint, as that would cause massive I/O spikes and defeat our No-Force policy. Instead, we use **Fuzzy Checkpoints**.

A `LogOpCheckpoint` record only contains:
1. **Active Transaction Table:** Who was running at the time of the checkpoint.
2. **Dirty Page Table:** A mapping of `PageID -> RecLSN`. 

The `RecLSN` (Recovery LSN) is simply the ID of the *very first log record* that dirtied the page since it was last flushed. The WAL naturally contains the actual data payload.

## 4. The 3 Phases of ARIES (Repeating History)
When recovering from a sudden power loss, the database boots up, finds the latest checkpoint, and executes three phases:

1. **Analysis Phase:** 
   Start at the Checkpoint and read forward. Reconstruct the Active Transaction Table and the Dirty Page Table to figure out exactly what was happening the millisecond the power died. Find the absolute smallest `RecLSN` in the Dirty Page Table (we will call this `MinLSN`).
2. **Redo Phase (Repeating History):** 
   Jump directly to `MinLSN` in the WAL and read to the end of the file. We blindly **REDO EVERYTHING**—even for transactions that didn't commit! 
   *Logic:* Read the physical page from disk. If the page's header `LSN` is less than the log's `LSN`, we re-apply the `INSERT`/`DELETE` payload from the log. By repeating history, we put the RAM exactly back into the physical state it was in before the crash.
3. **Undo Phase:** 
   Now that the memory state is restored, we look at our Active Transaction Table. We find all transactions that never logged a `COMMIT`, and we scan backward, undoing their operations (which simply means deleting their uncommitted MVCC records).

## 5. Log Truncation / Vacuuming
Because we track `RecLSN` in our Dirty Page Table, we know that any WAL record written *before* the smallest `RecLSN` (`MinLSN`) is fully persisted in the physical `.db` file. We can safely **truncate (delete)** all WAL logs before `MinLSN` to continuously free up disk space.
