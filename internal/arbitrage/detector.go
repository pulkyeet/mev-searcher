package arbitrage

import (
	"fmt"
	"math/big"
)

// checks if arbitrage is profitable between two pools

func DetectOpportunity(pair *PairPools, gasPrice, gasLimit *big.Int) (*Opportunity, error) {
	if len(pair.Pools)<2 {
		return nil, fmt.Errorf("need at least 2 pools to detect arbitrage")
	}

	prices := GetPoolPrices(pair)
	if len(prices)<2 {
		return nil, fmt.Errorf("failed to calculate prices")
	}

	priceDiff := ComparePrices(prices[0].Token1PerToken0, prices[1].Token1PerToken0)

	if priceDiff < 0.05 {
		return nil, nil
	}

	var cheapPool, expensivePool *Pool
	if prices[0].Token1PerToken0.Cmp(prices[1].Token1PerToken0) < 0{
		cheapPool = pair.Pools[0]
		expensivePool = pair.Pools[1]
	} else {
		cheapPool = pair.Pools[1]
		expensivePool = pair.Pools[0]
	}

	// search range is $1000 to $1000000
	minAmount := new(big.Int).Mul(big.NewInt(100), big.NewInt(1e6))    
	maxAmount := new(big.Int).Mul(big.NewInt(10000), big.NewInt(1e6))

	optimalIn, grossProfit := FindOptimalInput(cheapPool, expensivePool, false, minAmount, maxAmount)

	gasCostWei := new(big.Int).Mul(gasPrice, gasLimit)

	// convert ETH gas cost to USDC using pool price
	gasCostFloat := new(big.Float).SetInt(gasCostWei)
	gasCostFloat.Quo(gasCostFloat, big.NewFloat(1e18)) // wei to ETH
	
	usdcPerWeth := new(big.Float).Quo(big.NewFloat(1), prices[0].Token1PerToken0)
	gasCostFloat.Mul(gasCostFloat, usdcPerWeth)
	gasCostFloat.Mul(gasCostFloat, big.NewFloat(1e6)) 
	
	//gasCostUSDC, _ := gasCostFloat.Int(nil)
	gasCostUSDC := big.NewInt(0)  // Ignore gas for now
	
	netProfit := new(big.Int).Sub(grossProfit, gasCostUSDC)

	// check if profitable
	if netProfit.Cmp(big.NewInt(0)) <= 0 {
		return nil, nil // Not profitable after gas
	}

	return &Opportunity{
		Pair:        fmt.Sprintf("WETH/%s", pair.Pools[0].DEX),
		BuyPool:     cheapPool,
		SellPool:    expensivePool,
		PriceDiff:   priceDiff,
		EstProfit:   netProfit,
		OptimalIn:   optimalIn,
		BlockNumber: 0, // Will be set by caller
	}, nil
}