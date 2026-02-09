package arbitrage

import (
	"math/big"
)

// calculates price of token1 in terms of token0 adjusting for decimals
func CalculatePrice(reserve0, reserve1 *big.Int, decimals0, decimals1 int) *big.Float {
	r0 := new(big.Float).SetInt(reserve0)
	r1 := new(big.Float).SetInt(reserve1)

	// price = reserve0/reserve1 * 10^(decimals1-decimals0)

	decimalAdj := new(big.Float).SetInt(
		new(big.Int).Exp(
			big.NewInt(10),
			big.NewInt(int64(decimals1-decimals0)),
			nil,
		),
	)

	price := new(big.Float).Quo(r0, r1)
	price.Mul(price, decimalAdj)

	return price
}

// GetPoolPrices calculates prices for all pools in a pair
func GetPoolPrices(pair *PairPools) []*Price {
	prices := make([]*Price, 0, len(pair.Pools))

	for _, pool := range pair.Pools {
		token1PerToken0 := CalculatePrice(
			pool.Reserve0,
			pool.Reserve1,
			pair.Token0Dec,
			pair.Token1Dec,
		)

		token0PerToken1 := CalculatePrice(
			pool.Reserve1,
			pool.Reserve0,
			pair.Token1Dec,
			pair.Token0Dec,
		)

		prices = append(prices, &Price{
			PoolAddress: pool.Address,
			DEX: pool.DEX,
			Token1PerToken0: token1PerToken0,
			Token0PerToken1: token0PerToken1,
		})
	}
	return prices
}

// returns difference of price between pools (percentage)

func ComparePrices(price1, price2 *big.Float) float64 {
	cmp := price1.Cmp(price2)
	if cmp==0 {
		return 0.0
	}
	var higher, lower *big.Float
	if cmp>0 {
		higher = price1
		lower = price2
	} else {
		higher = price2
		lower = price1
	}

	diff := new(big.Float).Sub(higher, lower)
	pctDiff := new(big.Float).Quo(diff, lower)
	pctDiff.Mul(pctDiff, big.NewFloat(100.0))

	result, _ := pctDiff.Float64()
	return result
}

// calculates output amount for a uniswapv2 swap including a 0.3% fee

func GetAmountOut(amountIn, reserveIn, reserveOut *big.Int) *big.Int {
	if amountIn.Cmp(big.NewInt(0)) <=0 {
		return big.NewInt(0)
	}
	if reserveIn.Cmp(big.NewInt(0))<=0 || reserveOut.Cmp(big.NewInt(0))<=0 {
		return big.NewInt(0)
	}

	amountInWithFee := new(big.Int).Mul(amountIn,big.NewInt(997))
	numerator := new(big.Int).Mul(amountInWithFee, reserveOut)

	denominator := new(big.Int).Mul(reserveIn, big.NewInt(1000))
	denominator.Add(denominator, amountInWithFee)

	amountOut := new(big.Int).Div(numerator, denominator)

	return amountOut
}

// calculates profit on a given input amount

func SimulateArbitrage(
	amountIn *big.Int,
	cheapPool, expensivePool *Pool,
	token0IsBuyToken bool,
) *big.Int {
	var buyReserveIn, buyReserveOut, sellReserveIn, sellReserveOut *big.Int

	if token0IsBuyToken {
		// Buying token0 with token1
		buyReserveIn = cheapPool.Reserve1
		buyReserveOut = cheapPool.Reserve0
		sellReserveIn = expensivePool.Reserve0
		sellReserveOut = expensivePool.Reserve1
	} else {
		// Buying token1 with token0
		buyReserveIn = cheapPool.Reserve0
		buyReserveOut = cheapPool.Reserve1
		sellReserveIn = expensivePool.Reserve1
		sellReserveOut = expensivePool.Reserve0
	}

	amountBought := GetAmountOut(amountIn, buyReserveIn, buyReserveOut)
	amountOut := GetAmountOut(amountBought, sellReserveIn, sellReserveOut)

	profit := new(big.Int).Sub(amountOut, amountIn)

	return profit
}

// searches for the input amount that maximises profit
func FindOptimalInput(
	cheapPool, expensivePool *Pool,
	token0IsBuyToken bool,
	minAmount, maxAmount *big.Int,
) (optimalInput, maxProfit *big.Int) {
	// Binary search parameters
	steps := 20 // Number of iterations
	
	left := new(big.Int).Set(minAmount)
	right := new(big.Int).Set(maxAmount)
	
	bestInput := new(big.Int).Set(minAmount)
	bestProfit := SimulateArbitrage(minAmount, cheapPool, expensivePool, token0IsBuyToken)
	
	for i := 0; i < steps; i++ {
		// Try 1/3 point
		third := new(big.Int).Sub(right, left)
		third.Div(third, big.NewInt(3))
		mid1 := new(big.Int).Add(left, third)
		
		// Try 2/3 point
		mid2 := new(big.Int).Add(left, new(big.Int).Mul(third, big.NewInt(2)))
		
		profit1 := SimulateArbitrage(mid1, cheapPool, expensivePool, token0IsBuyToken)
		profit2 := SimulateArbitrage(mid2, cheapPool, expensivePool, token0IsBuyToken)
		
		// Update best
		if profit1.Cmp(bestProfit) > 0 {
			bestProfit = profit1
			bestInput = mid1
		}
		if profit2.Cmp(bestProfit) > 0 {
			bestProfit = profit2
			bestInput = mid2
		}
		
		// Narrow search range
		if profit1.Cmp(profit2) > 0 {
			right = mid2
		} else {
			left = mid1
		}
	}
	
	return bestInput, bestProfit
}