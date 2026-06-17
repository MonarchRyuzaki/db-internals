package engine

import (
	"os"
	"testing"
	"time"

	"github.com/MonarchRyuzaki/db-internals/internal/storage"
)

func TestMVCC_BasicTimeTravel(t *testing.T) {
	// 1. Setup
	dbPath := "test_mvcc_db"
	os.RemoveAll(dbPath)
	os.MkdirAll(dbPath, 0755)
	defer os.RemoveAll(dbPath)

	bTree, err := storage.NewBTree("test_mvcc", dbPath)
	if err != nil {
		t.Fatalf("Failed to create BTree: %v", err)
	}
	defer bTree.Close()

	mvccDB := NewDB(bTree)

	// 2. Insert Version 1
	err = mvccDB.Set("UserA", "Alice_V1")
	if err != nil {
		t.Fatalf("Failed to set V1: %v", err)
	}

	val, err := mvccDB.Get("UserA")
	if err != nil || val != "Alice_V1" {
		t.Fatalf("Expected Alice_V1, got %v (err: %v)", val, err)
	}

	// 3. Sleep slightly to ensure TxID (UnixNano) increases significantly
	time.Sleep(2 * time.Millisecond)

	// Capture the TxID boundary
	boundaryTxID := uint64(time.Now().UnixNano())

	time.Sleep(2 * time.Millisecond)

	// 4. Insert Version 2
	err = mvccDB.Set("UserA", "Alice_V2")
	if err != nil {
		t.Fatalf("Failed to set V2: %v", err)
	}

	// 5. Get should now return Version 2
	val, err = mvccDB.Get("UserA")
	if err != nil || val != "Alice_V2" {
		t.Fatalf("Expected Alice_V2, got %v (err: %v)", val, err)
	}

	// 6. Time travel! Ask the raw B-Tree for the version of "UserA" at the boundaryTxID
	oldSearchKey := BuildMVCCKey([]byte("UserA"), boundaryTxID)
	oldValBytes, err := bTree.FindLatest(oldSearchKey)
	if err != nil {
		t.Fatalf("Failed to time travel: %v", err)
	}

	if string(oldValBytes) != "Alice_V1" {
		t.Fatalf("Time travel failed. Expected Alice_V1, got %v", string(oldValBytes))
	}
}

func TestMVCC_DeleteTimeTravel(t *testing.T) {
	dbPath := "test_mvcc_del_db"
	os.RemoveAll(dbPath)
	os.MkdirAll(dbPath, 0755)
	defer os.RemoveAll(dbPath)

	bTree, err := storage.NewBTree("test_mvcc_del", dbPath)
	if err != nil {
		t.Fatalf("Failed to create BTree: %v", err)
	}
	defer bTree.Close()

	mvccDB := NewDB(bTree)

	// 1. Insert Version 1
	mvccDB.Set("UserB", "Bob_V1")
	
	time.Sleep(2 * time.Millisecond)
	boundaryTxID := uint64(time.Now().UnixNano())
	time.Sleep(2 * time.Millisecond)

	// 2. Delete the key (Inserts a Tombstone Version)
	mvccDB.Delete("UserB")

	// 3. Ensure it is deleted from the current perspective
	_, err = mvccDB.Get("UserB")
	if err == nil {
		t.Fatalf("Expected error getting deleted key")
	}

	// 4. Time travel back to before it was deleted!
	oldSearchKey := BuildMVCCKey([]byte("UserB"), boundaryTxID)
	oldValBytes, err := bTree.FindLatest(oldSearchKey)
	if err != nil {
		t.Fatalf("Failed to time travel to deleted key: %v", err)
	}

	if string(oldValBytes) != "Bob_V1" {
		t.Fatalf("Time travel failed. Expected Bob_V1, got %v", string(oldValBytes))
	}
}
