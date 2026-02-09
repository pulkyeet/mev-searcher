package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"

	"github.com/joho/godotenv"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pulkyeet/mev-searcher/internal/arbitrage"
)

func main() {
	_ = godotenv.Load("../../.env")
	// flags
	blockNum := flag.Uint64("block", 18000000, "block number to scan")
	pair := flag.String("pair", "WETH/USDC", "Trading pair (WETH/USDC or WETH/USDT)")
	flag.Parse()

	// get rpc url from env
	rpcURL := os.Getenv("ALCHEMY_URL")
	if rpcURL=="" {
		log.Fatal("ALCHEMY_URL environment variable not set")
	}

	client, err := ethclient.Dial(rpcURL)
	if err!=nil {
		log.Fatalf("failed to connect to Ethereum: %v", err)
	}
	defer client.Close()

	ctx := context.Background()
	blockBigInt := new(big.Int).SetUint64(*blockNum)

	fmt.Printf("scanning block %d for %s arbitrage opportunities...\n\n", *blockNum, *pair)

	// load pools based on pair
	var pools *arbitrage.PairPools
	switch *pair {
	case "WETH/USDC":
		pools, err = arbitrage.GetWETHUSDCPools(ctx, client, blockBigInt)
	case "WETH/USDT":
		pools, err = arbitrage.GetWETHUSDTPools(ctx, client, blockBigInt)
	default:
		log.Fatalf("unsupported pair: %s (use WETH/USDC or WETH/USDT)", *pair)
	}

	if err!=nil {
		log.Fatalf("failed to load pools: %v", err)
	}

	// Display pool reserves
	fmt.Println("Pool Reserves:")
	fmt.Println("==============")
	for _, pool := range pools.Pools {
		fmt.Printf("\n%s (%s):\n", pool.DEX, pool.Address.Hex())
		fmt.Printf("  Reserve0: %s\n", pool.Reserve0.String())
		fmt.Printf("  Reserve1: %s\n", pool.Reserve1.String())
	}
	// Calculate prices
	prices := arbitrage.GetPoolPrices(pools)

	fmt.Println("\n\nPrices:")
	fmt.Println("=======")
	for i, price := range prices {
		pool := pools.Pools[i]
		fmt.Printf("\n%s (%s):\n", pool.DEX, pool.Address.Hex())
		fmt.Printf("  Token1/Token0: %s\n", price.Token1PerToken0.Text('f', 6))
		fmt.Printf("  Token0/Token1: %s\n", price.Token0PerToken1.Text('f', 6))
	}

	// Compare prices
	if len(prices) >= 2 {
		fmt.Println("\n\nPrice Comparison:")
		fmt.Println("=================")
		
		// Compare using Token1PerToken0 (e.g., WETH price in USDC)
		diff := arbitrage.ComparePrices(
			prices[0].Token1PerToken0,
			prices[1].Token1PerToken0,
		)
		
		fmt.Printf("Price difference: %.4f%%\n", diff)
		
		if diff > 0.1 {
			fmt.Printf("\n🚨 ARBITRAGE OPPORTUNITY DETECTED! 🚨\n")
			fmt.Printf("Price spread: %.4f%%\n", diff)
		} else {
			fmt.Printf("\nNo significant arbitrage opportunity (threshold: 0.1%%)\n")
		}
	}

	gasPrice := big.NewInt(30e9)    // 30 gwei
	gasLimit := big.NewInt(300000)   // ~300k gas for 2 swaps
	
	opp, err := arbitrage.DetectOpportunity(pools, gasPrice, gasLimit)
	if err != nil {
		log.Fatalf("Failed to detect opportunity: %v", err)
	}

	if opp == nil {
		fmt.Println("\n\nNo profitable arbitrage opportunity found")
		fmt.Println("(Spread exists but not enough to cover gas + fees)")
	} else {
		fmt.Println("\n\n🚨 PROFITABLE ARBITRAGE DETECTED! 🚨")
		fmt.Println("=====================================")
		fmt.Printf("Buy from:  %s (%s)\n", opp.BuyPool.DEX, opp.BuyPool.Address.Hex())
		fmt.Printf("Sell to:   %s (%s)\n", opp.SellPool.DEX, opp.SellPool.Address.Hex())
		fmt.Printf("Price diff: %.4f%%\n", opp.PriceDiff)
		fmt.Printf("\nOptimal trade:\n")
		fmt.Printf("  Input:  %s USDC ($%s)\n", 
			opp.OptimalIn.String(),
			new(big.Float).Quo(new(big.Float).SetInt(opp.OptimalIn), big.NewFloat(1e6)).Text('f', 2))
		fmt.Printf("  Profit: %s USDC ($%s)\n",
			opp.EstProfit.String(),
			new(big.Float).Quo(new(big.Float).SetInt(opp.EstProfit), big.NewFloat(1e6)).Text('f', 2))
	}

	fmt.Println("\n✅ Scan complete")
}