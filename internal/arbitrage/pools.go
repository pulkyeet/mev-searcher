package arbitrage

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pulkyeet/mev-searcher/internal/eth"
)

// computePairAddress derives pair address via CREATE2 — zero RPC calls
func ComputePairAddress(dex eth.DEXConfig, token0, token1 common.Address) common.Address {
	// salt = keccak256(token0 ++ token1)  — tokens must already be sorted
	salt := crypto.Keccak256Hash(append(token0.Bytes(), token1.Bytes()...))

	// CREATE2: keccak256(0xff ++ factory ++ salt ++ initCodeHash)[12:]
	data := append([]byte{0xff}, dex.Factory.Bytes()...)
	data = append(data, salt.Bytes()...)
	data = append(data, dex.InitCodeHash[:]...)

	return common.BytesToAddress(crypto.Keccak256(data)[12:])
}

// sortTokens returns (token0, dec0, token1, dec1) in ascending address order
// Uniswap V2 always stores lower address as token0
func sortTokens(
	tokenA common.Address, tokenADec int,
	tokenB common.Address, tokenBDec int,
) (common.Address, int, common.Address, int) {
	if bytes.Compare(tokenA.Bytes(), tokenB.Bytes()) < 0 {
		return tokenA, tokenADec, tokenB, tokenBDec
	}
	return tokenB, tokenBDec, tokenA, tokenADec
}

// FetchReserves — unchanged, calls getReserves() on a pool contract
func FetchReserves(
	ctx context.Context,
	client *eth.Client,
	poolAddress common.Address,
	blockNum *big.Int,
) (reserve0, reserve1 *big.Int, err error) {
	contractABI, err := abi.JSON(strings.NewReader(eth.UniswapV2PairABI))
	if err != nil {
		return nil, nil, fmt.Errorf("parse ABI: %w", err)
	}

	data, err := contractABI.Pack("getReserves")
	if err != nil {
		return nil, nil, fmt.Errorf("pack getReserves: %w", err)
	}

	result, err := client.CallContract(ctx, ethereum.CallMsg{To: &poolAddress, Data: data}, blockNum)
	if err != nil {
		return nil, nil, fmt.Errorf("call contract: %w", err)
	}

	unpacked, err := contractABI.Unpack("getReserves", result)
	if err != nil {
		return nil, nil, fmt.Errorf("unpack reserves: %w", err)
	}
	if len(unpacked) < 2 {
		return nil, nil, fmt.Errorf("unexpected unpack length: %d", len(unpacked))
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

// LoadPool — no longer calls FetchTokens, caller provides token0/token1
func LoadPool(
	ctx context.Context,
	client *eth.Client,
	poolAddress common.Address,
	dex string,
	blockNum *big.Int,
	token0, token1 common.Address,
) (*Pool, error) {
	reserve0, reserve1, err := FetchReserves(ctx, client, poolAddress, blockNum)
	if err != nil {
		return nil, fmt.Errorf("fetch reserves: %w", err)
	}

	return &Pool{
		Address:  poolAddress,
		Token0:   token0,
		Token1:   token1,
		Reserve0: reserve0,
		Reserve1: reserve1,
		DEX:      dex,
	}, nil
}

// GetPairPools replaces all GetWETH*Pools — works for any pair on any known DEX
// Pools with zero reserves (inactive) are skipped silently
func GetPairPools(
	ctx context.Context,
	client *eth.Client,
	blockNum *big.Int,
	tokenA common.Address, tokenADec int,
	tokenB common.Address, tokenBDec int,
) (*PairPools, error) {
	token0, token0Dec, token1, token1Dec := sortTokens(tokenA, tokenADec, tokenB, tokenBDec)

	pools := make([]*Pool, 0, len(eth.KnownDEXes))
	for _, dex := range eth.KnownDEXes {
		pairAddr := ComputePairAddress(dex, token0, token1)

		pool, err := LoadPool(ctx, client, pairAddr, dex.Name, blockNum, token0, token1)
		if err != nil {
			// Pool likely doesn't exist on this DEX — skip, don't fail all
			fmt.Printf("  [skip] %s %s pool: %v\n", dex.Name, pairAddr.Hex()[:10], err)
			continue
		}

		// Skip inactive pools (zero reserves = no liquidity deployed)
		if pool.Reserve0.Sign() == 0 || pool.Reserve1.Sign() == 0 {
			fmt.Printf("  [skip] %s — zero reserves\n", dex.Name)
			continue
		}

		pools = append(pools, pool)
	}

	if len(pools) < 2 {
		return nil, fmt.Errorf("need at least 2 active pools for arbitrage, found %d", len(pools))
	}

	return &PairPools{
		Token0:    token0,
		Token1:    token1,
		Token0Dec: token0Dec,
		Token1Dec: token1Dec,
		Pools:     pools,
	}, nil
}