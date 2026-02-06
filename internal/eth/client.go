package eth

import (
	"context"
	"math/big"
	"os"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
    "github.com/ethereum/go-ethereum/core/types"
    "github.com/ethereum/go-ethereum/ethclient"
    "github.com/joho/godotenv"
)

type Client struct {
	rpc *ethclient.Client
}

func NewClient() (*Client, error) {
	godotenv.Load()
	url := os.Getenv("ALCHEMY_URL")
	
	if url == "" {
		return nil, fmt.Errorf("ALCHEMY_URL not set in .env")
	}
	
	rpc, err := ethclient.Dial(url)
	if err != nil {
		return nil, err
	}
	
	return &Client{rpc: rpc}, nil
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