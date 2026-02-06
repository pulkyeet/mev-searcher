package simulator

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/tracing"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie/utils"
	"github.com/holiman/uint256"
	"github.com/ethereum/go-ethereum/crypto"
)

// ForkedStateDB implements vm.StateDB interface for our forked state
type ForkedStateDB struct {
	fork           *StateFork
	logs           []*types.Log
	refund         uint64
	accessList     map[common.Address]map[common.Hash]bool
	accessListAddr map[common.Address]bool
	originalStorage map[common.Address]map[common.Hash]common.Hash
}

func NewForkedStateDB(fork *StateFork) *ForkedStateDB {
	return &ForkedStateDB{
		fork:           fork,
		logs:           make([]*types.Log, 0),
		refund:         0,
		accessList:     make(map[common.Address]map[common.Hash]bool),
		accessListAddr: make(map[common.Address]bool),
		originalStorage: make(map[common.Address]map[common.Hash]common.Hash),
	}
}

// CreateAccount creates a new account
func (s *ForkedStateDB) CreateAccount(addr common.Address) {
	s.fork.SetBalance(addr, big.NewInt(0))
	s.fork.SetNonce(addr, 0)
}

// CreateContract is like CreateAccount but for contract creation
func (s *ForkedStateDB) CreateContract(addr common.Address) {
	s.CreateAccount(addr)
}

// Balance operations with uint256
func (s *ForkedStateDB) GetBalance(addr common.Address) *uint256.Int {
	bal, err := s.fork.GetBalance(addr)
	if err != nil {
		return uint256.NewInt(0)
	}
	val, overflow := uint256.FromBig(bal)
	if overflow {
		return uint256.NewInt(0)
	}
	return val
}

func (s *ForkedStateDB) AddBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	bal := s.GetBalance(addr)
	newBal := new(uint256.Int).Add(bal, amount)
	s.fork.SetBalance(addr, newBal.ToBig())
	return *bal // Return value, not pointer
}

func (s *ForkedStateDB) SubBalance(addr common.Address, amount *uint256.Int, reason tracing.BalanceChangeReason) uint256.Int {
	bal := s.GetBalance(addr)
	newBal := new(uint256.Int).Sub(bal, amount)
	s.fork.SetBalance(addr, newBal.ToBig())
	return *bal // Return value, not pointer
}

// Nonce operations
func (s *ForkedStateDB) GetNonce(addr common.Address) uint64 {
	nonce, err := s.fork.GetNonce(addr)
	if err != nil {
		return 0
	}
	return nonce
}

func (s *ForkedStateDB) SetNonce(addr common.Address, nonce uint64, reason tracing.NonceChangeReason) {
	s.fork.SetNonce(addr, nonce)
}

// Code operations
func (s *ForkedStateDB) GetCode(addr common.Address) []byte {
	code, err := s.fork.GetCode(addr)
	if err != nil {
		return nil
	}
	return code
}

func (s *ForkedStateDB) GetCodeSize(addr common.Address) int {
	return len(s.GetCode(addr))
}

func (s *ForkedStateDB) GetCodeHash(addr common.Address) common.Hash {
	code := s.GetCode(addr)
	if len(code) == 0 {
		if s.Exist(addr) {
			return  crypto.Keccak256Hash(nil)
		}
		return common.Hash{}
	}
	return common.BytesToHash(code)
}

func (s *ForkedStateDB) SetCode(addr common.Address, code []byte, reason tracing.CodeChangeReason) []byte {
	oldCode := s.GetCode(addr)
	s.fork.mu.Lock()
	defer s.fork.mu.Unlock()
	s.fork.cache.code[addr] = code
	return oldCode
}

// Storage operations
func (s *ForkedStateDB) GetState(addr common.Address, hash common.Hash) common.Hash {
	val, err := s.fork.GetStorageAt(addr, hash)
	if err != nil {
		return common.Hash{}
	}
	return val
}

func (s *ForkedStateDB) SetState(addr common.Address, key, value common.Hash) common.Hash {
	oldVal := s.GetState(addr, key)
	s.fork.SetStorageAt(addr, key, value)
	return oldVal
}

func (s *ForkedStateDB) GetStateAndCommittedState(addr common.Address, hash common.Hash) (common.Hash, common.Hash) {
    current := s.GetState(addr, hash)
    
    // Return cached original, or current is original (first read)
    if addrMap, ok := s.originalStorage[addr]; ok {
        if orig, ok := addrMap[hash]; ok {
            return current, orig
        }
    }
    // First time seeing this slot in this tx — current IS the original
    if s.originalStorage[addr] == nil {
        s.originalStorage[addr] = make(map[common.Hash]common.Hash)
    }
    s.originalStorage[addr][hash] = current
    return current, current
}

func (s *ForkedStateDB) GetStorageRoot(addr common.Address) common.Hash {
	return common.Hash{}
}

// Transient storage (EIP-1153)
func (s *ForkedStateDB) GetTransientState(addr common.Address, key common.Hash) common.Hash {
	return common.Hash{}
}

func (s *ForkedStateDB) SetTransientState(addr common.Address, key, value common.Hash) {
	// No-op for now
}

// Account existence
func (s *ForkedStateDB) Exist(addr common.Address) bool {
	code := s.GetCode(addr)
	balance := s.GetBalance(addr)
	nonce := s.GetNonce(addr)
	return len(code) > 0 || balance.Sign() > 0 || nonce > 0
}

func (s *ForkedStateDB) Empty(addr common.Address) bool {
	code := s.GetCode(addr)
	balance := s.GetBalance(addr)
	nonce := s.GetNonce(addr)
	return len(code) == 0 && balance.Sign() == 0 && nonce == 0
}

// Snapshot operations
func (s *ForkedStateDB) Snapshot() int {
	return s.fork.Snapshot()
}

func (s *ForkedStateDB) RevertToSnapshot(id int) {
	s.fork.RevertToSnapshot(id)
}

// Logs
func (s *ForkedStateDB) AddLog(log *types.Log) {
	s.logs = append(s.logs, log)
}

func (s *ForkedStateDB) Logs() []*types.Log {
	return s.logs
}

// Refunds
func (s *ForkedStateDB) AddRefund(gas uint64) {
	s.refund += gas
}

func (s *ForkedStateDB) SubRefund(gas uint64) {
	if gas > s.refund {
		s.refund = 0
	} else {
		s.refund -= gas
	}
}

func (s *ForkedStateDB) GetRefund() uint64 {
	return s.refund
}

// Preimages
func (s *ForkedStateDB) AddPreimage(hash common.Hash, preimage []byte) {}

// Self-destruct operations
func (s *ForkedStateDB) SelfDestruct(addr common.Address) uint256.Int {
	bal := s.GetBalance(addr)
	s.fork.SetBalance(addr, big.NewInt(0))
	return *bal
}

func (s *ForkedStateDB) HasSelfDestructed(addr common.Address) bool {
	return false
}

func (s *ForkedStateDB) SelfDestruct6780(addr common.Address) (uint256.Int, bool) {
	return s.SelfDestruct(addr), true
}

// Access list (EIP-2929)
func (s *ForkedStateDB) AddAddressToAccessList(addr common.Address) {s.accessListAddr[addr] = true}

func (s *ForkedStateDB) AddSlotToAccessList(addr common.Address, slot common.Hash) {
	s.accessListAddr[addr] = true
    if s.accessList[addr] == nil {
        s.accessList[addr] = make(map[common.Hash]bool)
    }
    s.accessList[addr][slot] = true
}

func (s *ForkedStateDB) AddressInAccessList(addr common.Address) bool {
	return s.accessListAddr[addr]
}

func (s *ForkedStateDB) SlotInAccessList(addr common.Address, slot common.Hash) (bool, bool) {
	addrOk := s.accessListAddr[addr]
    if !addrOk {
        return false, false
    }
    if s.accessList[addr] == nil {
        return true, false
    }
    return true, s.accessList[addr][slot]
}

// Prepare for transaction execution
func (s *ForkedStateDB) Prepare(rules params.Rules, sender, coinbase common.Address, dest *common.Address, precompiles []common.Address, txAccesses types.AccessList) {
	s.AddAddressToAccessList(sender)
	if dest != nil {
		s.AddAddressToAccessList(*dest)
	}
	s.AddAddressToAccessList(coinbase)
	for _, addr := range precompiles {
		s.AddAddressToAccessList(addr)
	}
	for _, el := range txAccesses {
		s.AddAddressToAccessList(el.Address)
		for _, key := range el.StorageKeys {
			s.AddSlotToAccessList(el.Address, key)
		}
	}
}

// Point cache for verkle trees
func (s *ForkedStateDB) PointCache() *utils.PointCache {
	return nil
}

// Witness for stateless execution
func (s *ForkedStateDB) Witness() *stateless.Witness {
	return nil
}

// Access events for EIP-2930
func (s *ForkedStateDB) AccessEvents() *state.AccessEvents {
	return nil
}

// Finalise completes the state transition
func (s *ForkedStateDB) Finalise(deleteEmptyObjects bool) {
	// No-op for our simple implementation
}
