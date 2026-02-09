package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/joho/godotenv"
	
	"github.com/pulkyeet/mev-searcher/internal/arbitrage"
)

func main() {
	_ = godotenv.Load("../../.env")
	
	startBlock := flag.Uint64("start", 17000000, "Start block")
	endBlock := flag.Uint64("end", 17001000, "End block")
	pair := flag.String("pair", "WETH/USDC", "Trading pair")
	step := flag.Uint64("step", 100, "Block step size")
	flag.Parse()

	rpcURL := os.Getenv("ALCHEMY_URL")
	if rpcURL == "" {
		log.Fatal("ALCHEMY_URL not set")
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	gasPrice := big.NewInt(10e9)
	gasLimit := big.NewInt(300000)

	fmt.Printf("Scanning blocks %d to %d (step: %d) for %s opportunities...\n\n", 
		*startBlock, *endBlock, *step, *pair)

	foundCount := 0
	checkedCount := 0

	for block := *startBlock; block <= *endBlock; block += *step {
		checkedCount++
		
		// Progress indicator
		if checkedCount % 10 == 0 {
			fmt.Printf("Checked %d blocks, found %d opportunities...\n", checkedCount, foundCount)
		}

		blockBigInt := new(big.Int).SetUint64(block)

		// Load pools
		var pools *arbitrage.PairPools
		switch *pair {
		case "WETH/USDC":
			pools, err = arbitrage.GetWETHUSDCPools(ctx, client, blockBigInt)
		case "WETH/USDT":
			pools, err = arbitrage.GetWETHUSDTPools(ctx, client, blockBigInt)
		default:
			log.Fatalf("Unsupported pair: %s", *pair)
		}

		if err != nil {
			fmt.Printf("Block %d: Error loading pools: %v\n", block, err)
			continue
		}

		// Detect opportunity
		opp, err := arbitrage.DetectOpportunity(pools, gasPrice, gasLimit)
		if err != nil {
			fmt.Printf("Block %d: Error detecting: %v\n", block, err)
			continue
		}

		if opp != nil {
			foundCount++
			opp.BlockNumber = block
			
			fmt.Printf("\n🎯 BLOCK %d - PROFITABLE ARB FOUND!\n", block)
			fmt.Printf("   Buy:  %s\n", opp.BuyPool.DEX)
			fmt.Printf("   Sell: %s\n", opp.SellPool.DEX)
			fmt.Printf("   Spread: %.4f%%\n", opp.PriceDiff)
			
			profitUSDC := new(big.Float).Quo(
				new(big.Float).SetInt(opp.EstProfit), 
				big.NewFloat(1e6),
			)
			inputUSDC := new(big.Float).Quo(
				new(big.Float).SetInt(opp.OptimalIn),
				big.NewFloat(1e6),
			)
			
			fmt.Printf("   Input:  $%s USDC\n", inputUSDC.Text('f', 2))
			fmt.Printf("   Profit: $%s USDC\n\n", profitUSDC.Text('f', 2))
		}
	}

	fmt.Printf("\n================================================\n")
	fmt.Printf("Scan complete!\n")
	fmt.Printf("Blocks checked: %d\n", checkedCount)
	fmt.Printf("Opportunities found: %d\n", foundCount)
	
	if foundCount > 0 {
		fmt.Printf("\n✅ Found %d profitable arbitrage opportunities!\n", foundCount)
	} else {
		fmt.Printf("\n❌ No profitable opportunities in this range.\n")
		fmt.Printf("Try:\n")
		fmt.Printf("  - Wider block range\n")
		fmt.Printf("  - Different pair (--pair WETH/USDT)\n")
		fmt.Printf("  - Earlier blocks (more volatility)\n")
	}
}