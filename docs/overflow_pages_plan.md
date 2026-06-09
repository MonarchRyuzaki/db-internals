# Overflow Pages Implementation Plan

## Goal
Support inserting key-value pairs that are larger than a single page (e.g., a 50KB JSON document).

## Context
A standard B-Tree crashes or fails if a single record exceeds the fixed page size (4KB). To solve this without breaking the structural integrity of the tree, we will implement Cell-Level Overflow chaining.

Because this fundamentally changes how data is fetched and stored outside of the standard B-Tree page logic, it is treated as a separate, distinct feature.

## Architecture

### 1. The Overflow Page (PageType = 3)
We will introduce a new page type: `PageTypeOverflow = 3`. 
Unlike Leaf or Internal nodes, Overflow pages do not have a slotted directory. They are dedicated purely to holding raw bytes of a single large value.
* **Header:** They have a tiny header containing a `NextOverflowPageID` pointer. This allows them to act as a linked list if the value spans more than one overflow page.
* **Body:** The rest of the 4KB page is pure data.

### 2. The Overflow Cell Flag
We will introduce `KEY_OVERFLOW_FLAG` (`4`) in the `KVCell`.

### 3. Insertion Logic
When `Insert()` detects that a cell is too large to fit in an entirely empty page, it intercepts the raw data instead of throwing a "page full" error.
1. It allocates a dedicated chain of Overflow Pages and sets the overflow flag for that cell.
2. The massive data is chunked and written across this chain.
3. The original `KVCell` stored in the Leaf Node becomes tiny. Instead of holding the raw string, its `Value` byte slice holds exactly 8 bytes of metadata:
   - `4 bytes`: The `uint32` PageID of the first overflow page.
   - `4 bytes`: The `uint32` total length of the raw data.

### 4. Fetch Logic
When `Find()` retrieves a `KVCell`, it checks the flag.
1. If the flag indicates it is an Overflow Cell, it reads the 8-byte metadata.
2. It fetches the `FirstOverflowPageID` from the Buffer Manager.
3. It reads the data chunk, then follows the `NextOverflowPageID` pointer to the next page, continuing until it has reassembled the full byte array.
4. It returns the reassembled array to the user.

## Why Cell-Level Overflow? (Loose Coupling)
By tying the overflow chain to the individual `KVCell` rather than the Leaf Page header, we achieve strict loose coupling. 

If a Leaf Page splits, the B-Tree logic simply moves the tiny 8-byte `OverflowCell` to the new leaf. The massive 50KB data sitting in the overflow pages on disk remains completely untouched and ignorant of the tree's rebalancing. This keeps B-Tree splits incredibly fast, requires zero data copying for large records during a split, and perfectly preserves the `O(log N)` search guarantee.
