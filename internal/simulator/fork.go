package simulator

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/pulkyeet/mev-searcher/internal/eth"
)

type StateFork struct {
	client      *eth.Client
	blockNumber *big.Int
	block       *types.Block

	// cache
	cache *StateCache
	mu    sync.RWMutex

	// snapshot for revert
	snapshots []*StateCache
}

func NewStateFork(client *eth.Client, blockNumber *big.Int) (*StateFork, error) {
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	block, err := client.BlockByNumber(ctx, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch block %s: %w", blockNumber, err)
	}

	return &StateFork{
		client:      client,
		blockNumber: blockNumber,
		block:       block,
		cache:       NewStateCache(),
		snapshots:   make([]*StateCache, 0),
	}, nil
}

// returns account balance at forked state
func (f *StateFork) GetBalance(addr common.Address) (*big.Int, error) {
	
	f.mu.RLock()
	if bal, ok := f.cache.balances[addr]; ok {
		f.mu.RUnlock()
		return new(big.Int).Set(bal), nil
	}
	f.mu.RUnlock()
	
	// Cache miss - fetch from RPC
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	bal, err := f.client.BalanceAt(ctx, addr, f.blockNumber)
	if err != nil {
		return nil, fmt.Errorf("RPC call failed for addr %s at block %s: %w", addr.Hex(), f.blockNumber, err)
	}

	f.mu.Lock()
	f.cache.balances[addr] = bal
	f.mu.Unlock()

	return new(big.Int).Set(bal), nil
}

// returns account nonce at forked state
func (f *StateFork) GetNonce(addr common.Address) (uint64, error) {
	f.mu.RLock()
	if nonce, ok := f.cache.nonces[addr]; ok {
		f.mu.RUnlock()
		return nonce, nil
	}
	f.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	nonce, err := f.client.NonceAt(ctx, addr, f.blockNumber)
	if err != nil {
		return 0, err
	}

	f.mu.Lock()
	f.cache.nonces[addr] = nonce
	f.mu.Unlock()

	return nonce, nil
}

// returns contract bytecode at forked state
func (f *StateFork) GetCode(addr common.Address) ([]byte, error) {
	f.mu.RLock()
	if code, ok := f.cache.code[addr]; ok {
		f.mu.RUnlock()
		return code, nil
	}
	f.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	code, err := f.client.CodeAt(ctx, addr, f.blockNumber)
	if err != nil {
		return nil, err
	}

	f.mu.Lock()
	f.cache.code[addr] = code
	f.mu.Unlock()

	return code, nil
}

// returns storage slot value at forked state
func (f *StateFork) GetStorageAt(addr common.Address, slot common.Hash) (common.Hash, error) {
	f.mu.RLock()
	if addrStorage, ok := f.cache.storage[addr]; ok {
		if val, ok := addrStorage[slot]; ok {
			f.mu.RUnlock()
			return val, nil
		}
	}
	f.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data, err := f.client.StorageAt(ctx, addr, slot, f.blockNumber)
	if err != nil {
		return common.Hash{}, err
	}

	val := common.BytesToHash(data)

	f.mu.Lock()
	if f.cache.storage[addr] == nil {
		f.cache.storage[addr] = make(map[common.Hash]common.Hash)
	}
	f.cache.storage[addr][slot] = val
	f.mu.Unlock()

	return val, nil
}

// modify balance for simulation
func (f *StateFork) SetBalance(addr common.Address, bal *big.Int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache.balances[addr] = new(big.Int).Set(bal)
}

// modify nonce for simulation
func (f *StateFork) SetNonce(addr common.Address, nonce uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.cache.nonces[addr] = nonce
}

// modifies storage
func (f *StateFork) SetStorageAt(addr common.Address, slot common.Hash, val common.Hash) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cache.storage[addr] == nil {
		f.cache.storage[addr] = make(map[common.Hash]common.Hash)
	}
	f.cache.storage[addr][slot] = val
}

// snapshot creates a revert point
func (f *StateFork) Snapshot() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	snap := &StateCache{
		balances: make(map[common.Address]*big.Int),
		nonces:   make(map[common.Address]uint64),
		code:     make(map[common.Address][]byte),
		storage:  make(map[common.Address]map[common.Hash]common.Hash),
	}

	for addr, bal := range f.cache.balances {
		snap.balances[addr] = new(big.Int).Set(bal)
	}

	for addr, nonce := range f.cache.nonces {
		snap.nonces[addr] = nonce
	}

	for addr, code := range f.cache.code {
		snap.code[addr] = code
	}

	for addr, slots := range f.cache.storage {
		snap.storage[addr] = make(map[common.Hash]common.Hash)
		for slot, val := range slots {
			snap.storage[addr][slot] = val
		}
	}

	f.snapshots = append(f.snapshots, snap)
	return len(f.snapshots) - 1
}

func (f *StateFork) RevertToSnapshot(snapID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if snapID < 0 || snapID >= len(f.snapshots) {
		return fmt.Errorf("invalid snapshot id: %d", snapID)
	}

	f.cache = f.snapshots[snapID]
	f.snapshots = f.snapshots[:snapID]

	return nil
}

func (f *StateFork) BlockContext() *types.Block {
	return f.block
}
