package backtest

import (
	"database/sql"
	"fmt"
	_ "os"
	_ "path/filepath"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	_ "github.com/mattn/go-sqlite3"
)

type MempoolDB struct {
	db *sql.DB
}

func NewMempoolDB(dbPath string) (*MempoolDB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open db: %w", err)
	}

	// Enable WAL mode
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	// Schema is created by Python ingestion script
	// Just verify table exists
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM mempool_txs LIMIT 1").Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("mempool_txs table not found - run ingestion first: %w", err)
	}

	return &MempoolDB{db: db}, nil
}

func (m *MempoolDB) Close() error {
	return m.db.Close()
}

// InsertTx stores a mempool transaction
func (m *MempoolDB) InsertTx(tx *MempoolTx) error {
	_, err := m.db.Exec(`
		INSERT OR IGNORE INTO mempool_txs 
		(tx_hash, block_number, timestamp, raw_tx, gas_price, tx_to, tx_value)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`,
		tx.Hash.Hex(),
		tx.BlockNumber,
		tx.Timestamp,
		tx.RawTx,
		tx.GasPrice.String(),
		addressToString(tx.To),
		tx.Value.String(),
	)
	return err
}

// BatchInsert stores multiple transactions efficiently
func (m *MempoolDB) BatchInsert(txs []*MempoolTx) error {
	if len(txs) == 0 {
		return nil
	}

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO mempool_txs 
		(tx_hash, block_number, timestamp, raw_tx, gas_price, tx_to, tx_value)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, mtx := range txs {
		_, err := stmt.Exec(
			mtx.Hash.Hex(),
			mtx.BlockNumber,
			mtx.Timestamp,
			mtx.RawTx,
			mtx.GasPrice.String(),
			addressToString(mtx.To),
			mtx.Value.String(),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetMempoolForBlock returns all txs that were in mempool when block N was built
// This means: all txs with timestamp < block_N_timestamp
func (m *MempoolDB) GetMempoolForBlock(blockNumber uint64) ([]*types.Transaction, error) {
	// First, get the timestamp when this block was included
	var blockTimestamp int64
	err := m.db.QueryRow(`
		SELECT included_block_timestamp 
		FROM mempool_txs 
		WHERE included_block = ? AND included_block_timestamp IS NOT NULL
		LIMIT 1
	`, blockNumber).Scan(&blockTimestamp)

	if err != nil {
		return nil, fmt.Errorf("block %d not found in mempool data: %w", blockNumber, err)
	}

	// Get all txs that were seen BEFORE this block was mined
	// These represent the mempool state at block N-1
	rows, err := m.db.Query(`
		SELECT raw_tx FROM mempool_txs 
		WHERE timestamp < ?
		ORDER BY timestamp ASC
	`, blockTimestamp)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txs []*types.Transaction
	for rows.Next() {
		var rawTxHex string
		if err := rows.Scan(&rawTxHex); err != nil {
			continue
		}

		// Decode hex string to bytes
		rawTx, err := hexToBytes(rawTxHex)
		if err != nil {
			continue
		}

		var tx types.Transaction
		if err := rlp.DecodeBytes(rawTx, &tx); err != nil {
			continue
		}

		txs = append(txs, &tx)
	}

	return txs, nil
}

// returns DB stats
func (m *MempoolDB) GetStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	var count int64
	err := m.db.QueryRow("SELECT COUNT(*) FROM mempool_txs").Scan(&count)
	if err!=nil {
		return nil, err
	}
	stats["total_txs"] = count

	err = m.db.QueryRow("SELECT COUNT(DISTINCT included_block) FROM mempool_txs").Scan(&count)
	if err != nil {
		return nil, err
	}
	stats["blocks_covered"] = count

	return stats, nil
}

func addressToString(addr *common.Address) string {
	if addr == nil {
		return ""
	}
	return addr.Hex()
}