-- Mempool transaction storage
CREATE TABLE IF NOT EXISTS mempool_txs (
    tx_hash TEXT PRIMARY KEY,
    block_number INTEGER NOT NULL,
    timestamp INTEGER NOT NULL,
    raw_tx BLOB NOT NULL,
    gas_price TEXT,
    tx_to TEXT,
    tx_value TEXT
);

CREATE INDEX IF NOT EXISTS idx_mempool_block ON mempool_txs(block_number);
CREATE INDEX IF NOT EXISTS idx_mempool_timestamp ON mempool_txs(timestamp);