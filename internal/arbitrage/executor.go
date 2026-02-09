package arbitrage

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/pulkyeet/mev-searcher/internal/simulator"
)

type ArbExecutor struct {
	fork *simulator.StateFork
}

func NewArbExecutor(fork *simulator.StateFork) *ArbExecutor {
	return &ArbExecutor{fork: fork}
}

// gives the executor USDC and token approvals

func (e *ArbExecutor) SetupExecutorState(executor common.Address, usdcAmount *big.Int) error {
	// Give executor USDC balance
	usdcAddr := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	e.fork.SetBalance(executor, big.NewInt(1e18)) // 1 ETH for gas
	
	// Set USDC balance in storage
	// USDC uses slot: keccak256(abi.encode(address, uint256(9)))
	// where 9 is the balances mapping slot
	slot := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(executor.Bytes(), 32),
			common.LeftPadBytes(big.NewInt(9).Bytes(), 32)...,
		),
	)
	
	// Store USDC balance
	balanceBytes := common.LeftPadBytes(usdcAmount.Bytes(), 32)
	e.fork.SetStorageAt(usdcAddr, slot, common.BytesToHash(balanceBytes))
	
	// Approve Uniswap router for max USDC
	maxApproval := new(big.Int).SetBytes(common.Hex2Bytes("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"))
	
	// Approval slot for Uniswap: keccak256(abi.encode(router, keccak256(abi.encode(executor, 10))))
	// where 10 is the allowances mapping slot
	innerHash := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(executor.Bytes(), 32),
			common.LeftPadBytes(big.NewInt(10).Bytes(), 32)...,
		),
	)
	
	uniRouter := common.HexToAddress("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
	approvalSlot := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(uniRouter.Bytes(), 32),
			innerHash.Bytes()...,
		),
	)
	
	e.fork.SetStorageAt(usdcAddr, approvalSlot, common.BytesToHash(maxApproval.Bytes()))
	
	// Same for Sushiswap router
	sushiRouter := common.HexToAddress("0xd9e1cE17f2641f24aE83637ab66a2cca9C378B9F")
	approvalSlotSushi := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(sushiRouter.Bytes(), 32),
			innerHash.Bytes()...,
		),
	)
	
	e.fork.SetStorageAt(usdcAddr, approvalSlotSushi, common.BytesToHash(maxApproval.Bytes()))
	
	// Also approve WETH (we'll need this for the sell side)
	wethAddr := common.HexToAddress("0xC02aaA39b223FE8D0A0e5C4F27eAD9083C756Cc2")
	
	wethInnerHash := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(executor.Bytes(), 32),
			common.LeftPadBytes(big.NewInt(2).Bytes(), 32)..., // WETH allowances slot is 2
		),
	)
	
	wethApprovalUni := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(uniRouter.Bytes(), 32),
			wethInnerHash.Bytes()...,
		),
	)
	
	wethApprovalSushi := crypto.Keccak256Hash(
		append(
			common.LeftPadBytes(sushiRouter.Bytes(), 32),
			wethInnerHash.Bytes()...,
		),
	)
	
	e.fork.SetStorageAt(wethAddr, wethApprovalUni, common.BytesToHash(maxApproval.Bytes()))
	e.fork.SetStorageAt(wethAddr, wethApprovalSushi, common.BytesToHash(maxApproval.Bytes()))
	
	return nil
}

// builds and simulates arbitrage bundle and returns actual profit extracted from simulation

func (e *ArbExecutor) SimulateArbitrage(opp *Opportunity) (*SimulationResult, error) {
	block := e.fork.BlockContext()

	// generating a fake executor address 
	privateKey, _ := crypto.GenerateKey()
	executor := crypto.PubkeyToAddress(privateKey.PublicKey)

	legacyTxs, err := BuildArbTransactions(opp, executor, block.Time())
	if err!=nil {
		return nil, fmt.Errorf("failed to build transactions: %w", err)
	}

	signer := types.LatestSignerForChainID(big.NewInt(1))

	txs := make([]*types.Transaction, len(legacyTxs))
	for i, legacyTx := range legacyTxs {
		tx := types.NewTx(legacyTx)
		signedTx, err := types.SignTx(tx, signer, privateKey)
		if err!=nil {
			return nil, fmt.Errorf("failed to sign tx %d: %w", i, err)
		}
		txs[i] = signedTx
	}

	// executing bundle
	bundleSim := simulator.NewBundleSimulator(e.fork)
	bundleResult, err := bundleSim.ExecuteBundle(txs, block)
	if err!=nil {
		return nil, fmt.Errorf("bundle execution failed: %w", err)
	}

	if !bundleResult.Success {
		return &SimulationResult{
			Success: false,
			EstProfit: opp.EstProfit,
			ActualProfit: big.NewInt(0),
			RevertReason: getRevertReason(bundleResult),
		}, nil
	}

	// get actual profit from state changes
	actualProfit := extractProfitFromLogs(bundleResult, opp)

	return &SimulationResult{
		Success:      true,
		EstProfit:    opp.EstProfit,
		ActualProfit: actualProfit,
		GasUsed:      bundleResult.TotalGasUsed,
		TxResults:    bundleResult.Transactions,
	}, nil
}

type SimulationResult struct {
	Success bool
	EstProfit *big.Int
	ActualProfit *big.Int
	GasUsed uint64
	RevertReason string
	TxResults []*simulator.TxResult
}

// attempts to return actual profit from swap events; for now it returns estimated profit

func extractProfitFromLogs(result *simulator.BundleResult, opp *Opportunity) *big.Int {
	return opp.EstProfit
}

func getRevertReason(result *simulator.BundleResult) string {
	if result.RevertedAt >=0 && result.RevertedAt < len(result.Transactions) {
		return result.Transactions[result.RevertedAt].RevertReason
	}
	return "unknown"
}

// compares estimated profit vs simulated profit

func (r *SimulationResult) CompareResults() string {
	if !r.Success {
		return fmt.Sprintf("❌ Simulation FAILED: %s", r.RevertReason)
	}

	estFloat := new(big.Float).SetInt(r.EstProfit)
	actFloat := new(big.Float).SetInt(r.ActualProfit)
	
	estUSDC := new(big.Float).Quo(estFloat, big.NewFloat(1e6))
	actUSDC := new(big.Float).Quo(actFloat, big.NewFloat(1e6))

	// calculate accuracy
	diff := new(big.Float).Sub(actFloat, estFloat)
	pctError := new(big.Float).Quo(diff, estFloat)
	pctError.Mul(pctError, big.NewFloat(100))
	pctErrorVal, _ := pctError.Float64()

	return fmt.Sprintf(
		"Estimated: $%s USDC | Simulated: $%s USDC | Error: %.2f%%",
		estUSDC.Text('f', 2),
		actUSDC.Text('f', 2),
		pctErrorVal,
	)
}