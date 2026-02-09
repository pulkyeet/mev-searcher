package arbitrage

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"strings"
)

var (
	// Uniswap V2 Router02 (same for Uni and Sushi)
	UniswapV2Router = common.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
	SushiswapRouter = common.HexToAddress("0xd9e1cE17f2641f24aE83637ab66a2cca9C378B9F")
	
	// Router ABI - only the function we need
	routerABI = `[{
		"inputs": [
			{"internalType": "uint256", "name": "amountIn", "type": "uint256"},
			{"internalType": "uint256", "name": "amountOutMin", "type": "uint256"},
			{"internalType": "address[]", "name": "path", "type": "address[]"},
			{"internalType": "address", "name": "to", "type": "address"},
			{"internalType": "uint256", "name": "deadline", "type": "uint256"}
		],
		"name": "swapExactTokensForTokens",
		"outputs": [
			{"internalType": "uint256[]", "name": "amounts", "type": "uint256[]"}
		],
		"stateMutability": "nonpayable",
		"type": "function"
	}]`
)

// creates calldata for swapExactTokensForTokens

func BuildSwapCalldata(
	amountIn *big.Int,
	amountOutMin *big.Int,
	path []common.Address,
	recipient common.Address,
	deadline *big.Int,
) ([]byte, error) {
	parsedABI, err := abi.JSON(strings.NewReader(routerABI))
	if err!=nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	calldata, err := parsedABI.Pack("swapExactTokensForTokens", amountIn, amountOutMin, path, recipient, deadline)
	if err!=nil {
		return nil, fmt.Errorf("failed to pack calldata: %w", err)
	}

	return calldata, nil
}

// return 2 swap transactions for an arbitrage

func BuildArbTransactions(
	opp *Opportunity,
	executor common.Address,
	blockTimestamp uint64,
) ([]*types.LegacyTx, error) {
	// deadline = blocktimestamp + 2 minutes
	deadline := new(big.Int).Add(big.NewInt(int64(blockTimestamp)), big.NewInt(120))

	txs := make([]*types.LegacyTx, 2)

	// tx 1: buy on cheap pool
	buyPath := []common.Address{
		opp.BuyPool.Token0, // usdc
		opp.BuyPool.Token1, // weth
	}

	// calculate expected output from buy (for amountout, use 98% for slippage)
	buyReserveIn := opp.BuyPool.Reserve0
	buyReserveOut := opp.BuyPool.Reserve1
	expectedBuyOut := GetAmountOut(opp.OptimalIn, buyReserveIn, buyReserveOut)

	buyOutMin := new(big.Int).Mul(expectedBuyOut, big.NewInt(98))
	buyOutMin.Div(buyOutMin, big.NewInt(100))

	buyRouter := getRouterAddress(opp.BuyPool.DEX)
	buyCalldata, err := BuildSwapCalldata(opp.OptimalIn, buyOutMin, buyPath, executor, deadline)
	if err!=nil {
		return nil, fmt.Errorf("failed to build buy calldata: %w", err)
	}
	txs[0] = &types.LegacyTx{
		To:       &buyRouter,
		Value:    big.NewInt(0),
		Gas:      150000,  // Estimated gas for single swap
		GasPrice: big.NewInt(30e9), // 30 gwei
		Data:     buyCalldata,
	}

	// Transaction 2: Sell on expensive pool
	// For WETH/USDC: Sell WETH for USDC
	// Path: [WETH, USDC]
	sellPath := []common.Address{
		opp.SellPool.Token1,  // WETH
		opp.SellPool.Token0,  // USDC
	}

	// Use the output from buy as input to sell
	expectedSellOut := GetAmountOut(expectedBuyOut, opp.SellPool.Reserve1, opp.SellPool.Reserve0)
	
	sellOutMin := new(big.Int).Mul(expectedSellOut, big.NewInt(98))
	sellOutMin.Div(sellOutMin, big.NewInt(100))

	sellRouter := getRouterAddress(opp.SellPool.DEX)
	sellCalldata, err := BuildSwapCalldata(
		expectedBuyOut,  // sell all WETH we bought
		sellOutMin,
		sellPath,
		executor,
		deadline,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build sell calldata: %w", err)
	}

	txs[1] = &types.LegacyTx{
		To:       &sellRouter,
		Value:    big.NewInt(0),
		Gas:      150000,
		GasPrice: big.NewInt(30e9),
		Data:     sellCalldata,
	}

	return txs, nil
}

func getRouterAddress(dex string) common.Address {
	switch dex {
	case "Uniswap V2":
		return UniswapV2Router
	case "Sushiswap":
		return SushiswapRouter
	default:
		return UniswapV2Router
	}
}