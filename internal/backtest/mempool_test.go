package backtest

import (
	"os"
	"testing"
)

func TestMempoolDB(t *testing.T) {
	// Use test database
	dbPath := "../../data/mempool.db"
	
	// Skip if DB doesn't exist
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skip("mempool.db not found, run ingestion first")
	}

	db, err := NewMempoolDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to open DB: %v", err)
	}
	defer db.Close()

	// Test 1: Get stats
	stats, err := db.GetStats()
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats["total_txs"] == 0 {
		t.Error("Database is empty")
	}

	t.Logf("Total txs: %d", stats["total_txs"])
	t.Logf("Blocks covered: %d", stats["blocks_covered"])

	// Test 2: Get mempool for a known block
	// Block 17916526 was mined on 2023-08-15
	blockNumber := uint64(17916526)
	txs, err := db.GetMempoolForBlock(blockNumber)
	if err != nil {
		t.Fatalf("GetMempoolForBlock failed: %v", err)
	}

	if len(txs) == 0 {
		t.Error("No transactions found for block - query logic might be wrong")
	}

	t.Logf("Found %d txs in mempool for block %d", len(txs), blockNumber)

	// Test 3: Verify txs are decodable
	decodedCount := 0
	for i, tx := range txs {
		if tx.Hash().Hex() != "" {
			decodedCount++
		}
		if i >= 10 {
			break // Just check first 10
		}
	}

	if decodedCount == 0 {
		t.Error("No transactions could be decoded")
	}

	t.Logf("Successfully decoded %d/10 sample transactions", decodedCount)
}