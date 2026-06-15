# Write-Ahead Logging & ARIES Recovery Plan

This document outlines the architecture for Phase 1 of our Database Internals project: Durability and Crash Recovery.

## 1. Physiological Logging
To ensure we can recover from a crash without destroying disk space, we use **Physiological Logging** (a hybrid of physical and logical logging used by ARIES).

Instead of storing the entire 4KB page for every tiny update (pure physical), or storing the raw SQL/KV command without page context (pure logical), we store the **Physical Page ID** and the **Logical Operation** applied to it.

**Example Log Record:**
`[LSN: 10] [TxnID: 1] [PageID: 5] [Op: INSERT] [Key: X] [Value: Y]`

- **To REDO:** We read the log, fetch `Page 5` into memory, and execute the `INSERT` logic.
- **To UNDO:** We read the log backwards, fetch `Page 5`, and execute the exact inverse `DELETE` logic.

## 2. The "Repeating History" Paradigm
If the database crashes *while* it is trying to Undo an aborted transaction, we could corrupt the database if we simply delete logs. 

ARIES prevents this using the **Repeating History** rule:
1. **Redo Phase:** We replay the entire WAL forward, exactly as it happened, bringing the B-Tree back to the exact millisecond of the crash.
2. **Undo Phase:** We scan backward to find uncommitted transactions and roll them back. However, we **never delete logs**. As we undo actions, we append **Compensation Log Records (CLRs)** forward into the WAL. The log only ever grows forward, providing a permanent historical trail of failures and rollbacks.

---

## 3. The Torn Page Problem
A major risk with physiological logging is the **Torn Page Problem**. 
If the power fails while the Operating System is flushing a 4KB B-Tree page from the Buffer Pool to the physical disk, the disk head might only write 2KB of it. The page on disk becomes physically corrupted (half old data, half new data). 

Physiological redo logs assume the underlying physical page is intact. If we apply an `INSERT X` logical redo operation to a corrupted page, the internal byte offsets and cell headers will panic, destroying the database.

### How Real Databases Solve Torn Pages:
1. **Full Page Writes (PostgreSQL):**
   The very first time a page is modified after a system Checkpoint, the database logs the **entire 4KB physical page** into the WAL. If a torn page occurs, recovery simply copies the full 4KB backup from the WAL over the corrupted page on disk, and then resumes applying physiological logs.
2. **The Doublewrite Buffer (MySQL/InnoDB):**
   Before flushing a dirty 4KB page to the main data file, MySQL first writes it sequentially to a dedicated "Doublewrite Buffer" file. Once `fsync` confirms it is safe there, it writes it to the main file. If the system crashes during the main file write, MySQL copies the intact page from the Doublewrite Buffer.

### Our Strategy: Full Page Writes (FPW)
We have chosen to implement **Strategy 1: Full Page Writes** because it perfectly integrates into our existing append-only WAL architecture. We do not need to manage a separate, complex Doublewrite Buffer file. 

Instead, we will add an `IsFullPage` flag to our `LogRecord`. The first time the Buffer Manager dirties a clean page, it will ask the WAL to append the entire 4KB byte array. Subsequent edits to that page will just append tiny physiological records. During recovery, if we see an `IsFullPage` flag, we completely overwrite the B-Tree page with the payload before continuing.
