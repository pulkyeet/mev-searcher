package simulator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
)

type SimulationResult struct {
	Success bool
	GasUsed uint64
	Logs []*types.Log
	ReturnData []byte
	RevertReason string
	StateChanges *StateChanges
}

type StateChanges struct {
	BalanceChanges map[common.Address]*big.Int
	NonceChanges map[common.Address]uint64
	StorageChanges map[common.Address]map[common.Hash]common.Hash
	CodeChanges map[common.Address][]byte
}

type StateCache struct {
	balances map[common.Address]*big.Int
	nonces map[common.Address]uint64
	code map[common.Address][]byte
	storage map[common.Address]map[common.Hash]common.Hash
}

func NewStateCache() *StateCache {
	return &StateCache{
        balances: make(map[common.Address]*big.Int),
        nonces:   make(map[common.Address]uint64),
        code:     make(map[common.Address][]byte),
        storage:  make(map[common.Address]map[common.Hash]common.Hash),
    }
}

type StateTracker struct {
	fork *StateFork
	changes *StateChanges
}

func NewStateTracker(fork *StateFork) *StateTracker {
	return &StateTracker{
		fork: fork,
		changes: &StateChanges{
			BalanceChanges: make(map[common.Address]*big.Int),
			NonceChanges:   make(map[common.Address]uint64),
			StorageChanges: make(map[common.Address]map[common.Hash]common.Hash),
			CodeChanges:    make(map[common.Address][]byte),
		},
	}
}