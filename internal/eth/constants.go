package eth

import (
	"github.com/ethereum/go-ethereum/common"
)

// Token addresses — Ethereum mainnet
var (
	WETHAddress = common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	USDCAddress = common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	USDTAddress = common.HexToAddress("0xdAC17F958D2ee523a2206206994597C13D831ec7")
	DAIAddress  = common.HexToAddress("0x6B175474E89094C44Da98b954EedeAC495271d0F")
	WBTCAddress = common.HexToAddress("0x2260FAC5E5542a773Aa44fBCfeDf7C193bc2C599")
)

const (
	WETHDecimals = 18
	USDCDecimals = 6
	USDTDecimals = 6
	DAIDecimals  = 18
	WBTCDecimals = 8
)

// TokenInfo bundles address + decimals for easy lookup
type TokenInfo struct {
	Address  common.Address
	Decimals int
	Symbol   string
}

// KnownTokens — lookup by symbol string
var KnownTokens = map[string]TokenInfo{
	"WETH": {WETHAddress, WETHDecimals, "WETH"},
	"USDC": {USDCAddress, USDCDecimals, "USDC"},
	"USDT": {USDTAddress, USDTDecimals, "USDT"},
	"DAI":  {DAIAddress, DAIDecimals, "DAI"},
	"WBTC": {WBTCAddress, WBTCDecimals, "WBTC"},
}

// DEXConfig — factory + init code hash is all you need to derive ANY pair address
type DEXConfig struct {
	Name         string
	Factory      common.Address
	InitCodeHash [32]byte
}

// KnownDEXes — all tracked Uniswap V2 forks on Ethereum mainnet
var KnownDEXes = []DEXConfig{
	{
		Name:         "uniswap",
		Factory:      common.HexToAddress("0x5C69bEe701ef814a2B6a3EDD4B1652CB9cc5aA6f"),
		InitCodeHash: hexToBytes32("96e8ac4277198ff8b6f785478aa9a39f403cb768dd02cbee326c3e7da348845f"),
	},
	{
		Name:         "sushiswap",
		Factory:      common.HexToAddress("0xC0AEe478e3658e2610c5F7A4A2E1777cE9e4f2Ac"),
		InitCodeHash: hexToBytes32("e18a34eb0e04b04f7a0ac29a6e80748dca96319b42c54d679cb821dca90c6303"),
	},
	{
		Name:         "shibaswap",
		Factory:      common.HexToAddress("0x115934131916C8b277DD010Ee02de363c09d037c"),
		InitCodeHash: hexToBytes32("65d1a3b1e46c6e4f1be1ad5f99ef14dc488ae0549dc97db9b30afe2241ce1c7a"),
	},
}

func hexToBytes32(s string) [32]byte {
	var b [32]byte
	copy(b[:], common.FromHex(s))
	return b
}

// Uniswap V2 Pair ABI — getReserves only (token0/token1 no longer needed)
const UniswapV2PairABI = `[
	{
		"constant": true,
		"inputs": [],
		"name": "getReserves",
		"outputs": [
			{"internalType": "uint112", "name": "reserve0", "type": "uint112"},
			{"internalType": "uint112", "name": "reserve1", "type": "uint112"},
			{"internalType": "uint32",  "name": "blockTimestampLast", "type": "uint32"}
		],
		"payable": false,
		"stateMutability": "view",
		"type": "function"
	}
]`