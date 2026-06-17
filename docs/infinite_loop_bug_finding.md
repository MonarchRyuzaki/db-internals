# Bug Log: Infinite Loop during B-Tree Traversal

## The Symptom
Running `SET k v` in the CLI REPL (`cmd/db/main.go`) would occasionally hang indefinitely without returning an error or success message. The CPU would spike, but the terminal remained frozen.

## The Root Cause
The freeze was not caused by a concurrency deadlock, but rather an **infinite loop** during the B-Tree traversal inside `findLeafPage`.

Here is the exact sequence of events that led to the corruption:
1. When the CLI booted up for the first time, it allocated `Page 0` (the Meta Page) and `Page 1` (the Root Leaf Page). It updated the Meta Page in memory to set `RootPageID = 1`.
2. The user executed several `SET` commands, mutating pages in memory, and then typed `EXIT` to shut down the CLI.
3. Because the `BTree` lacked a graceful `Close()` method, the `BufferManager` never flushed its dirty pages to disk. As a result, the physical `.db` file contained a completely empty `Meta Page` with `RootPageID` still set to `0`.
4. Upon rebooting the CLI, the engine read the corrupted `.db` file, resolving the `RootPageID` to `0`.
5. When `SET k v` was executed, `findLeafPage` fetched `Page 0`. Because `Page 0` had no valid child pointers, the `nextChildID` fallback evaluated to `0`. The loop set `currPageID = 0` and restarted the iteration, spinning infinitely and repeatedly acquiring read-locks on `Page 0`.

## The Solution
To fix this and ensure absolute persistence:
1. **Buffer Manager Flush:** Added `FlushAll()` to the `BufferManager` to iterate through the `pageTable` and forcefully write all `IsDirty == true` frames to the physical disk.
2. **Graceful Shutdown:** Implemented `Close()` on the `BTree` to correctly close the WAL and trigger the Buffer Manager's flush routine.
3. **Application Lifecycle:** Added `defer db.Close()` to `cmd/db/main.go` to guarantee that all pending changes in the buffer pool are flushed to disk when the CLI terminates.
4. **Traversal Safeguard:** Added a strict sanity check inside `findLeafPage`: if `nextChildID == 0`, it instantly returns a `database corrupted: child pointer is 0` error rather than spinning infinitely.
