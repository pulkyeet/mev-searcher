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
	// Create state database wrapper
	stateDB := NewForkedStateDB(e.fork)

	// Build block context from target block
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

	// Extract sender from transaction
	signer := types.LatestSignerForChainID(tx.ChainId())
	sender, err := types.Sender(signer, tx)
	if err != nil {
		return nil, fmt.Errorf("failed to get sender: %w", err)
	}

	// Initialize EVM
	evm := vm.NewEVM(blockContext, stateDB, e.config, vm.Config{})
	evm.SetTxContext(vm.TxContext{
		Origin:   sender,
		GasPrice: tx.GasPrice(),
	})

	// Take snapshot for potential revert
	snap := stateDB.Snapshot()

	// Build message from transaction
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

	// Validate intrinsic gas
	_, err = core.IntrinsicGas(msg.Data, msg.AccessList, nil, msg.To == nil, true, true, true)
	if err != nil {
		return nil, fmt.Errorf("intrinsic gas validation failed: %w", err)
	}

	// Execute transaction
	gp := new(core.GasPool).AddGas(block.GasLimit())
	result, err := core.ApplyMessage(evm, msg, gp)
	if err != nil {
		stateDB.RevertToSnapshot(snap)
		return &SimulationResult{
			Success:      false,
			RevertReason: err.Error(),
		}, nil
	}

	// Build simulation result
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