package storage

import (
	"encoding/binary"
	"sync"
	"time"
)

const TXN_RUNNING = 0
const TXN_COMMITED = 1
const TXN_ROLLINGBACK = 2
const TXN_ROLLEDBACK = 3

type TxnID uint64
type TxnStatus uint8

type TransactionManager struct {
	txnTable map[TxnID]TxnStatus

	txnMu sync.Mutex
}

func NewTransactionManager() *TransactionManager {
	return &TransactionManager{
		txnTable: make(map[TxnID]TxnStatus),
	}
}

func (t *TransactionManager) SetStatus(txid TxnID, status TxnStatus) {
	t.txnMu.Lock()
	defer t.txnMu.Unlock()
	t.txnTable[txid] = status
}

func (t *TransactionManager) GetStatus(txid TxnID) TxnStatus {
	t.txnMu.Lock()
	defer t.txnMu.Unlock()
	return t.txnTable[txid]
}

func (t *TransactionManager) Begin() TxnID {
	t.txnMu.Lock()
	defer t.txnMu.Unlock()
	txId := GenerateTxID()
	t.txnTable[txId] = TXN_RUNNING
	return txId
}

func (t *TransactionManager) Commit(txid TxnID) {
	t.txnMu.Lock()
	defer t.txnMu.Unlock()
	t.txnTable[txid] = TXN_COMMITED
}

func (t *TransactionManager) Rollback(txid TxnID) {
	t.txnMu.Lock()
	defer t.txnMu.Unlock()
	t.txnTable[txid] = TXN_ROLLEDBACK
}

// generateTxID generates a monotonically increasing Transaction ID.
// For now, we use UnixNano to guarantee unique, increasing IDs.
func GenerateTxID() TxnID {
	return TxnID(time.Now().UnixNano())
}

// BuildMVCCKey formats the key as: [UserKey] + [\x00] + [8-byte BigEndian TxID]
func BuildMVCCKey(key []byte, txID uint64) []byte {
	mvccKey := make([]byte, len(key)+9)
	copy(mvccKey, key)
	mvccKey[len(key)] = 0x00
	binary.BigEndian.PutUint64(mvccKey[len(key)+1:], txID)
	return mvccKey
}
