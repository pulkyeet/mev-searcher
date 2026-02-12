# MEV Searcher

Production-grade Ethereum arbitrage detector that simulates transactions against forked mainnet state, detects opportunities across DEX pools, and validates predictions through historical backtesting.

**Stack:** Go, go-ethereum, SQLite, Alchemy RPC, Flashbots mempool data  
**Purpose:** Portfolio piece demonstrating blockchain infrastructure engineering and DeFi market research

## Architecture

High-level system design:

```
Data Sources
  ├── Alchemy Archive RPC (historical state)
  └── Flashbots mempool-dumpster (pending transactions)
       ↓
EVM Simulator
  ├── State forking at any block
  ├── Transaction execution engine
  └── 4-layer caching (execution → LRU → SQLite → batched RPC)
       ↓
Arbitrage Detector
  ├── Pool state tracking (CREATE2 address computation)
  ├── Multi-DEX support (Uniswap, Sushiswap, Shibaswap)
  ├── Optimal input calculation (AMM math)
  └── Profit estimation with gas costs
       ↓
Backtester
  ├── Historical block replay
  ├── Actual arbitrage detection (on-chain pattern matching)
  └── Prediction validation metrics
```

## Tech Stack

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Language | Go 1.21+ | Performance, geth compatibility |
| EVM | go-ethereum/core/vm | Direct geth integration |
| State | go-ethereum/core/state | StateDB with snapshot/revert |
| RPC | Alchemy (free tier) | Archive node access |
| Caching | golang-lru + SQLite | Multi-layer state cache |
| Historical Data | Flashbots mempool-dumpster | Free mempool snapshots |

Cost: $0/month

## Setup

Prerequisites: Go 1.21+, SQLite3, Python 3.8+, Alchemy API key

```bash
git clone <repo>
cd mev-searcher

# Install dependencies
go mod download
pip install pyarrow pandas

# Configure
export ALCHEMY_API_KEY="your-key"

# Download mempool data for target block range
python scripts/ingest_mempool.py --start 18500000 --end 18501000

# Build
make build
```

## Usage

Run backtest over historical blocks:

```bash
./bin/backtest --start 18500000 --end 18501000
```

Outputs:
- Predicted opportunities (from detector)
- Actual arbitrages executed (from on-chain data)
- Comparison metrics (precision, recall, profit accuracy)

Simulate single transaction:

```bash
./bin/simulate --block 18500000 --tx 0xabcd...
```

Scan specific block for opportunities:

```bash
./bin/scan --block 18500000
```

## Features

**EVM Simulator**
- Forks mainnet state at any historical block
- Executes transactions with exact geth semantics
- Snapshot/revert for atomic operations
- 4-layer caching: execution cache, LRU (10K entries), SQLite persistent, batched RPC prewarming
- Performance: sub-100ms transaction simulation, 85%+ cache hit rate

**Arbitrage Detector**
- CREATE2-based pool address computation (zero RPC overhead)
- Multi-DEX pool tracking (Uniswap V2, Sushiswap, Shibaswap)
- Optimal input calculation using closed-form AMM math
- Gas cost estimation and profitability threshold filtering
- Monitors: WETH/USDC, WETH/USDT, WETH/DAI, WETH/WBTC, WETH/CRV, WETH/LINK, WETH/UNI

**Backtester**
- Replays historical blocks (18.5M - 18.51M, Oct 2023)
- Loads mempool state at each block
- Detects actual arbitrages via pattern matching (swap A → swap B in same tx)
- Validates predictions against ground truth
- Generates precision/recall metrics

## Results

Tested 1,000 blocks (18,500,000-18,501,000), ~160K transactions, October 2023.

**High-Liquidity Pairs (WETH/USDC, USDT, DAI, WBTC)**
- Predicted: 0 opportunities
- Actual: 0 arbitrages executed
- Typical spreads: 0.05-0.3%
- Fee structure: 0.6% total (0.3% per swap)
- Conclusion: Fee barrier eliminates all profitable opportunities

**Low-Liquidity Pairs (WETH/CRV, LINK, UNI)**
- Predicted: 7 opportunities
- Actual: 0 arbitrages executed
- Example: WETH/CRV persistent 0.768% spread across 7 blocks
  - Capital required: $100,000
  - Net profit: $133.81
  - ROI: 0.13%
  - Why ignored: Capital efficiency too poor, risk-adjusted returns negative

**Performance**
- Cache hits: 91% (78% LRU, 13% SQLite)
- RPC calls reduced: 7x vs naive implementation
- Average latency: 87ms per transaction simulation

## Key Findings

Simple Uniswap V2↔V2 arbitrage is economically extinct.

The 0.6% combined fee wall (0.3% × 2 swaps) exceeds typical price spreads (0.05-0.3%) on high-liquidity pairs. Even when spreads exist on low-liquidity pairs, capital requirements ($100K+) far exceed profitability ($100-200), explaining why opportunities persist uncaptured.

This represents a "dead pool" phenomenon: pools with structural imbalances that no rational actor arbitrages because the economics don't justify the capital deployment and execution risk.

Real MEV activity has migrated to:
- Uniswap V3 (0.05% fee tier = 0.35% total vs 0.6%)
- Sandwich attacks (70% of MEV volume)
- Multi-hop arbitrage paths
- JIT liquidity provision

The research validates that building MEV infrastructure requires understanding not just technical execution but economic viability. A working detector that finds zero results reveals more about market structure than one that claims false opportunities.

## Technical Highlights

**CREATE2 Address Computation**

Eliminates RPC calls for pool address lookup. Computes deterministically:

```go
salt := keccak256(token0 ++ token1)  // tokens sorted by address
pool := keccak256(0xff ++ factory ++ salt ++ initCodeHash)[12:]
```

Result: Zero RPC overhead for discovering pools.

**Optimal Input Derivation**

Closed-form solution instead of binary search. Given reserves (x₁, y₁) and (x₂, y₂), profit-maximizing input:

```
dx* = √[(x₁ × x₂ × y₁ × 997²) / (y₂ × 1000²)] - x₁
```

Derived by setting marginal profit to zero: ∂profit/∂dx = 0

**Multi-Layer Caching**

Hierarchical approach reduces RPC dependency:

1. Execution cache: In-memory modifications during tx execution
2. LRU cache: 10K most-recently-used entries (78% hit rate)
3. SQLite persistent: Survives process restarts (13% hit rate)
4. Batched RPC: Prefetch using debug_traceTransaction

Impact: 85%+ cache hit rate, 7x fewer RPC calls

**Batched State Prefetching**

Single debug_traceTransaction call reveals all touched state:

```go
trace := debug_traceTransaction(tx)
accounts := BatchGetAccounts(trace.addresses)  // 1 RPC call
storage := BatchGetStorage(trace.slots)        // 1 RPC call
```

Replaces 100+ individual calls with 3 total.

## License

MIT
