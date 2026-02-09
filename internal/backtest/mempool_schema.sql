-- Mempool transaction storage
CREATE TABLE IF NOT EXISTS mempool_txs (
    tx_hash TEXT PRIMARY KEY,
    timestamp INTEGER NOT NULL,
    included_block INTEGER NOT NULL,
    included_block_timestamp INTEGER,
    raw_tx TEXT NOT NULL,
    tx_from TEXT NOT NULL,
    tx_to TEXT,
    gas_price TEXT
);

CREATE INDEX IF NOT EXISTS idx_mempool_block ON mempool_txs(included_block);
CREATE INDEX IF NOT EXISTS idx_mempool_timestamp ON mempool_txs(timestamp);
CREATE INDEX IF NOT EXISTS idx_mempool_block_timestamp ON mempool_txs(included_block_timestamp);
CREATE INDEX IF NOT EXISTS idx_mempool_from ON mempool_txs(tx_from);