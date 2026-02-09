package backtest

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// converts a parquet row to mempooltx
func ParseParquetRow(row map[string]interface{}) (*MempoolTx, error) {
	hashStr, ok := row["hash"].(string)
	if !ok {
		return nil, fmt.Errorf("missing hash")
	}

	hash := common.HexToHash(hashStr)

	// extract timestamp (unix milliseconds)
	var timestamp uint64
	switch t := row["timestamp"].(type) {
	case int64:
		timestamp = uint64(t) / 1000 // Convert ms to seconds
	case uint64:
		timestamp = t / 1000
	default:
		return nil, fmt.Errorf("invalid timestamp type")
	}

	// extract included block
	var includedBlock uint64
	switch b := row["includedAtBlockHeight"].(type) {
	case int64:
		includedBlock = uint64(b)
	case uint64:
		includedBlock = b
	default:
		includedBlock = 0 // Not included yet
	}

	// extract raw tx bytes
	rawTx, ok := row["rowTx"].([]byte)
	if !ok {
		return nil, fmt.Errorf("missing rawTx")
	}

	// Decode the transaction to extract from/to/gasPrice
	var tx types.Transaction
	if err := rlp.DecodeBytes(rawTx, &tx); err != nil {
		return nil, fmt.Errorf("failed to decode tx: %w", err)
	}

	// Extract sender
	signer := types.LatestSignerForChainID(tx.ChainId())
	from, err := types.Sender(signer, &tx)
	if err != nil {
		return nil, fmt.Errorf("failed to extract sender: %w", err)
	}

	return &MempoolTx{
		Hash:          hash,
		Timestamp:     timestamp,
		IncludedBlock: includedBlock,
		RawTx:         rawTx,
		From:          from,
		To:            tx.To(),
		GasPrice:      tx.GasPrice(),
	}, nil
}

// Helper to convert hex string with 0x prefix to bytes
func hexToBytes(s string) ([]byte, error) {
	s = strings.TrimPrefix(s, "0x")
	return hex.DecodeString(s)
}

// Helper to parse big int from string
func parseBigInt(s string) (*big.Int, error) {
	val := new(big.Int)
	_, ok := val.SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("invalid big int: %s", s)
	}
	return val, nil
}