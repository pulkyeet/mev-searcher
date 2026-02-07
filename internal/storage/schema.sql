-- Account state cache
CREATE TABLE IF NOT EXISTS account_state (
    block_number INTEGER NOT NULL,
    address TEXT NOT NULL,
    balance TEXT,
    nonce INTEGER,
    code BLOB,
    PRIMARY KEY (block_number, address)
);

CREATE INDEX IF NOT EXISTS idx_account_block ON account_state(block_number);
CREATE INDEX IF NOT EXISTS idx_account_address ON account_state(address);

-- Storage slot cache
CREATE TABLE IF NOT EXISTS storage_state (
    block_number INTEGER NOT NULL,
    address TEXT NOT NULL,
    slot TEXT NOT NULL,
    value TEXT NOT NULL,
    PRIMARY KEY (block_number, address, slot)
);

CREATE INDEX IF NOT EXISTS idx_storage_block ON storage_state(block_number);
CREATE INDEX IF NOT EXISTS idx_storage_address ON storage_state(address);

-- Metadata for cache stats
CREATE TABLE IF NOT EXISTS cache_metadata (
    key TEXT PRIMARY KEY,
    value TEXT
);