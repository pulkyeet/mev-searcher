package eth

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum"  
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/joho/godotenv"
)

type Client struct {
	rpc *ethclient.Client
	rawRPC *rpc.Client
}

func NewClient() (*Client, error) {
	godotenv.Load()
	url := os.Getenv("ALCHEMY_URL")
	
	if url == "" {
		return nil, fmt.Errorf("ALCHEMY_URL not set in .env")
	}
	
	// Create raw RPC client first
	rawRPCClient, err := rpc.Dial(url)
	if err != nil {
		return nil, err
	}
	
	// Wrap in ethclient
	ethClient := ethclient.NewClient(rawRPCClient)
	
	return &Client{
		rpc:    ethClient,
		rawRPC: rawRPCClient,
	}, nil
}

func (c *Client) BlockByNumber(ctx context.Context, number *big.Int) (*types.Block, error) {
    return c.rpc.BlockByNumber(ctx, number)
}

func (c *Client) BalanceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (*big.Int, error) {
    return c.rpc.BalanceAt(ctx, account, blockNumber)
}

func (c *Client) CodeAt(ctx context.Context, account common.Address, blockNumber *big.Int) ([]byte, error) {
    return c.rpc.CodeAt(ctx, account, blockNumber)
}

func (c *Client) StorageAt(ctx context.Context, account common.Address, key common.Hash, blockNumber *big.Int) ([]byte, error) {
    return c.rpc.StorageAt(ctx, account, key, blockNumber)
}

func (c *Client) NonceAt(ctx context.Context, account common.Address, blockNumber *big.Int) (uint64, error) {
    return c.rpc.NonceAt(ctx, account, blockNumber)
}

func (c *Client) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return c.rpc.TransactionReceipt(ctx, txHash)
}

func (c *Client) CallContract(ctx context.Context, msg ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	return c.rpc.CallContract(ctx, msg, blockNumber)
}

// batch RPC call structures

type BatchAccountRequest struct {
	Address common.Address
	BlockNumber *big.Int
}

type BatchAccountResult struct {
	Address common.Address
	Balance *big.Int
	Nonce uint64
	Code []byte
	Err error
}

// batch call to get multiple accounts in single RPC call

func (c *Client) BatchGetAccounts(ctx context.Context, requests []BatchAccountRequest) []BatchAccountResult {
	results := make([]BatchAccountResult, len(requests))

	if len(requests)==0 {
		return results
	}

	// building a batch request
	batch := make([]rpc.BatchElem, len(requests)*3) // balance, nonce and code for each account

	for i, req := range requests {
		blockNumHex := toBlockNumArg(req.BlockNumber)
		
		// balance
		batch[i*3] = rpc.BatchElem{
			Method: "eth_getBalance",
			Args: []interface{}{req.Address, blockNumHex},
			Result: new(string),
		}

		// nonce
		batch[i*3+1] = rpc.BatchElem{
			Method: "eth_getTransactionCount",
			Args: []interface{}{req.Address, blockNumHex},
			Result: new(string),
		}

		// code
		batch[i*3+2] = rpc.BatchElem{
			Method: "eth_getCode",
			Args: []interface{}{req.Address, blockNumHex},
			Result: new(string),
		}
	}

	// execute batch
	if err:= c.rawRPC.BatchCallContext(ctx, batch); err!=nil {
		// if batch fails entirely, mark all as errors
		for i:= range results {
			results[i].Address = requests[i].Address
			results[i].Err = err
		}
		return results
	}

	// parse results
	for i:= range requests {
		results[i].Address = requests[i].Address

		if batch[i*3].Error!=nil {
			results[i].Err = batch[i*3].Error
			continue
		}
		balanceHex := *batch[i*3].Result.(*string)
		balance := new(big.Int)
		balance.SetString(balanceHex[2:], 16) // removing 0x
		results[i].Balance = balance

		// parse nonce
		if batch[i*3+1].Error!=nil {
			results[i].Err = batch[i*3+1].Error
			continue
		}
		nonceHex := *batch[i*3+1].Result.(*string)
		var nonce uint64
		fmt.Sscanf(nonceHex, "0x%x", &nonce)
		results[i].Nonce = nonce

		// parse code
		if batch[i*3+2].Error!=nil {
			results[i].Err = batch[i*3+2].Error
			continue
		}
		codeHex := *batch[i*3+2].Result.(*string)
		results[i].Code = common.FromHex(codeHex)
	}

	return results
}

type BatchStorageRequest struct {
	Address common.Address
	Slot common.Hash
	BlockNumber *big.Int
}

type BatchStorageResult struct {
	Address common.Address
	Slot common.Hash
	Value common.Hash
	Err error
}

// fetch multiple storage slots in a single batched rpc call

func (c *Client) BatchGetStorage(ctx context.Context, requests []BatchStorageRequest) []BatchStorageResult {
	results := make([]BatchStorageResult, len(requests))

	if len(requests)==0 {
		return results
	}

	batch := make([]rpc.BatchElem, len(requests))

	for i, req := range requests {
		blockNumHex := toBlockNumArg(req.BlockNumber)

		batch[i] = rpc.BatchElem{
			Method: "eth_getStorageAt",
			Args: []interface{}{req.Address, req.Slot, blockNumHex},
			Result: new(string),
		}
	}

	if err:=c.rawRPC.BatchCallContext(ctx, batch); err!=nil {
		for i:= range results {
			results[i].Address = requests[i].Address
			results[i].Slot = requests[i].Slot
			results[i].Err = err
		}
		return results
	}

	for i := range requests {
		results[i].Address = requests[i].Address
		results[i].Slot = requests[i].Slot

		if batch[i].Error!=nil {
			results[i].Err = batch[i].Error
			continue
		}

		valueHex := *batch[i].Result.(*string)
		results[i].Value = common.HexToHash(valueHex)
	}

	return results
}

// debug traceTransaction support
type TraceResult struct {
	TouchedAddresses []common.Address
	TouchedSlots map[common.Address][]common.Hash
}

// calls debug_traceTransaction to get all touched addresses/slots

func (c *Client) TraceTransaction(ctx context.Context, txHash common.Hash, blockNumber *big.Int) (*TraceResult, error) {
	var result map[string]interface{}

	// set timeout (trace can be slow)
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	err := c.rawRPC.CallContext(ctx, &result, "debug_traceTransaction", txHash, map[string]interface{}{
		"tracer": "prestateTracer",
	})

	if err!=nil {
		return nil, fmt.Errorf("debug_traceTransaction failed: %w", err)
	}

	trace := &TraceResult{
		TouchedAddresses: make([]common.Address, 0),
		TouchedSlots: make(map[common.Address][]common.Hash),
	}

	// parse prestate tracer output
	for addrHex, data := range result {
		addr := common.HexToAddress(addrHex)
		trace.TouchedAddresses = append(trace.TouchedAddresses, addr)

		// check for storage accesses
		if dataMap, ok := data.(map[string]interface{}); ok {
			if storage, ok := dataMap["storage"].(map[string]interface{}); ok {
				slots := make([]common.Hash, 0, len(storage))
				for slotHex := range storage {
					slots = append(slots, common.HexToHash(slotHex))
				}
				if len(slots)>0 {
					trace.TouchedSlots[addr] = slots
				}
			}
		}
	}

	return trace, nil
}

// convert bigInt to hex block number for rpc
func toBlockNumArg(number *big.Int) string {
	if number == nil {
		return "latest"
	}
	return fmt.Sprintf("0x%x", number)
}