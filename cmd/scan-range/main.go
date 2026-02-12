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
)

func main() {
	_ = godotenv.Load("../../.env")

	startBlock := flag.Uint64("start", 17000000, "Start block")
	endBlock   := flag.Uint64("end", 17001000, "End block")
	pair       := flag.String("pair", "WETH/USDC", "Trading pair (e.g. WETH/USDC, WETH/DAI, WETH/WBTC)")
	step       := flag.Uint64("step", 100, "Block step size")
	flag.Parse()

	client, err := eth.NewClient()
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	parts := strings.Split(*pair, "/")
	if len(parts) != 2 {
		log.Fatalf("Invalid pair format: %s (use e.g. WETH/USDC)", *pair)
	}
	tokenAInfo, okA := eth.KnownTokens[parts[0]]
	tokenBInfo, okB := eth.KnownTokens[parts[1]]
	if !okA || !okB {
		log.Fatalf("Unknown token in pair: %s (known: WETH, USDC, USDT, DAI, WBTC)", *pair)
	}

	ctx := context.Background()
	gasPrice := big.NewInt(5e9)
	gasLimit := big.NewInt(300000)

	fmt.Printf("Scanning blocks %d to %d (step: %d) for %s opportunities...\n",
		*startBlock, *endBlock, *step, *pair)
	fmt.Printf("(Checking pre-MEV state at block N-1)\n\n")

	foundCount   := 0
	checkedCount := 0

	for block := *startBlock; block <= *endBlock; block += *step {
		checkedCount++

		if checkedCount%10 == 0 {
			fmt.Printf("Checked %d blocks, found %d opportunities...\n", checkedCount, foundCount)
		}

		preMEVBlock := new(big.Int).SetUint64(block - 1)

		pools, err := arbitrage.GetPairPools(ctx, client, preMEVBlock,
			tokenAInfo.Address, tokenAInfo.Decimals,
			tokenBInfo.Address, tokenBInfo.Decimals)
		if err != nil {
			continue
		}

		prices := arbitrage.GetPoolPrices(pools)
		if len(prices) >= 2 {
			diff := arbitrage.ComparePrices(prices[0].Token1PerToken0, prices[1].Token1PerToken0)
			if diff > 0.05 {
				fmt.Printf("Block %d: Spread %.4f%% (checking profitability...)\n", block, diff)
			}
		}

		opp, err := arbitrage.DetectOpportunity(pools, gasPrice, gasLimit)
		if err != nil || opp == nil {
			continue
		}

		foundCount++
		divisor := new(big.Float).SetInt(
			new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(pools.Token0Dec)), nil),
		)
		token0Sym := parts[0]
		for sym, info := range eth.KnownTokens {
			if info.Address == pools.Token0 {
				token0Sym = sym
				break
			}
		}

		fmt.Printf("\n🎯 BLOCK %d - PROFITABLE ARB FOUND!\n", block)
		fmt.Printf("   Buy:    %s\n", opp.BuyPool.DEX)
		fmt.Printf("   Sell:   %s\n", opp.SellPool.DEX)
		fmt.Printf("   Spread: %.4f%%\n", opp.PriceDiff)
		fmt.Printf("   Input:  %s %s\n", token0Sym,
			new(big.Float).Quo(new(big.Float).SetInt(opp.OptimalIn), divisor).Text('f', 6))
		fmt.Printf("   Profit: %s %s\n\n", token0Sym,
			new(big.Float).Quo(new(big.Float).SetInt(opp.EstProfit), divisor).Text('f', 6))
	}

	fmt.Printf("\n================================================\n")
	fmt.Printf("Scan complete! Blocks checked: %d | Opportunities: %d\n", checkedCount, foundCount)
}