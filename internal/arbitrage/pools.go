package arbitrage

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/pulkyeet/mev-searcher/internal/eth"

)

// fetchreserves gets reserves for a pool at a specific block
func FetchReserves(
	ctx context.Context,
	client *eth.Client,
	poolAddress common.Address,
	blockNum *big.Int,
) (reserve0, reserve1 *big.Int, err error) {
	// Parse ABI
	contractABI, err := abi.JSON(strings.NewReader(eth.UniswapV2PairABI))
	if err != nil {
		return nil, nil, fmt.Errorf("parse ABI: %w", err)
	}

	// Pack getReserves call
	data, err := contractABI.Pack("getReserves")
	if err != nil {
		return nil, nil, fmt.Errorf("pack getReserves: %w", err)
	}

	// Call contract
	msg := ethereum.CallMsg{
		To:   &poolAddress,
		Data: data,
	}
	
	result, err := client.CallContract(ctx, msg, blockNum)
	if err != nil {
		return nil, nil, fmt.Errorf("call contract: %w", err)
	}

	// Unpack into map (more flexible)
	unpacked, err := contractABI.Unpack("getReserves", result)
	if err != nil {
		return nil, nil, fmt.Errorf("unpack reserves: %w", err)
	}

	// Extract values from unpacked slice
	if len(unpacked) < 2 {
		return nil, nil, fmt.Errorf("unexpected unpack result length: %d", len(unpacked))
	}

	reserve0, ok := unpacked[0].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("reserve0 type assertion failed")
	}

	reserve1, ok = unpacked[1].(*big.Int)
	if !ok {
		return nil, nil, fmt.Errorf("reserve1 type assertion failed")
	}

	return reserve0, reserve1, nil
}

// fetchTokens gets token0 and token1 addresses for a pool
func FetchTokens(ctx context.Context, client *eth.Client, poolAddress common.Address, blockNum *big.Int) (token0, token1 common.Address, err error) {
	contractABI, err := abi.JSON(strings.NewReader(eth.UniswapV2PairABI))
	if err!=nil {
		return common.Address{}, common.Address{}, fmt.Errorf("parse abi: %w", err)
	}

	// getting token0
	data0, err := contractABI.Pack("token0")
	if err!=nil {
		return common.Address{}, common.Address{}, fmt.Errorf("pack token0: %w", err)
	}

	msg0 := ethereum.CallMsg{To: &poolAddress, Data: data0}
	result0, err := client.CallContract(ctx, msg0, blockNum)
	if err!=nil {
		return common.Address{}, common.Address{}, fmt.Errorf("call token0: %w", err)
	}
	token0 = common.BytesToAddress(result0)

	data1, err := contractABI.Pack("token1")
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("pack token1: %w", err)
	}
	
	msg1 := ethereum.CallMsg{To: &poolAddress, Data: data1}
	result1, err := client.CallContract(ctx, msg1, blockNum)
	if err != nil {
		return common.Address{}, common.Address{}, fmt.Errorf("call token1: %w", err)
	}
	token1 = common.BytesToAddress(result1)

	return token0, token1, nil
}

// loadpool fetches complete pool state at a block

func LoadPool(ctx context.Context, client *eth.Client, poolAddress common.Address, dex string, blockNum *big.Int) (*Pool, error) {
	token0, token1, err := FetchTokens(ctx, client, poolAddress, blockNum)
	if err!=nil {
		return nil, fmt.Errorf("fetch tokens: %w", err)
	}

	reserve0, reserve1, err := FetchReserves(ctx, client, poolAddress, blockNum)
	if err!=nil {
		return nil, fmt.Errorf("fetch reserves: %w", err)
	}

	return &Pool{
		Address: poolAddress,
		Token0: token0,
		Token1: token1,
		Reserve0: reserve0,
		Reserve1: reserve1,
		DEX: dex,
	}, nil
}

// returns all tracked WETH/USDC pools
func GetWETHUSDCPools(ctx context.Context, client *eth.Client, blockNum *big.Int) (*PairPools, error) {
	pools := make([]*Pool,0,2)

	uniPool, err := LoadPool(ctx, client, eth.UniV2_WETH_USDC, "uniswap", blockNum)
	if err!=nil {
		return nil, fmt.Errorf("load uniswap pool: %w", err)
	}
	pools = append(pools, uniPool)

	sushiPool, err := LoadPool(ctx, client, eth.Sushi_WETH_USDC, "sushiswap", blockNum)
	if err!=nil {
		return nil, fmt.Errorf("load sushiswap pool: %w", err)
	}
	pools = append(pools, sushiPool)

	return &PairPools{
		Token0: eth.USDCAddress,
		Token1: eth.WETHAddress,
		Token0Dec: eth.USDCDecimals,
		Token1Dec: eth.WETHDecimals,
		Pools: pools,
	}, nil
}

// GetWETHUSDTPools returns all tracked WETH/USDT pools
func GetWETHUSDTPools(
	ctx context.Context,
	client *eth.Client,
	blockNum *big.Int,
) (*PairPools, error) {
	pools := make([]*Pool, 0, 2)

	uniPool, err := LoadPool(ctx, client, eth.UniV2_WETH_USDT, "uniswap", blockNum)
	if err != nil {
		return nil, fmt.Errorf("load uniswap pool: %w", err)
	}
	pools = append(pools, uniPool)

	sushiPool, err := LoadPool(ctx, client, eth.Sushi_WETH_USDT, "sushiswap", blockNum)
	if err != nil {
		return nil, fmt.Errorf("load sushiswap pool: %w", err)
	}
	pools = append(pools, sushiPool)

	return &PairPools{
		Token0:    eth.USDTAddress, 
		Token1:    eth.WETHAddress,
		Token0Dec: eth.USDTDecimals,
		Token1Dec: eth.WETHDecimals,
		Pools:     pools,
	}, nil
}