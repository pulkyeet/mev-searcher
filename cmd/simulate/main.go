package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pulkyeet/mev-searcher/internal/eth"
	"github.com/pulkyeet/mev-searcher/internal/simulator"
)

func main() {
	blockNum := flag.Int64("block", 0, "Block number to fork from")
	txHash := flag.String("tx", "", "Transaction hash to simulate")
	flag.Parse()

	if *blockNum == 0 {
		log.Fatal("Usage: simulate --block <number> --tx <hash>")
	}

	client, err := eth.NewClient()
	if err!=nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// forking at block n-1
	fmt.Printf("Forking state at block %d...\n", *blockNum-1)
	fork, err := simulator.NewStateFork(client, big.NewInt(*blockNum-1))
	if err!=nil {
		log.Fatal(err)
	}

	// if no tx hash
	if *txHash=="" {
		fmt.Printf("no tx specified, testing balance fetch only")
		vitalik := common.HexToAddress("0xd8dA6BF26964aF9D7eEd9e03E53415D37aA96045")
		bal, _ := fork.GetBalance(vitalik)
		fmt.Printf("Vitalik balance: %s wei\n", bal.String())
		return
	}

	// fetching tx from block N
	fmt.Printf("fetching transaction %s...\n", *txHash)
	block ,err := client.BlockByNumber(ctx, big.NewInt(*blockNum))
	if err!=nil {
		log.Fatal(err)
	}

	hash := common.HexToHash(*txHash)
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
		fmt.Printf("Transaction %s not found in block %d\n", *txHash, *blockNum)
		fmt.Println("\nAvailable transactions in this block (first 3):")
		for i, tx := range block.Transactions() {
			if i >= 3 {
				break
			}
			fmt.Printf("  %s\n", tx.Hash().Hex())
		}
		return
	}

	fmt.Printf("found tx at index %d from %s\n", txIndex, targetTx.To().Hex())

	// exec all prior tx to build up state
	if txIndex>0 {
		fmt.Printf("Applying %d prior transactions to build state...\n", txIndex)
		executor := simulator.NewExecutor(fork)
		for i:=0; i<txIndex; i++ {
			priorTx := block.Transactions()[i]
			_, err := executor.ExecuteTransaction(priorTx, block)
			if err!=nil {
				log.Fatalf("Failed to apply prior tx %d: %v", i, err)
			}
		}
		fmt.Println("state built successfully")
	}

	// Debug transaction details
	fmt.Printf("\nTransaction Details:\n")
	fmt.Printf("  Type: %d\n", targetTx.Type())
	fmt.Printf("  Gas Limit: %d\n", targetTx.Gas())
	fmt.Printf("  Data Size: %d bytes\n", len(targetTx.Data()))
	fmt.Printf("  Value: %s wei\n", targetTx.Value().String())
	fmt.Printf("  Access List: %d entries\n", len(targetTx.AccessList()))
	if targetTx.Type() == types.DynamicFeeTxType {
		fmt.Printf("  Gas Fee Cap: %s\n", targetTx.GasFeeCap())
		fmt.Printf("  Gas Tip Cap: %s\n", targetTx.GasTipCap())
	}
	fmt.Println()

	// exec tx
	executor := simulator.NewExecutor(fork)
	result, err := executor.ExecuteTransaction(targetTx, block)
	if err!=nil {
		log.Fatal(err)
	}

	// get actual gas used from block receipt
	receipt, err := client.TransactionReceipt(ctx, targetTx.Hash())
	if err!=nil {
		log.Printf("Warning: couldnt fetch receipt: %v", err)
	}

	fmt.Printf("\n=== Simulation Result ===\n")
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Gas used: %d\n", result.GasUsed)
	if receipt!=nil {
		fmt.Printf("Gas Used (actual): %d\n", receipt.GasUsed)
		fmt.Printf("Receipt Status: %d\n", receipt.Status)
		fmt.Printf("Receipt Logs: %d\n", len(receipt.Logs))
		fmt.Printf("Cumulative Gas: %d\n", receipt.CumulativeGasUsed)
		diff := int64(result.GasUsed) - int64(receipt.GasUsed)
		if diff == 0 {
			fmt.Printf("GAS USED PERFECT MATCH (receipt vs result)\n")
		} else {
			fmt.Printf(" (%.2f%% off)\n", float64(diff)/float64(receipt.GasUsed)*100)
		}
	}
	fmt.Printf("Logs: %d events emitted\n", len(result.Logs))
	if !result.Success {
		fmt.Printf("Revert Reason: %s\n", result.RevertReason)
	}

}