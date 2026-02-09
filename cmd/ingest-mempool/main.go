package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/joho/godotenv"
	"github.com/pulkyeet/mev-searcher/internal/backtest"
	"github.com/xitongsys/parquet-go-source/local"
	"github.com/xitongsys/parquet-go/reader"
)

// ParquetRow matches the structure from Flashbots mempool-dumpster
type ParquetRow struct {
	Timestamp              int64
	Hash                   string
	ChainId                string
	From                   string
	To                     string
	Value                  string
	Nonce                  string
	Gas                    string
	GasPrice               string
	GasTipCap              string
	GasFeeCap              string
	DataSize               int64
	Data4Bytes             string
	Sources                []string
	IncludedAtBlockHeight  int64
	IncludedBlockTimestamp int64
	InclusionDelayMs       int64
	RawTx                  string
}

func main() {
	_ = godotenv.Load("../../.env")

	parquetFile := flag.String("file", "", "Path to parquet file")
	dbPath := flag.String("db", "data/mempool.db", "Path to SQLite database")
	flag.Parse()

	if *parquetFile == "" {
		log.Fatal("Usage: --file <parquet_file>")
	}

	fmt.Printf("📥 Ingesting mempool data from %s...\n", *parquetFile)

	// Open mempool database
	db, err := backtest.NewMempoolDB(*dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Open parquet file
	fr, err := local.NewLocalFileReader(*parquetFile)
	if err != nil {
		log.Fatalf("Failed to open parquet file: %v", err)
	}
	defer fr.Close()

	pr, err := reader.NewParquetReader(fr, new(ParquetRow), 4)
	if err != nil {
		log.Fatalf("Failed to create parquet reader: %v", err)
	}
	defer pr.ReadStop()

	numRows := int(pr.GetNumRows())
	fmt.Printf("📊 Total transactions: %d\n", numRows)

	// Read in batches
	batchSize := 1000
	totalIngested := 0
	startTime := time.Now()

	for i := 0; i < numRows; i += batchSize {
		// Calculate how many to read
		toRead := batchSize
		if i+toRead > numRows {
			toRead = numRows - i
		}

		// Read batch using ReadByNumber
		rawRows, err := pr.ReadByNumber(toRead)
		if err != nil {
			log.Printf("Warning: failed to read batch at %d: %v", i, err)
			break
		}

		if len(rawRows) == 0 {
			break
		}

		// Parse rows
		batch := make([]*backtest.MempoolTx, 0, len(rawRows))
		for _, rawRow := range rawRows {
			// Cast to ParquetRow
			pRow, ok := rawRow.(ParquetRow)
			if !ok {
				// Try pointer type
				pRowPtr, ok := rawRow.(*ParquetRow)
				if !ok {
					continue
				}
				pRow = *pRowPtr
			}

			// Decode raw tx (it's hex string)
			rawTx, err := hex.DecodeString(strings.TrimPrefix(pRow.RawTx, "0x"))
			if err != nil {
				continue
			}

			// Decode the transaction
			var tx types.Transaction
			if err := rlp.DecodeBytes(rawTx, &tx); err != nil {
				continue
			}

			// Extract sender
			signer := types.LatestSignerForChainID(tx.ChainId())
			from, err := types.Sender(signer, &tx)
			if err != nil {
				continue
			}

			mtx := &backtest.MempoolTx{
				Hash:          common.HexToHash(pRow.Hash),
				Timestamp:     uint64(pRow.Timestamp / 1000), // ms to seconds
				IncludedBlock: uint64(pRow.IncludedAtBlockHeight),
				RawTx:         rawTx,
				From:          from,
				To:            tx.To(),
				GasPrice:      tx.GasPrice(),
			}

			batch = append(batch, mtx)
		}

		// Insert batch
		if len(batch) > 0 {
			if err := db.BatchInsert(batch); err != nil {
				log.Printf("Warning: failed to insert batch: %v", err)
				continue
			}
		}

		totalIngested += len(batch)

		if totalIngested%10000 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(totalIngested) / elapsed.Seconds()
			fmt.Printf("  ✓ Ingested %d txs (%.0f tx/s)\n", totalIngested, rate)
		}
	}

	elapsed := time.Since(startTime)
	fmt.Printf("\n✅ Ingestion complete!\n")
	fmt.Printf("  Total: %d transactions\n", totalIngested)
	fmt.Printf("  Time: %s\n", elapsed)
	fmt.Printf("  Rate: %.0f tx/s\n", float64(totalIngested)/elapsed.Seconds())

	// Show stats
	stats, err := db.GetStats()
	if err != nil {
		log.Printf("Failed to get stats: %v", err)
		return
	}

	fmt.Printf("\n📊 Database Stats:\n")
	fmt.Printf("  Total txs: %d\n", stats["total_txs"])
	fmt.Printf("  Blocks covered: %d\n", stats["blocks_covered"])
}
