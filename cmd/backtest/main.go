package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/pulkyeet/mev-searcher/internal/backtest"
	"github.com/pulkyeet/mev-searcher/internal/eth"
)

func main() {
	_ = godotenv.Load("../../.env")

	var (
		dbPath    = flag.String("db", "data/mempool.db", "Path to mempool database")
		startBlock = flag.Uint64("start", 17916526, "Start block number")
		endBlock   = flag.Uint64("end", 17916626, "End block number")
	)
	flag.Parse()

	// Validate
	if *startBlock >= *endBlock {
		fmt.Println("Error: start block must be < end block")
		os.Exit(1)
	}

	// Connect to Ethereum
	client, err := eth.NewClient()
	if err != nil {
		fmt.Printf("Failed to connect to RPC: %v\n", err)
		os.Exit(1)
	}

	// Create runner
	runner, err := backtest.NewRunner(client, *dbPath)
	if err != nil {
		fmt.Printf("Failed to create runner: %v\n", err)
		os.Exit(1)
	}
	defer runner.Close()

	// Run backtest
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Hour)
	defer cancel()

	report, err := runner.RunBacktest(ctx, *startBlock, *endBlock)
	if err != nil {
		fmt.Printf("Backtest failed: %v\n", err)
		os.Exit(1)
	}

	// Print results
	report.Print()
}