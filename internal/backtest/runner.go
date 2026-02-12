package backtest

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/pulkyeet/mev-searcher/internal/arbitrage"
	"github.com/pulkyeet/mev-searcher/internal/eth"
	"github.com/pulkyeet/mev-searcher/internal/simulator"
)

type Runner struct {
	client    *eth.Client
	mempoolDB *MempoolDB
	gasPrice  *big.Int
	gasLimit  *big.Int
}

type pairDef struct {
	name      string
	tokenA    common.Address
	tokenADec int
	tokenB    common.Address
	tokenBDec int
}

var trackedPairs = []pairDef{
	{"WETH/USDC", eth.WETHAddress, eth.WETHDecimals, eth.USDCAddress, eth.USDCDecimals},
	{"WETH/USDT", eth.WETHAddress, eth.WETHDecimals, eth.USDTAddress, eth.USDTDecimals},
	{"WETH/DAI",  eth.WETHAddress, eth.WETHDecimals, eth.DAIAddress,  eth.DAIDecimals},
	{"WETH/WBTC", eth.WETHAddress, eth.WETHDecimals, eth.WBTCAddress, eth.WBTCDecimals},
}

func NewRunner(client *eth.Client, dbPath string) (*Runner, error) {
	db, err := NewMempoolDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open mempool db: %w", err)
	}

	return &Runner{
		client:    client,
		mempoolDB: db,
		gasPrice:  big.NewInt(30e9),   // 30 gwei
		gasLimit:  big.NewInt(300000), // 300k gas
	}, nil
}

func (r *Runner) Close() error {
	return r.mempoolDB.Close()
}

// executes backtest over a range of blocks
func (r *Runner) RunBacktest(ctx context.Context, startBlock, endBlock uint64) (*BacktestReport, error) {
	report := &BacktestReport{
		StartBlock: startBlock,
		EndBlock:   endBlock,
		Results:    make([]*BlockResult, 0),
	}

	fmt.Printf("\nstarting backtest: blocks %d-%d\n", startBlock, endBlock)
	startTime := time.Now()

	for blockNum := startBlock; blockNum <= endBlock; blockNum++ {
		time.Sleep(500 * time.Millisecond)
		blockCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		result, err := r.ProcessBlock(blockCtx, blockNum)
		cancel()
		if err != nil {
			fmt.Printf("\nBlock %d error: %v\n", blockNum, err)
			continue
		}
		report.Results = append(report.Results, result)

		if blockNum%10 == 0 {
			elapsed := time.Since(startTime)
			fmt.Printf("📊 Processed %d/%d blocks (%.1f%%) - elapsed: %s\n",
				blockNum-startBlock+1,
				endBlock-startBlock+1,
				float64(blockNum-startBlock+1)/float64(endBlock-startBlock+1)*100,
				elapsed.Round(time.Second))
		}
	}

	report.CalculateMetrics()
	return report, nil
}

// runs detections for a single block
func (r *Runner) ProcessBlock(ctx context.Context, blockNum uint64) (*BlockResult, error) {
	_, err := simulator.NewStateFork(r.client, new(big.Int).SetUint64(blockNum-1))
	if err != nil {
		return nil, fmt.Errorf("fork state error at %d: %w", blockNum-1, err)
	}

	preMEV := new(big.Int).SetUint64(blockNum - 1)
	predicted := make([]*arbitrage.Opportunity, 0)

	for _, p := range trackedPairs {
		pools, err := arbitrage.GetPairPools(ctx, r.client, preMEV,
			p.tokenA, p.tokenADec, p.tokenB, p.tokenBDec)
		if err != nil {
			// Not enough active pools for this pair — skip
			continue
		}

		opp, err := arbitrage.DetectOpportunity(pools, r.gasPrice, r.gasLimit)
		if err == nil && opp != nil {
			opp.BlockNumber = blockNum
			predicted = append(predicted, opp)
		}
	}

	actual, err := FindActualArbitrages(ctx, r.client, blockNum)
	if err != nil {
		return nil, fmt.Errorf("find actual arb error: %w", err)
	}

	// Log missed blocks with spread info across all pairs
	if len(actual) > 0 && len(predicted) == 0 {
		for _, p := range trackedPairs {
			pools, err := arbitrage.GetPairPools(ctx, r.client, preMEV,
				p.tokenA, p.tokenADec, p.tokenB, p.tokenBDec)
			if err != nil {
				continue
			}
			prices := arbitrage.GetPoolPrices(pools)
			if len(prices) >= 2 {
				diff := arbitrage.ComparePrices(prices[0].Token1PerToken0, prices[1].Token1PerToken0)
				fmt.Printf("  ⚠️  MISSED block %d [%s]: spread=%.4f%%, actual_arbs=%d\n",
					blockNum, p.name, diff*100, len(actual))
			}
		}
	}

	return &BlockResult{
		BlockNumber: blockNum,
		Predicted:   predicted,
		Actual:      actual,
	}, nil
}