package simulator

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
)

type Executor struct {
	fork   *StateFork
	config *params.ChainConfig
}

func NewExecutor(fork *StateFork) *Executor {
	return &Executor{
		fork:   fork,
		config: params.MainnetChainConfig,
	}
}

func (e *Executor) ExecuteTransaction(tx *types.Transaction, targetBlock *types.Block) (*SimulationResult, error) {
	// create state db
	stateDB := NewForkedStateDB(e.fork)

	// get context (block)
	block := e.fork.BlockContext()
	blockContext := vm.BlockContext{
		CanTransfer: core.CanTransfer,
		Transfer:    core.Transfer,
		GetHash:     func(n uint64) common.Hash { return common.Hash{} },
		Coinbase:    targetBlock.Coinbase(),
		BlockNumber: targetBlock.Number(),
		Time:        targetBlock.Time(),
		Difficulty:  targetBlock.Difficulty(),
		GasLimit:    targetBlock.GasLimit(),
		BaseFee:     targetBlock.BaseFee(),
	}

	// get sender
	signer := types.LatestSignerForChainID(tx.ChainId())
	sender, err := types.Sender(signer, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}

	evm := vm.NewEVM(blockContext, stateDB, e.config, vm.Config{})

	// Set transaction context
	evm.SetTxContext(vm.TxContext{
		Origin:   sender,
		GasPrice: tx.GasPrice(),
	})

	// taking snapshot for revert
	snap := stateDB.Snapshot()

	// exec tx
	msg := &core.Message{
		To:         tx.To(),
		From:       sender,
		Nonce:      tx.Nonce(),
		Value:      tx.Value(),
		GasLimit:   tx.Gas(),
		GasPrice:   tx.GasPrice(),
		GasFeeCap:  tx.GasFeeCap(),
		GasTipCap:  tx.GasTipCap(),
		Data:       tx.Data(),
		AccessList: tx.AccessList(),
	}

	// calculate intrinsic gas
	intrinsicGas, err := core.IntrinsicGas(msg.Data, msg.AccessList, nil, msg.To == nil, true, true, true)
	if err != nil {
		return nil, fmt.Errorf("intrinsic gas calculation failed: %w", err)
	}
	fmt.Printf("DEBUG: Intrinsic gas: %d\n", intrinsicGas)
	fmt.Printf("DEBUG: tx gas limit: %d\n", msg.GasLimit)

	// apply message
	gp := new(core.GasPool).AddGas(block.GasLimit())
	result, err := core.ApplyMessage(evm, msg, gp)
	if err != nil {
		stateDB.RevertToSnapshot(snap)
		return &SimulationResult{
			Success:      false,
			RevertReason: err.Error(),
		}, nil
	}

	fmt.Printf("DEBUG: Result.UsedGas: %d\n", result.UsedGas)
	fmt.Printf("DEBUG: Result.Failed: %v\n", result.Failed())

	// build result
	simResult := &SimulationResult{
		Success:    !result.Failed(),
		GasUsed:    result.UsedGas,
		ReturnData: result.ReturnData,
		Logs:       stateDB.logs,
	}

	if result.Failed() {
		simResult.RevertReason = result.Err.Error()
		stateDB.RevertToSnapshot(snap)
	}

	return simResult, nil
}
