package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/joho/godotenv"
	"github.com/pulkyeet/mev-searcher/internal/arbitrage"
	"github.com/pulkyeet/mev-searcher/internal/eth"
	"github.com/pulkyeet/mev-searcher/internal/simulator"
)

func main() {
	_ = godotenv.Load("../../.env")

	blockNum := flag.Uint64("block", 18000000, "block number to scan")
	simulateFlag := flag.Bool("simulate", false, "Simulate the arbitrage bundle")
	pair := flag.String("pair", "WETH/USDC", "Trading pair (WETH/USDC or WETH/USDT)")
	flag.Parse()

	client, err := eth.NewClient()
	if err != nil {
		log.Fatalf("failed to connect to Ethereum: %v", err)
	}

	ctx := context.Background()
	blockBigInt := new(big.Int).SetUint64(*blockNum)

	// Fork to block N-1 to see pre-MEV state
	preMEVBlock := new(big.Int).Sub(blockBigInt, big.NewInt(1))

	fmt.Printf("scanning block %d (forking at %d) for %s arbitrage opportunities...\n\n",
		*blockNum, preMEVBlock.Uint64(), *pair)

	parts := strings.Split(*pair, "/")
	if len(parts) != 2 {
		log.Fatalf("invalid pair format: %s (use e.g. WETH/USDC)", *pair)
	}
	tokenAInfo, okA := eth.KnownTokens[parts[0]]
	tokenBInfo, okB := eth.KnownTokens[parts[1]]
	if !okA || !okB {
		log.Fatalf("unknown token in pair: %s (known: WETH, USDC, USDT, DAI, WBTC)", *pair)
	}

	pools, err := arbitrage.GetPairPools(ctx, client, preMEVBlock,
		tokenAInfo.Address, tokenAInfo.Decimals,
		tokenBInfo.Address, tokenBInfo.Decimals)
	if err != nil {
		log.Fatalf("failed to load pools: %v", err)
	}

	// Resolve display symbols from pool's sorted token0/token1
	token0Symbol, token1Symbol := parts[0], parts[1]
	for sym, info := range eth.KnownTokens {
		if info.Address == pools.Token0 {
			token0Symbol = sym
		}
		if info.Address == pools.Token1 {
			token1Symbol = sym
		}
	}
	_ = token1Symbol

	// Display pool reserves
	fmt.Println("Pool Reserves at block", preMEVBlock.Uint64(), ":")
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

	gasPrice := big.NewInt(5e9)
	gasLimit := big.NewInt(300000)

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
		divisor := new(big.Float).SetInt(
			new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(pools.Token0Dec)), nil),
		)
		fmt.Printf("  Input:      %s %s\n", token0Symbol,
			new(big.Float).Quo(new(big.Float).SetInt(opp.OptimalIn), divisor).Text('f', 6))
		fmt.Printf("  Est Profit: %s %s\n", token0Symbol,
			new(big.Float).Quo(new(big.Float).SetInt(opp.EstProfit), divisor).Text('f', 6))

		if *simulateFlag {
			fmt.Println("\n🔧 Simulating arbitrage bundle...")

			fork, err := simulator.NewStateFork(client, preMEVBlock)
			if err != nil {
				log.Fatalf("Failed to fork: %v", err)
			}
			defer fork.Close()

			arbExec := arbitrage.NewArbExecutor(fork)
			simResult, err := arbExec.SimulateArbitrage(opp)
			if err != nil {
				log.Fatalf("Simulation error: %v", err)
			}

			fmt.Println("\n📊 Simulation Results:")
			fmt.Println("======================")
			fmt.Println(simResult.CompareResults())
			fmt.Printf("Gas Used: %d\n", simResult.GasUsed)

			fork.PrintStats()
		} else {
			fmt.Println("\n💡 Add --simulate flag to test this opportunity")
		}
	}

	fmt.Println("\n✅ Scan complete")
}
