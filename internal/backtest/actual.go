package backtest

import (
	"bytes"
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pulkyeet/mev-searcher/internal/arbitrage"
	"github.com/pulkyeet/mev-searcher/internal/eth"
)

var swapEventTopic = common.HexToHash("0xd78ad95fa46c994b6551d0da85fc275fe613ce37657fb8d5e3d130840159d822")

type ActualArbitrage struct {
	TxHash      common.Hash
	BlockNumber uint64
	From        common.Address
	PoolsHit    []common.Address
	GasUsed     uint64
}

// sortAddrs returns (lower, higher) by byte comparison — mirrors Uniswap's token ordering
func sortAddrs(a, b common.Address) (common.Address, common.Address) {
	if bytes.Compare(a.Bytes(), b.Bytes()) < 0 {
		return a, b
	}
	return b, a
}

// buildPairGroups computes pool addresses for all tracked pairs × all known DEXes
// Returns: tracked set (for fast lookup) + grouped slices (for pair matching)
func buildPairGroups() (map[common.Address]bool, [][]common.Address) {
	tracked := make(map[common.Address]bool)
	groups := make([][]common.Address, 0, len(trackedPairs))

	for _, p := range trackedPairs {
		token0, token1 := sortAddrs(p.tokenA, p.tokenB)
		group := make([]common.Address, 0, len(eth.KnownDEXes))
		for _, dex := range eth.KnownDEXes {
			addr := arbitrage.ComputePairAddress(dex, token0, token1)
			group = append(group, addr)
			tracked[addr] = true
		}
		groups = append(groups, group)
	}
	return tracked, groups
}

func TrackedPools() []common.Address {
	tracked, _ := buildPairGroups()
	all := make([]common.Address, 0, len(tracked))
	for addr := range tracked {
		all = append(all, addr)
	}
	return all
}

func arePairMatched(a, b common.Address) bool {
	_, groups := buildPairGroups()
	for _, group := range groups {
		hasA, hasB := false, false
		for _, addr := range group {
			if addr == a {
				hasA = true
			}
			if addr == b {
				hasB = true
			}
		}
		if hasA && hasB {
			return true
		}
	}
	return false
}

func swapDirection(log *types.Log) int {
	if len(log.Topics) < 1 || log.Topics[0] != swapEventTopic {
		return 0
	}
	if len(log.Data) < 128 {
		return 0
	}

	amount0In := new(big.Int).SetBytes(log.Data[0:32])
	amount1In := new(big.Int).SetBytes(log.Data[32:64])
	amount0Out := new(big.Int).SetBytes(log.Data[64:96])
	amount1Out := new(big.Int).SetBytes(log.Data[96:128])

	if amount0In.Sign() > 0 && amount1Out.Sign() > 0 && amount1In.Sign() == 0 && amount0Out.Sign() == 0 {
		return 1
	}
	if amount1In.Sign() > 0 && amount0Out.Sign() > 0 && amount0In.Sign() == 0 && amount1Out.Sign() == 0 {
		return -1
	}
	return 0
}

func FindActualArbitrages(ctx context.Context, client *eth.Client, blockNum uint64) ([]*ActualArbitrage, error) {
	block, err := client.BlockByNumber(ctx, new(big.Int).SetUint64(blockNum))
	if err != nil {
		return nil, fmt.Errorf("fetch block %d: %w", blockNum, err)
	}
	fmt.Printf("  Block %d: %d txs to scan\n", blockNum, len(block.Transactions()))

	tracked, _ := buildPairGroups()

	receipts, err := client.GetBlockReceipts(ctx, blockNum)
	if err != nil {
		return nil, fmt.Errorf("fetch receipts: %w", err)
	}
	receiptMap := make(map[common.Hash]*types.Receipt, len(receipts))
	for _, r := range receipts {
		receiptMap[r.TxHash] = r
	}

	totalSwaps := 0
	for _, receipt := range receipts {
		for _, log := range receipt.Logs {
			if tracked[log.Address] && len(log.Topics) > 0 && log.Topics[0] == swapEventTopic {
				totalSwaps++
			}
		}
	}
	fmt.Printf("  Block %d: %d swaps on tracked pools\n", blockNum, totalSwaps)

	var arbs []*ActualArbitrage

	for _, tx := range block.Transactions() {
		receipt, ok := receiptMap[tx.Hash()]
		if !ok {
			continue
		}

		poolSwaps := make(map[common.Address]int)
		for _, log := range receipt.Logs {
			if !tracked[log.Address] {
				continue
			}
			dir := swapDirection(log)
			if dir != 0 {
				poolSwaps[log.Address] = dir
				fmt.Printf("    SWAP on %s dir=%d tx=%s\n", log.Address.Hex()[:10], dir, tx.Hash().Hex()[:16])
			}
		}

		if len(poolSwaps) < 2 {
			continue
		}

		pools := make([]common.Address, 0, len(poolSwaps))
		for addr := range poolSwaps {
			pools = append(pools, addr)
		}

		isArb := false
		for i := 0; i < len(pools) && !isArb; i++ {
			for j := i + 1; j < len(pools); j++ {
				if arePairMatched(pools[i], pools[j]) && poolSwaps[pools[i]] != poolSwaps[pools[j]] {
					isArb = true
					break
				}
			}
		}

		if !isArb {
			continue
		}

		sender, _ := types.Sender(types.LatestSignerForChainID(tx.ChainId()), tx)
		arbs = append(arbs, &ActualArbitrage{
			TxHash:      tx.Hash(),
			BlockNumber: blockNum,
			From:        sender,
			PoolsHit:    pools,
			GasUsed:     receipt.GasUsed,
		})
	}

	fmt.Printf("  Block %d: found %d actual arbs\n", blockNum, len(arbs))
	return arbs, nil
}
