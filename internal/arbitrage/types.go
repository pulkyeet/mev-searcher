package arbitrage

import (
	"math/big"
	"github.com/ethereum/go-ethereum/common"
)

// a Pool represents a uniswapv2 style AMM pool

type Pool struct {
	Address common.Address
	Token0 common.Address
	Token1 common.Address
	Reserve0 *big.Int
	Reserve1 *big.Int
	DEX string
}

// Pairpools groups pools that trade same token pair
type PairPools struct {
	Token0 common.Address
	Token1 common.Address
	Token0Dec int
	Token1Dec int
	Pools []*Pool
}

// price represents the price of one token in terms of another
type Price struct {
	PoolAddress common.Address
	DEX string
	Token1PerToken0 *big.Float
	Token0PerToken1 *big.Float
}

// opportunity represents a detected arbitrade opportunity
type Opportunity struct {
	Pair string
	BuyPool *Pool
	SellPool *Pool
	PriceDiff float64
	EstProfit *big.Int
	OptimalIn *big.Int
	BlockNumber uint64
}