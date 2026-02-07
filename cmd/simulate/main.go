package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pulkyeet/mev-searcher/internal/eth"
	"github.com/pulkyeet/mev-searcher/internal/simulator"
)

func main() {
	blockNum := flag.Int64("block", 0, "Block number to fork from")
	txHash := flag.String("tx", "", "Transaction hash to simulate")
	bundle := flag.String("bundle", "", "Comma-separated tx hashes for bundle simulation")
	verbose := flag.Bool("v", false, "Verbose output (show debug info)")

	flag.Parse()

	if *blockNum == 0 {
		log.Fatal("Usage: simulate --block <number> [--tx <hash> | --bundle <hash1,hash2,...>]")
	}

	client, err := eth.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// Fork state
	if *verbose {
		fmt.Printf("Forking state at block %d...\n", *blockNum-1)
	}
	fork, err := simulator.NewStateFork(client, big.NewInt(*blockNum-1))
	if err != nil {
		log.Fatal(err)
	}

	// Fetch block
	if *verbose {
		fmt.Printf("Fetching block %d...\n", *blockNum)
	}
	block, err := client.BlockByNumber(ctx, big.NewInt(*blockNum))
	if err != nil {
		log.Fatal(err)
	}

	// Bundle mode
	if *bundle != "" {
		executeBundleMode(ctx, client, fork, block, *bundle, *verbose)
		return
	}

	// Single tx mode
	if *txHash != "" {
		executeSingleTxMode(ctx, client, fork, block, *txHash, *verbose)
		return
	}

	// No tx specified - show available txs
	fmt.Printf("Block %d has %d transactions\n", *blockNum, len(block.Transactions()))
	fmt.Println("\nFirst 5 transactions:")
	for i := 0; i < 5 && i < len(block.Transactions()); i++ {
		fmt.Printf("  [%d] %s\n", i, block.Transactions()[i].Hash().Hex())
	}
}

func executeBundleMode(ctx context.Context, client *eth.Client, fork *simulator.StateFork, block *types.Block, bundleStr string, verbose bool) {
	hashes := strings.Split(bundleStr, ",")
	if len(hashes) < 2 {
		log.Fatal("Bundle must contain at least 2 transactions")
	}

	// Fetch bundle txs
	var bundleTxs []*types.Transaction
	for _, hashStr := range hashes {
		hashStr = strings.TrimSpace(hashStr)
		hash := common.HexToHash(hashStr)

		var found bool
		for _, tx := range block.Transactions() {
			if tx.Hash() == hash {
				bundleTxs = append(bundleTxs, tx)
				found = true
				break
			}
		}

		if !found {
			log.Fatalf("Transaction %s not found in block %d", hashStr, block.Number())
		}
	}

	// Execute bundle
	bundleSim := simulator.NewBundleSimulator(fork)
	result, err := bundleSim.ExecuteBundle(bundleTxs, block)
	if err != nil {
		log.Fatal(err)
	}

	// Print results
	fmt.Printf("\n=== Bundle Simulation ===\n")
	fmt.Printf("Block:       %d\n", block.Number())
	fmt.Printf("Transactions: %d\n", len(bundleTxs))
	fmt.Printf("Success:     %v\n", result.Success)
	fmt.Printf("Total Gas:   %d\n", result.TotalGasUsed)
	
	if !result.Success {
		fmt.Printf("Failed at:   tx %d\n", result.RevertedAt)
	}

	fmt.Println("\nTransaction Details:")
	for i, txResult := range result.Transactions {
		status := "✓"
		if !txResult.Success {
			status = "✗"
		}
		fmt.Printf("  [%d] %s %s (gas: %d, logs: %d)\n", 
			i, status, txResult.TxHash.Hex()[:10]+"...", txResult.GasUsed, len(txResult.Logs))
		if !txResult.Success && verbose {
			fmt.Printf("      Revert: %s\n", txResult.RevertReason)
		}
	}
}

func executeSingleTxMode(ctx context.Context, client *eth.Client, fork *simulator.StateFork, block *types.Block, txHashStr string, verbose bool) {
	hash := common.HexToHash(txHashStr)
	
	// Find tx in block
	var targetTx *types.Transaction
	var txIndex int
	for i, tx := range block.Transactions() {
		if tx.Hash() == hash {
			targetTx = tx
			txIndex = i
			break
		}
	}

	if targetTx == nil {
		fmt.Printf("Transaction %s not found in block %d\n", txHashStr, block.Number())
		fmt.Println("\nFirst 3 transactions in this block:")
		for i := 0; i < 3 && i < len(block.Transactions()); i++ {
			fmt.Printf("  %s\n", block.Transactions()[i].Hash().Hex())
		}
		return
	}

	// Apply prior transactions
	if txIndex > 0 {
		if verbose {
			fmt.Printf("Applying %d prior transactions...\n", txIndex)
		}
		executor := simulator.NewExecutor(fork)
		for i := 0; i < txIndex; i++ {
			priorTx := block.Transactions()[i]
			_, err := executor.ExecuteTransaction(priorTx, block)
			if err != nil {
				log.Fatalf("Failed to apply prior tx %d: %v", i, err)
			}
		}
	}

	// Execute target tx
	executor := simulator.NewExecutor(fork)
	result, err := executor.ExecuteTransaction(targetTx, block)
	if err != nil {
		log.Fatal(err)
	}

	// Get on-chain receipt
	receipt, err := client.TransactionReceipt(ctx, targetTx.Hash())
	if err != nil && verbose {
		log.Printf("Warning: couldn't fetch receipt: %v", err)
	}

	// Print results
	fmt.Printf("\n=== Transaction Simulation ===\n")
	fmt.Printf("Block:    %d\n", block.Number())
	fmt.Printf("Index:    %d\n", txIndex)
	fmt.Printf("Hash:     %s\n", targetTx.Hash().Hex())
	fmt.Printf("From:     %s\n", getFrom(targetTx))
	if targetTx.To() != nil {
		fmt.Printf("To:       %s\n", targetTx.To().Hex())
	} else {
		fmt.Printf("To:       CONTRACT_CREATION\n")
	}
	
	fmt.Printf("\n--- Simulation ---\n")
	fmt.Printf("Success:  %v\n", result.Success)
	fmt.Printf("Gas Used: %d\n", result.GasUsed)
	fmt.Printf("Logs:     %d events\n", len(result.Logs))
	
	if !result.Success {
		fmt.Printf("Revert:   %s\n", result.RevertReason)
	}

	if receipt != nil {
		fmt.Printf("\n--- On-Chain Receipt ---\n")
		fmt.Printf("Status:   %d (1=success, 0=failed)\n", receipt.Status)
		fmt.Printf("Gas Used: %d\n", receipt.GasUsed)
		fmt.Printf("Logs:     %d events\n", len(receipt.Logs))
		
		diff := int64(result.GasUsed) - int64(receipt.GasUsed)
		if diff == 0 {
			fmt.Printf("\n✓ PERFECT MATCH\n")
		} else {
			fmt.Printf("\n⚠ Gas mismatch: %+d (%.2f%%)\n", diff, float64(diff)/float64(receipt.GasUsed)*100)
		}
	}
}

func getFrom(tx *types.Transaction) string {
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err := types.Sender(signer, tx)
	if err != nil {
		return "UNKNOWN"
	}
	return from.Hex()
}