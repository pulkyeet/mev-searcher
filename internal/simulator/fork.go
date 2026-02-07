package simulator

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	lru "github.com/hashicorp/golang-lru/v2"
	"github.com/pulkyeet/mev-searcher/internal/eth"
	"github.com/pulkyeet/mev-searcher/internal/storage"
)

type StateFork struct {
	client      *eth.Client
	blockNumber *big.Int
	block       *types.Block

	// Layer 1: In-memory cache (current execution)
	cache *StateCache
	mu    sync.RWMutex

	// Layer 2: LRU cache (recent executions)
	lruBalance *lru.Cache[string, *big.Int]
	lruNonce   *lru.Cache[string, uint64]
	lruCode    *lru.Cache[string, []byte]
	lruStorage *lru.Cache[string, common.Hash]

	// Layer 3: SQLite (persistent across runs)
	db *storage.CacheDB

	// Stats
	stats *CacheStats

	// Snapshot for revert
	snapshots []*StateCache
}

type CacheStats struct {
	mu              sync.Mutex
	LRUHits         int
	SQLiteHits      int
	RPCCalls        int
	BatchedRPCCalls int
}

func NewStateFork(client *eth.Client, blockNumber *big.Int) (*StateFork, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	block, err := client.BlockByNumber(ctx, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch block %s: %w", blockNumber, err)
	}

	// Initialize LRU caches (10k entries each)
	lruBalance, _ := lru.New[string, *big.Int](10000)
	lruNonce, _ := lru.New[string, uint64](10000)
	lruCode, _ := lru.New[string, []byte](10000)
	lruStorage, _ := lru.New[string, common.Hash](50000)

	// Initialize SQLite cache
	db, err := storage.NewCacheDB("data/state_cache.db")
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache db: %w", err)
	}

	return &StateFork{
		client:      client,
		blockNumber: blockNumber,
		block:       block,
		cache:       NewStateCache(),
		lruBalance:  lruBalance,
		lruNonce:    lruNonce,
		lruCode:     lruCode,
		lruStorage:  lruStorage,
		db:          db,
		stats:       &CacheStats{},
		snapshots:   make([]*StateCache, 0),
	}, nil
}

// Cache key helpers
func balanceKey(block uint64, addr common.Address) string {
	return fmt.Sprintf("%d:%s", block, addr.Hex())
}

func storageKey(block uint64, addr common.Address, slot common.Hash) string {
	return fmt.Sprintf("%d:%s:%s", block, addr.Hex(), slot.Hex())
}

// GetBalance with 3-layer cache
func (f *StateFork) GetBalance(addr common.Address) (*big.Int, error) {
	// Layer 0: Current execution cache
	f.mu.RLock()
	if bal, ok := f.cache.balances[addr]; ok {
		f.mu.RUnlock()
		return new(big.Int).Set(bal), nil
	}
	f.mu.RUnlock()

	blockNum := f.blockNumber.Uint64()
	key := balanceKey(blockNum, addr)

	// Layer 1: LRU cache
	if bal, ok := f.lruBalance.Get(key); ok {
		f.stats.mu.Lock()
		f.stats.LRUHits++
		f.stats.mu.Unlock()
		
		f.mu.Lock()
		f.cache.balances[addr] = bal
		f.mu.Unlock()
		
		return new(big.Int).Set(bal), nil
	}

	// Layer 2: SQLite cache
	if bal, ok := f.db.GetBalance(blockNum, addr); ok {
		f.stats.mu.Lock()
		f.stats.SQLiteHits++
		f.stats.mu.Unlock()
		
		f.lruBalance.Add(key, bal)
		
		f.mu.Lock()
		f.cache.balances[addr] = bal
		f.mu.Unlock()
		
		return new(big.Int).Set(bal), nil
	}

	// Layer 3: RPC call
	f.stats.mu.Lock()
	f.stats.RPCCalls++
	f.stats.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bal, err := f.client.BalanceAt(ctx, addr, f.blockNumber)
	if err != nil {
		return nil, fmt.Errorf("RPC call failed for addr %s at block %s: %w", addr.Hex(), f.blockNumber, err)
	}

	// Store in all cache layers
	f.lruBalance.Add(key, bal)
	f.db.SetBalance(blockNum, addr, bal)
	
	f.mu.Lock()
	f.cache.balances[addr] = bal
	f.mu.Unlock()

	return new(big.Int).Set(bal), nil
}

// GetNonce with 3-layer cache
func (f *StateFork) GetNonce(addr common.Address) (uint64, error) {
	f.mu.RLock()
	if nonce, ok := f.cache.nonces[addr]; ok {
		f.mu.RUnlock()
		return nonce, nil
	}
	f.mu.RUnlock()

	blockNum := f.blockNumber.Uint64()
	key := balanceKey(blockNum, addr) // Reuse same key format

	// LRU
	if nonce, ok := f.lruNonce.Get(key); ok {
		f.stats.mu.Lock()
		f.stats.LRUHits++
		f.stats.mu.Unlock()
		
		f.mu.Lock()
		f.cache.nonces[addr] = nonce
		f.mu.Unlock()
		
		return nonce, nil
	}

	// SQLite
	if nonce, ok := f.db.GetNonce(blockNum, addr); ok {
		f.stats.mu.Lock()
		f.stats.SQLiteHits++
		f.stats.mu.Unlock()
		
		f.lruNonce.Add(key, nonce)
		
		f.mu.Lock()
		f.cache.nonces[addr] = nonce
		f.mu.Unlock()
		
		return nonce, nil
	}

	// RPC
	f.stats.mu.Lock()
	f.stats.RPCCalls++
	f.stats.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	nonce, err := f.client.NonceAt(ctx, addr, f.blockNumber)
	if err != nil {
		return 0, err
	}

	f.lruNonce.Add(key, nonce)
	f.db.SetNonce(blockNum, addr, nonce)
	
	f.mu.Lock()
	f.cache.nonces[addr] = nonce
	f.mu.Unlock()

	return nonce, nil
}

// GetCode with 3-layer cache
func (f *StateFork) GetCode(addr common.Address) ([]byte, error) {
	f.mu.RLock()
	if code, ok := f.cache.code[addr]; ok {
		f.mu.RUnlock()
		return code, nil
	}
	f.mu.RUnlock()

	blockNum := f.blockNumber.Uint64()
	key := balanceKey(blockNum, addr)

	// LRU
	if code, ok := f.lruCode.Get(key); ok {
		f.stats.mu.Lock()
		f.stats.LRUHits++
		f.stats.mu.Unlock()
		
		f.mu.Lock()
		f.cache.code[addr] = code
		f.mu.Unlock()
		
		return code, nil
	}

	// SQLite
	if code, ok := f.db.GetCode(blockNum, addr); ok {
		f.stats.mu.Lock()
		f.stats.SQLiteHits++
		f.stats.mu.Unlock()
		
		f.lruCode.Add(key, code)
		
		f.mu.Lock()
		f.cache.code[addr] = code
		f.mu.Unlock()
		
		return code, nil
	}

	// RPC
	f.stats.mu.Lock()
	f.stats.RPCCalls++
	f.stats.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	code, err := f.client.CodeAt(ctx, addr, f.blockNumber)
	if err != nil {
		return nil, err
	}

	f.lruCode.Add(key, code)
	f.db.SetCode(blockNum, addr, code)
	
	f.mu.Lock()
	f.cache.code[addr] = code
	f.mu.Unlock()

	return code, nil
}

// GetStorageAt with 3-layer cache
func (f *StateFork) GetStorageAt(addr common.Address, slot common.Hash) (common.Hash, error) {
	f.mu.RLock()
	if addrStorage, ok := f.cache.storage[addr]; ok {
		if val, ok := addrStorage[slot]; ok {
			f.mu.RUnlock()
			return val, nil
		}
	}
	f.mu.RUnlock()

	blockNum := f.blockNumber.Uint64()
	key := storageKey(blockNum, addr, slot)

	// LRU
	if val, ok := f.lruStorage.Get(key); ok {
		f.stats.mu.Lock()
		f.stats.LRUHits++
		f.stats.mu.Unlock()
		
		f.mu.Lock()
		if f.cache.storage[addr] == nil {
			f.cache.storage[addr] = make(map[common.Hash]common.Hash)
		}
		f.cache.storage[addr][slot] = val
		f.mu.Unlock()
		
		return val, nil
	}

	// SQLite
	if val, ok := f.db.GetStorage(blockNum, addr, slot); ok {
		f.stats.mu.Lock()
		f.stats.SQLiteHits++
		f.stats.mu.Unlock()
		
		f.lruStorage.Add(key, val)
		
		f.mu.Lock()
		if f.cache.storage[addr] == nil {
			f.cache.storage[addr] = make(map[common.Hash]common.Hash)
		}
		f.cache.storage[addr][slot] = val
		f.mu.Unlock()
		
		return val, nil
	}

	// RPC
	f.stats.mu.Lock()
	f.stats.RPCCalls++
	f.stats.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	data, err := f.client.StorageAt(ctx, addr, slot, f.blockNumber)
	if err != nil {
		return common.Hash{}, err
	}

	val := common.BytesToHash(data)

	f.lruStorage.Add(key, val)
	f.db.SetStorage(blockNum, addr, slot, val)
	
	f.mu.Lock()
	if f.cache.storage[addr] == nil {
		f.cache.storage[addr] = make(map[common.Hash]common.Hash)
	}
	f.cache.storage[addr][slot] = val
	f.mu.Unlock()

	return val, nil
}

// Prewarm using debug_traceTransaction (Layer 4)
func (f *StateFork) PrewarmFromTrace(txHash common.Hash) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	trace, err := f.client.TraceTransaction(ctx, txHash, f.blockNumber)
	if err != nil {
		// debug_trace might not be available on all RPC endpoints
		// Silently continue without prewarming
		return nil
	}

	// Batch fetch all touched accounts
	if len(trace.TouchedAddresses) > 0 {
		accountReqs := make([]eth.BatchAccountRequest, len(trace.TouchedAddresses))
		for i, addr := range trace.TouchedAddresses {
			accountReqs[i] = eth.BatchAccountRequest{
				Address:     addr,
				BlockNumber: f.blockNumber,
			}
		}

		results := f.client.BatchGetAccounts(ctx, accountReqs)
		
		f.stats.mu.Lock()
		f.stats.BatchedRPCCalls++
		f.stats.mu.Unlock()

		blockNum := f.blockNumber.Uint64()
		accounts := make([]storage.AccountData, 0, len(results))

		for _, res := range results {
			if res.Err != nil {
				continue
			}

			// Store in all cache layers
			key := balanceKey(blockNum, res.Address)
			f.lruBalance.Add(key, res.Balance)
			f.lruNonce.Add(key, res.Nonce)
			f.lruCode.Add(key, res.Code)

			accounts = append(accounts, storage.AccountData{
				Address: res.Address,
				Balance: res.Balance,
				Nonce:   res.Nonce,
				Code:    res.Code,
			})
		}

		// Batch write to SQLite
		if len(accounts) > 0 {
			f.db.BatchSetAccounts(blockNum, accounts)
		}
	}

	// Batch fetch all touched storage slots
	totalSlots := 0
	for _, slots := range trace.TouchedSlots {
		totalSlots += len(slots)
	}

	if totalSlots > 0 {
		storageReqs := make([]eth.BatchStorageRequest, 0, totalSlots)
		for addr, slots := range trace.TouchedSlots {
			for _, slot := range slots {
				storageReqs = append(storageReqs, eth.BatchStorageRequest{
					Address:     addr,
					Slot:        slot,
					BlockNumber: f.blockNumber,
				})
			}
		}

		results := f.client.BatchGetStorage(ctx, storageReqs)
		
		f.stats.mu.Lock()
		f.stats.BatchedRPCCalls++
		f.stats.mu.Unlock()

		blockNum := f.blockNumber.Uint64()
		storageData := make([]storage.StorageData, 0, len(results))

		for _, res := range results {
			if res.Err != nil {
				continue
			}

			key := storageKey(blockNum, res.Address, res.Slot)
			f.lruStorage.Add(key, res.Value)

			storageData = append(storageData, storage.StorageData{
				Address: res.Address,
				Slot:    res.Slot,
				Value:   res.Value,
			})
		}

		if len(storageData) > 0 {
			f.db.BatchSetStorage(blockNum, storageData)
		}
	}

	return nil
}

// Setters remain unchanged
func (f *StateFork) SetBalance(addr common.Address, bal *big.Int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache.balances[addr] = new(big.Int).Set(bal)
}

func (f *StateFork) SetNonce(addr common.Address, nonce uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cache.nonces[addr] = nonce
}

func (f *StateFork) SetStorageAt(addr common.Address, slot common.Hash, val common.Hash) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.cache.storage[addr] == nil {
		f.cache.storage[addr] = make(map[common.Hash]common.Hash)
	}
	f.cache.storage[addr][slot] = val
}

// Snapshot/revert unchanged
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

func (f *StateFork) GetStats() *CacheStats {
	return f.stats
}

func (f *StateFork) PrintStats() {
	f.stats.mu.Lock()
	defer f.stats.mu.Unlock()

	total := f.stats.LRUHits + f.stats.SQLiteHits + f.stats.RPCCalls
	if total == 0 {
		return
	}

	lruRate := float64(f.stats.LRUHits) / float64(total) * 100
	sqliteRate := float64(f.stats.SQLiteHits) / float64(total) * 100
	rpcRate := float64(f.stats.RPCCalls) / float64(total) * 100

	fmt.Printf("\n=== Cache Stats ===\n")
	fmt.Printf("LRU hits:     %d (%.1f%%)\n", f.stats.LRUHits, lruRate)
	fmt.Printf("SQLite hits:  %d (%.1f%%)\n", f.stats.SQLiteHits, sqliteRate)
	fmt.Printf("RPC calls:    %d (%.1f%%)\n", f.stats.RPCCalls, rpcRate)
	fmt.Printf("Batched RPCs: %d\n", f.stats.BatchedRPCCalls)
	fmt.Printf("Total:        %d\n\n", total)
}

func (f *StateFork) Close() error {
	if f.db != nil {
		return f.db.Close()
	}
	return nil
}