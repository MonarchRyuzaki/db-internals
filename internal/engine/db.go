package engine

import (
	"encoding/binary"
	"time"

	"github.com/MonarchRyuzaki/db-internals/internal/storage"
)

// DB is the MVCC Coordinator that wraps the generic B-Tree storage engine.
type DB struct {
	index storage.Index
}

// NewDB creates a new MVCC Database wrapper.
func NewDB(index storage.Index) *DB {
	return &DB{
		index: index,
	}
}

// generateTxID generates a monotonically increasing Transaction ID.
// For now, we use UnixNano to guarantee unique, increasing IDs.
func (db *DB) generateTxID() uint64 {
	return uint64(time.Now().UnixNano())
}

// BuildMVCCKey formats the key as: [UserKey] + [\x00] + [8-byte BigEndian TxID]
func BuildMVCCKey(key []byte, txID uint64) []byte {
	mvccKey := make([]byte, len(key)+9)
	copy(mvccKey, key)
	mvccKey[len(key)] = 0x00
	binary.BigEndian.PutUint64(mvccKey[len(key)+1:], txID)
	return mvccKey
}

// Set inserts or updates a key with a new MVCC version.
func (db *DB) Set(key string, value string) error {
	txID := db.generateTxID()
	mvccKey := BuildMVCCKey([]byte(key), txID)
	return db.index.Insert(mvccKey, []byte(value))
}

// Get retrieves the latest committed version of the key.
func (db *DB) Get(key string) (string, error) {
	txID := db.generateTxID()
	mvccKey := BuildMVCCKey([]byte(key), txID)
	
	valBytes, err := db.index.FindLatest(mvccKey)
	if err != nil {
		return "", err
	}
	return string(valBytes), nil
}

// Delete marks the key as deleted by inserting a Tombstone version.
func (db *DB) Delete(key string) error {
	txID := db.generateTxID()
	mvccKey := BuildMVCCKey([]byte(key), txID)
	return db.index.Delete(mvccKey)
}
