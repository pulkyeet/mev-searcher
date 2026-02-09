package eth

import (
	"github.com/ethereum/go-ethereum/common"
)

// token addresses Ethereum mainnet

var (
	WETHAddress = common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	USDCAddress  = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	USDTAddress  = common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7")
)

// uniswap v2 addresses

var (
	UniV2_WETH_USDC = common.HexToAddress("0xB4e16d0168e52d35CaCD2c6185b44281Ec28C9Dc")
	UniV2_WETH_USDT = common.HexToAddress("0x0d4a11d5EEaaC28EC3F61d100daF4d40471f1852")
)

// Sushiswap pool addresses
var (
	Sushi_WETH_USDC = common.HexToAddress("0x397FF1542f962076d0BFE58eA045FfA2d347ACa0")
	Sushi_WETH_USDT = common.HexToAddress("0x06da0fd433C1A5d7a4faa01111c044910A184553")
)

// Token decimals
const (
	WETHDecimals = 18
	USDCDecimals = 6
	USDTDecimals = 6
)

// Uniswap V2 Pair ABI - just getReserves function
const UniswapV2PairABI = `[
	{
		"constant": true,
		"inputs": [],
		"name": "getReserves",
		"outputs": [
			{"internalType": "uint112", "name": "reserve0", "type": "uint112"},
			{"internalType": "uint112", "name": "reserve1", "type": "uint112"},
			{"internalType": "uint32", "name": "blockTimestampLast", "type": "uint32"}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "token0",
		"outputs": [{"internalType": "address", "name": "", "type": "address"}],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	},
	{
		"constant": true,
		"inputs": [],
		"name": "token1",
		"outputs": [{"internalType": "address", "name": "", "type": "address"}],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`