package backtest

import (
	"math/big"
	"github.com/ethereum/go-ethereum/common"
	_ "github.com/ethereum/go-ethereum/core/types"
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