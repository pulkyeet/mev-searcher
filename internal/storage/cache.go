package storage

import (
	"database/sql"
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common"
	_ "github.com/mattn/go-sqlite3"
)

type CacheDB struct {
	db *sql.DB
}

func NewCacheDB(dbPath string) (*CacheDB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err!=nil {
		return nil, fmt.Errorf("failed to create cache dir: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err!=nil {
		return nil, fmt.Errorf("failed to open cache db: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err!=nil {
		return nil, fmt.Errorf("failed to enable WAL: %w", err)
	}

	// load schenma
	schema, err := os.ReadFile("internal/storage/schema.sql")
	if err!=nil {
		return nil, fmt.Errorf("failed to read schema: %w", err)
	}

	if _, err := db.Exec(string(schema)); err!=nil {
		return nil, fmt.Errorf("failed to initialise schema: %w", err)
	}

	return &CacheDB{db: db}, nil
}

func (c *CacheDB) Close() error {
	return c.db.Close()
}

// Account State operations
func (c *CacheDB) GetBalance(blockNumber uint64, addr common.Address) (*big.Int, bool) {
	var balanceStr string
	err := c.db.QueryRow(
		"SELECT balance FROM account_state WHERE block_number=? AND address=?",
		blockNumber, addr.Hex(),
	).Scan(&balanceStr)

	if err==sql.ErrNoRows {
		return nil, false
	}
	if err!=nil {
		return nil, false
	}

	balance := new(big.Int)
	balance.SetString(balanceStr,10)
	return balance, true
}

func (c *CacheDB) SetBalance(blockNumber uint64, addr common.Address, balance *big.Int) error {
	_, err := c.db.Exec(
		"INSERT OR REPLACE INTO account_state (block_number, address, balance) VALUES (? , ?, ?)",
		blockNumber, addr.Hex(), balance.String(),
	)
	return err
}

func (c *CacheDB) GetNonce(blockNumber uint64, addr common.Address) (uint64, bool) {
	var nonce uint64
	err := c.db.QueryRow(
		"SELECT nonce FROM account_state WHERE block_numer = ? AND address = ?",
		blockNumber, addr.Hex(),
	).Scan(&nonce)

	if err==sql.ErrNoRows {
		return 0, false
	}
	if err!=nil {
		return 0, false
	}

	return nonce, true
}

func (c *CacheDB) SetNonce(blockNumber uint64, addr common.Address, nonce uint64) error {
	_, err := c.db.Exec(
		"INSERT OR REPLACE INTO account_state (block_number, address, nonce) VALUES (?, ?, ?)",
		blockNumber, addr.Hex(), nonce,
	)
	return err
}

func (c *CacheDB) GetCode(blockNumber uint64, addr common.Address) ([]byte, bool) {
	var code []byte
	err := c.db.QueryRow(
		"SELECT code FROM account_state WHERE block_number = ? AND address = ?",
		blockNumber, addr.Hex(),
	).Scan(&code)

	if err == sql.ErrNoRows {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	return code, true
}

func (c *CacheDB) SetCode(blockNumber uint64, addr common.Address, code []byte) error {
	_, err := c.db.Exec(
		"INSERT OR REPLACE INTO account_state (block_number, address, code) VALUES (?, ?, ?)",
		blockNumber, addr.Hex(), code,
	)
	return err
} 

// Storage operations
func (c *CacheDB) GetStorage(blockNumber uint64, addr common.Address, slot common.Hash) (common.Hash, bool) {
	var valueHex string
	err := c.db.QueryRow(
		"SELECT value FROM storage_state WHERE block_number = ? AND address = ? AND slot = ?",
		blockNumber, addr.Hex(), slot.Hex(),
	).Scan(&valueHex)

	if err == sql.ErrNoRows {
		return common.Hash{}, false
	}
	if err != nil {
		return common.Hash{}, false
	}

	return common.HexToHash(valueHex), true
}

func (c *CacheDB) SetStorage(blockNumber uint64, addr common.Address, slot, value common.Hash) error {
	_, err := c.db.Exec(
		"INSERT OR REPLACE INTO storage_state (block_number, address, slot, value) VALUES (?, ?, ?, ?)",
		blockNumber, addr.Hex(), slot.Hex(), value.Hex(),
	)
	return err
}

// Batch operations for prewarming

type AccountData struct {
	Address common.Address
	Balance *big.Int
	Nonce uint64
	Code []byte
}

func (c *CacheDB) BatchSetAccounts(blockNumber uint64, accounts []AccountData) error {
	tx, err := c.db.Begin()
	if err!=nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		"INSERT OR REPLACE INTO account_state (block_number, address, balance, nonce, code) VALUES (?,?,?,?,?)",
	)
	if err!=nil {
		return err
	}
	defer stmt.Close()

	for _, acc := range accounts {
		_, err := stmt.Exec(
			blockNumber,
			acc.Address.Hex(),
			acc.Balance.String(),
			acc.Nonce,
			acc.Code,
		)
		if err!=nil {
			return nil
		}
	}
	return tx.Commit()
}

type StorageData struct {
	Address common.Address
	Slot common.Hash
	Value common.Hash
}

func (c *CacheDB) BatchSetStorage(blockNumber uint64, storage []StorageData) error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		"INSERT OR REPLACE INTO storage_state (block_number, address, slot, value) VALUES (?, ?, ?, ?)",
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, s := range storage {
		_, err := stmt.Exec(
			blockNumber,
			s.Address.Hex(),
			s.Slot.Hex(),
			s.Value.Hex(),
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// stats for monitoring cache performance

func (c *CacheDB) GetStats() (map[string]int64, error) {
	stats := make(map[string]int64)

	var count int64
	if err := c.db.QueryRow("SELECT COUNT(*) FROM account_state").Scan(&count); err!=nil {
		return nil, err
	}
	stats["account_entries"] = count

	if err := c.db.QueryRow("SELECT COUNT(*) FROM storage_state").Scan(&count); err != nil {
		return nil, err
	}
	stats["storage_entries"] = count

	return stats, nil
}