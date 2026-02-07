package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pulkyeet/mev-searcher/internal/eth"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatal("Usage: go run find_consecutive.go <block_number>")
	}

	blockNum, _ := strconv.ParseInt(os.Args[1], 10, 64)

	client, err := eth.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	block, err := client.BlockByNumber(context.Background(), big.NewInt(blockNum))
	if err != nil {
		log.Fatal(err)
	}

	txs := block.Transactions()
	fmt.Printf("\nBlock %d has %d transactions\n\n", blockNum, len(txs))

	for i := 0; i < len(txs)-1; i++ {
		from1 := getFrom(txs[i])
		from2 := getFrom(txs[i+1])

		if from1 == from2 && from1 != "UNKNOWN" {
			fmt.Printf("found consecutive txs from %s:\n", from1)
			fmt.Printf("	[%d] %s\n", i, txs[i].Hash().Hex())
			fmt.Printf("	[%d] %s\n", i+1, txs[i+1].Hash().Hex())

			if i+2 < len(txs) {
				from3 := getFrom(txs[i+2])
				if from3 == from1 {
					fmt.Printf("	[%d] %s\n", i+2, txs[i+2].Hash().Hex())
				}
			}
			fmt.Println()
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
