package simulator

import (
	"fmt"

	"github.com/ethereum/go-ethereum/core/types"
)

type BundleSimulator struct {
	executor *Executor
}

func NewBundleSimulator(f *StateFork) *BundleSimulator {
	return &BundleSimulator{executor: NewExecutor(f)}
}

// Execute bundle execs transactions atomically; all succed or all fail
func (b *BundleSimulator) ExecuteBundle(txs []*types.Transaction, block *types.Block) (*BundleResult, error)  {
	if len(txs)==0 {
		return nil, fmt.Errorf("empty bundle")
	}

	// taking a snapshot so we can revert back if bundle fails
	snapID := b.executor.fork.Snapshot()
	result := &BundleResult{
		Success: true,
		Transactions: make([]*TxResult, 0, len(txs)),
		TotalGasUsed: 0,
		RevertedAt: -1,
	}

	// exec each tx in order
	for i, tx := range txs {
		fmt.Printf("\nBundle[%d/%d]: Executing %s...\n", i+1, len(txs), tx.Hash().Hex())

		simResult, err := b.executor.ExecuteTransaction(tx, block)
		if err!=nil {
			b.executor.fork.RevertToSnapshot(snapID)
			return nil, fmt.Errorf("\nbundle tx %d failed with error: %w", i, err)
		}

		txResult := &TxResult{
			TxHash: tx.Hash(),
			Success: simResult.Success,
			GasUsed: simResult.GasUsed,
			Logs: simResult.Logs,
			ReturnData: simResult.ReturnData,
			RevertReason: simResult.RevertReason,
		}
		result.Transactions = append(result.Transactions, txResult)
		result.TotalGasUsed += simResult.GasUsed

		// if tx failed, reverting entire bundle
		if !simResult.Success {
			fmt.Printf("  └─ REVERTED: %s\n", simResult.RevertReason)
			result.Success = false
			result.RevertedAt = i
			b.executor.fork.RevertToSnapshot(snapID)
			return result, nil
		}
		fmt.Printf("  └─ Success: %d gas\n", simResult.GasUsed)
	}

	// all txs succeed
	fmt.Printf("\n Bundle executed successfully: %d transactions, %d total gas n", len(txs), result.TotalGasUsed)
	return result, nil
}