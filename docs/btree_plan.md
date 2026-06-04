# B+ Tree Implementation Plan

## 1. Pluggable Architecture (The Interface)
To ensure the B+ Tree can be easily swapped out for a different implementation later (like a Hash Index or LSM Tree), we will start by defining an `Index` interface. The rest of the database will only interact with this interface.

```go
type Index interface {
    Insert(key []byte, value []byte) error
    Find(key []byte) ([]byte, error)
    Delete(key []byte) error
}
```

The B+ Tree will simply be a struct that implements this interface:
```go
type BTree struct {
    bm         *BufferManager
    rootPageID uint32
}
```

## 2. Core Philosophy: Direct Page Manipulation
We will **not** deserialize the entire B+ Tree into a massive in-memory node graph. Instead, the tree logic will act as a thin coordinator:
1. Fetch a `Page` from the Buffer Manager.
2. Search through its cells using `page.Get(slotID)`.
3. Extract child pointers or data.
4. Unpin the page and fetch the next one.

## 3. The "Breadcrumb" Stack (Path Traversal)
Because we operate directly on disk pages, we cannot use traditional in-memory parent pointers (`node.parent`). To handle splits and merges, we must track our path from the root down to the leaf. 

We will maintain a `Stack` (a simple slice of `uint32` PageIDs) during traversal. 
* **Going down:** Push the current `PageID` onto the stack before fetching the child.
* **Going up (Splitting):** Pop the stack to find the exact parent `PageID` we need to insert the new split key into.

---

## 4. Implementation Steps

### Step 1: Initialization & Find (Search)
* **Goal:** Implement the interface scaffold and tree traversal.
* **Logic:** 
  1. Fetch `rootPageID`.
  2. While `page.PageType == PageTypeInternal`:
     * Loop through cells, compare keys.
     * Find the highest key that is `<=` our search key.
     * Extract the `childPageID` from that cell.
     * `Unpin` current page, `Fetch` the child page.
  3. When `PageType == PageTypeLeaf`:
     * Binary or linear search the cells for the exact key.
     * Return the value.

### Step 2: Basic Insert (No Splitting)
* **Goal:** Successfully insert a record into a tree that has enough free space.
* **Logic:**
  1. Traverse to the correct leaf node, maintaining the breadcrumb stack.
  2. Try to insert the serialized `KVCell` into the leaf page.
  3. If it succeeds, `Unpin` the page with `isDirty = true`. Done.

### Step 3: Insert with Splitting (The Tricky Part)
* **Goal:** Handle the "Page Full" error from `page.Insert()`.
* **Logic:**
  1. Allocate a brand new page.
  2. Move half of the cells from the full page to the new page.
  3. Extract the "middle key" (the first key of the new page).
  4. **Pop the Stack** to get the parent's `PageID`.
  5. Fetch the parent page, and insert a `KeyCell` pointing to the new page.
  6. **Recursive Split:** If the parent is *also* full, repeat this process upwards. 
  7. **Root Split:** If the stack is empty (we split the root), allocate a *new* root page, make it point to the two halves, and update `btree.rootPageID`.

### Step 4: Delete & Merging (Advanced)
* **Goal:** Remove keys and handle underflow.
* **Logic:**
  1. Traverse to the leaf and delete the cell.
  2. Check for **Underflow** (Is the page less than half full?).
  3. If underflow, fetch left/right siblings.
  4. **Borrow:** If a sibling is comfortably full, steal one cell to satisfy the threshold and update the parent's boundary key.
  5. **Merge:** If siblings are also nearly empty, combine the two pages into one, and delete the boundary key from the parent (which might trigger a recursive merge upwards).
