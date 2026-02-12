package backtest

import (
	"fmt"
	"math/big"
	"github.com/ethereum/go-ethereum/common"
	_ "github.com/ethereum/go-ethereum/core/types"
	"github.com/pulkyeet/mev-searcher/internal/arbitrage"
)

type MempoolTx struct {
	Hash common.Hash
	RawTx []byte
	Timestamp uint64
	BlockNumber uint64
	GasPrice *big.Int
	To *common.Address
	From common.Address
	IncludedBlock uint64
	Value *big.Int
}

// contains results from backtesting a single block
type BacktestResult struct {
	BlockNumber      uint64
	ActualArbs       []*DetectedArb
	PredictedArbs    []*DetectedArb
	TruePositives    int
	FalsePositives   int
	FalseNegatives   int
	ProfitAccuracy   float64
}

// represents an arb found in a block
type DetectedArb struct {
	TxHash       common.Hash
	Executor     common.Address
	ProfitUSDC   *big.Int
	BuyPool      common.Address
	SellPool     common.Address
	InputAmount  *big.Int
	OutputAmount *big.Int
}

// aggregates results across multiple blocks
type BacktestMetrics struct {
	BlocksAnalyzed   int
	TotalActual      int
	TotalPredicted   int
	TruePositives    int
	FalsePositives   int
	FalseNegatives   int
	HitRate          float64  // Recall
	Precision        float64
	AvgProfitError   float64
}

//  stores prediction vs actual for one block
type BlockResult struct {
	BlockNumber uint64
	Predicted   []*arbitrage.Opportunity
	Actual      []*ActualArbitrage
}

// aggregates results across multiple blocks
type BacktestReport struct {
	StartBlock uint64
	EndBlock   uint64
	Results    []*BlockResult
	
	// Metrics (calculated after run)
	TotalBlocks      int
	TotalPredicted   int
	TotalActual      int
	TruePositives    int  // blocks where we predicted AND actual arb existed
	FalsePositives   int  // blocks where we predicted but no actual arb
	FalseNegatives   int  // blocks where actual arb but we didn't predict
}

func (r *BacktestReport) CalculateMetrics() {
	r.TotalBlocks = len(r.Results)

	for _, result := range r.Results {
		hasPredicted := len(result.Predicted)>0
		hasActual := len(result.Actual)>0

		r.TotalPredicted += len(result.Predicted)
		r.TotalActual += len(result.Actual)

		if hasPredicted&&hasActual {
			r.TruePositives++
		} else if hasPredicted && !hasActual {
			r.FalsePositives++
		} else if !hasPredicted && hasActual {
			r.FalseNegatives++
		}
	}
}

func (r *BacktestReport) Print() {
	fmt.Println("\n╔════════════════════════════════════════════╗")
	fmt.Println("║     MEV Searcher Backtest Report          ║")
	fmt.Println("╚════════════════════════════════════════════╝")
	fmt.Printf("\nBlocks analyzed:        %d (blocks %d-%d)\n", r.TotalBlocks, r.StartBlock, r.EndBlock)
	fmt.Printf("\nOpportunities:\n")
	fmt.Printf("  Predicted:            %d\n", r.TotalPredicted)
	fmt.Printf("  Actual (ground truth):%d\n", r.TotalActual)
	fmt.Printf("\nBlock-level accuracy:\n")
	fmt.Printf("  True Positives:       %d (predicted + actual)\n", r.TruePositives)
	fmt.Printf("  False Positives:      %d (predicted, no actual)\n", r.FalsePositives)
	fmt.Printf("  False Negatives:      %d (actual, not predicted)\n", r.FalseNegatives)
	
	if r.TotalPredicted > 0 {
		precision := float64(r.TruePositives) / float64(r.TruePositives + r.FalsePositives) * 100
		fmt.Printf("\nPrecision:              %.1f%%\n", precision)
	}
	
	if r.TotalActual > 0 {
		recall := float64(r.TruePositives) / float64(r.TruePositives + r.FalseNegatives) * 100
		fmt.Printf("Recall (Hit Rate):      %.1f%%\n", recall)
	}
	
	fmt.Println("\n" + string(make([]byte, 46)))
}